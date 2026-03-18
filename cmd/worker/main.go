package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/amdlahir/go-web-crawler/internal/config"
	"github.com/amdlahir/go-web-crawler/internal/crawler"
	"github.com/amdlahir/go-web-crawler/internal/dedup"
	"github.com/amdlahir/go-web-crawler/internal/frontier"
	"github.com/amdlahir/go-web-crawler/internal/metrics"
	"github.com/amdlahir/go-web-crawler/internal/storage"
	"github.com/amdlahir/go-web-crawler/pkg/models"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Load configuration
	cfg := config.Load()

	logger.Info("starting worker",
		zap.String("worker_id", cfg.Worker.ID),
	)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	// Initialize Redis
	redisClient, err := storage.NewRedisClient(cfg.Redis)
	if err != nil {
		logger.Fatal("failed to connect to redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Initialize OpenSearch
	osClient, err := storage.NewOpenSearchClient(cfg.OpenSearch)
	if err != nil {
		logger.Fatal("failed to connect to opensearch", zap.Error(err))
	}

	// Ensure index exists
	if err := osClient.EnsureIndex(ctx); err != nil {
		logger.Error("failed to ensure opensearch index", zap.Error(err))
	}

	// Initialize MinIO
	minioClient, err := storage.NewMinIOClient(cfg.MinIO)
	if err != nil {
		logger.Fatal("failed to connect to minio", zap.Error(err))
	}

	// Ensure bucket exists
	if err := minioClient.EnsureBucket(ctx); err != nil {
		logger.Error("failed to ensure minio bucket", zap.Error(err))
	}

	// Initialize frontier
	front, err := frontier.New(redisClient, cfg.Frontier)
	if err != nil {
		logger.Fatal("failed to create frontier", zap.Error(err))
	}

	// Initialize robots manager
	robotsMgr := frontier.NewRobotsManager(redisClient, cfg.Crawler, cfg.Frontier.RobotsTTL)

	// Initialize content dedup
	contentDedup := dedup.NewContentDedup(redisClient, dedup.DefaultSimilarityThreshold)

	// Initialize crawler manager
	crawlerMgr, err := crawler.NewManager(cfg.Crawler, cfg.Chromedp)
	if err != nil {
		logger.Fatal("failed to create crawler manager", zap.Error(err))
	}
	defer crawlerMgr.Close()

	// Start metrics server
	metricsServer := metrics.NewServer(cfg.Metrics)
	go func() {
		if err := metricsServer.Start(); err != nil {
			logger.Error("metrics server error", zap.Error(err))
		}
	}()

	// Create worker
	w := &worker{
		id:           cfg.Worker.ID,
		cfg:          cfg,
		logger:       logger,
		frontier:     front,
		robotsMgr:    robotsMgr,
		contentDedup: contentDedup,
		crawlerMgr:   crawlerMgr,
		osClient:     osClient,
		minioClient:  minioClient,
	}

	// Run worker loop
	logger.Info("worker started, waiting for URLs")
	metrics.UpdateActiveWorkers(1)

	if err := w.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("worker error", zap.Error(err))
	}

	metrics.UpdateActiveWorkers(0)

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Worker.ShutdownTimeout)
	defer shutdownCancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", zap.Error(err))
	}

	logger.Info("worker stopped")
}

type worker struct {
	id           string
	cfg          *config.Config
	logger       *zap.Logger
	frontier     *frontier.Frontier
	robotsMgr    *frontier.RobotsManager
	contentDedup *dedup.ContentDedup
	crawlerMgr   *crawler.Manager
	osClient     *storage.OpenSearchClient
	minioClient  *storage.MinIOClient
}

func (w *worker) run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Get next URL from frontier
		u, err := w.frontier.NextURL(ctx)
		if err != nil {
			if errors.Is(err, frontier.ErrNoURLs) {
				// No URLs available, wait and retry
				time.Sleep(100 * time.Millisecond)
				continue
			}
			w.logger.Error("failed to get next url", zap.Error(err))
			time.Sleep(1 * time.Second)
			continue
		}

		// Process the URL
		if err := w.processURL(ctx, u); err != nil {
			w.logger.Error("failed to process url",
				zap.String("url", u.Normalized),
				zap.Error(err),
			)
		}
	}
}

func (w *worker) processURL(ctx context.Context, u *models.URL) error {
	w.logger.Debug("processing url",
		zap.String("url", u.Normalized),
		zap.Int("depth", u.Depth),
	)

	// Check robots.txt
	if w.cfg.Crawler.RespectRobots {
		allowed, crawlDelay, err := w.robotsMgr.IsAllowed(ctx, u.Normalized)
		if err != nil {
			w.logger.Warn("robots.txt check failed", zap.Error(err))
		} else if !allowed {
			metrics.RobotsBlocked.Inc()
			w.logger.Debug("url blocked by robots.txt", zap.String("url", u.Normalized))
			return nil
		} else if crawlDelay > 0 {
			// Update politeness delay from robots.txt
			// This is handled by the frontier
		}
	}

	// Crawl the URL
	result, err := w.crawlerMgr.Crawl(ctx, u)
	if err != nil {
		metrics.RecordFetch(w.id, false, 0, 0)
		metrics.RecordError(w.id, categorizeError(err))
		w.frontier.Fail(ctx, u, err)
		return err
	}

	// Record metrics
	metrics.RecordFetch(w.id, result.Success(), result.FetchDuration.Seconds(), result.ContentLength)

	if !result.Success() {
		metrics.RecordError(w.id, "http_"+string(rune(result.StatusCode/100))+"xx")
		w.frontier.Fail(ctx, u, errors.New(result.Error))
		return nil
	}

	// Check for content duplicates
	isDupe, err := w.contentDedup.CheckAndStore(ctx, result.ContentHash, result.SimHash)
	if err != nil {
		w.logger.Warn("content dedup check failed", zap.Error(err))
	} else if isDupe {
		metrics.RecordDuplicate("content")
		w.logger.Debug("duplicate content detected", zap.String("url", u.Normalized))
		w.frontier.Complete(ctx, u, result)
		return nil
	}

	// Store raw HTML in MinIO
	if len(result.RawHTML) > 0 {
		if err := w.minioClient.StoreHTML(ctx, result.ContentHash, result.RawHTML); err != nil {
			w.logger.Warn("failed to store html", zap.Error(err))
			metrics.RecordStorage("minio", "store", false)
		} else {
			metrics.RecordStorage("minio", "store", true)
		}
	}

	// Store parsed content in OpenSearch
	if err := w.osClient.StorePage(ctx, result); err != nil {
		w.logger.Warn("failed to store page", zap.Error(err))
		metrics.RecordStorage("opensearch", "store", false)
	} else {
		metrics.RecordStorage("opensearch", "store", true)
	}

	// Extract and queue new URLs
	if len(result.Links) > 0 && u.Depth < w.cfg.Crawler.MaxDepth {
		metrics.RecordLinks(w.id, len(result.Links))

		newURLs := make([]*models.URL, 0, len(result.Links))
		for _, link := range result.Links {
			newURL, err := models.NewURL(link, u.Depth+1, u.Priority, u.Normalized)
			if err != nil {
				continue
			}
			newURLs = append(newURLs, newURL)
		}

		added, err := w.frontier.AddURLs(ctx, newURLs)
		if err != nil {
			w.logger.Warn("failed to add urls", zap.Error(err))
		} else {
			w.logger.Debug("added new urls",
				zap.Int("added", added),
				zap.Int("total", len(result.Links)),
			)
		}
	}

	// Mark URL as completed
	w.frontier.Complete(ctx, u, result)

	w.logger.Info("crawled url",
		zap.String("url", u.Normalized),
		zap.Int("status", result.StatusCode),
		zap.Duration("duration", result.FetchDuration),
		zap.Int("links", len(result.Links)),
	)

	return nil
}

func categorizeError(err error) string {
	errStr := err.Error()
	switch {
	case contains(errStr, "timeout"):
		return "timeout"
	case contains(errStr, "dns"):
		return "dns"
	case contains(errStr, "connection"):
		return "connection"
	case contains(errStr, "tls"):
		return "tls"
	default:
		return "other"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if matchLower(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func matchLower(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
