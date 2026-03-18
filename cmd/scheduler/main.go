package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/mhq-projects/web-crawler/internal/config"
	"github.com/mhq-projects/web-crawler/internal/frontier"
	"github.com/mhq-projects/web-crawler/internal/metrics"
	"github.com/mhq-projects/web-crawler/internal/storage"
	"github.com/mhq-projects/web-crawler/pkg/models"
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

	logger.Info("starting scheduler")

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

	// Initialize frontier
	front, err := frontier.New(redisClient, cfg.Frontier)
	if err != nil {
		logger.Fatal("failed to create frontier", zap.Error(err))
	}

	// Create scheduler
	s := &scheduler{
		cfg:      cfg,
		logger:   logger,
		redis:    redisClient,
		os:       osClient,
		frontier: front,
	}

	// Start metrics server
	metricsServer := metrics.NewServer(cfg.Metrics)
	go func() {
		if err := metricsServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", zap.Error(err))
		}
	}()

	// Start API server
	apiServer := s.newAPIServer()
	go func() {
		logger.Info("starting API server", zap.Int("port", 8081))
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("API server error", zap.Error(err))
		}
	}()

	// Start stats collector
	go s.collectStats(ctx)

	logger.Info("scheduler started")

	// Wait for shutdown
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("API server shutdown error", zap.Error(err))
	}

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", zap.Error(err))
	}

	logger.Info("scheduler stopped")
}

type scheduler struct {
	cfg      *config.Config
	logger   *zap.Logger
	redis    *storage.RedisClient
	os       *storage.OpenSearchClient
	frontier *frontier.Frontier
}

func (s *scheduler) newAPIServer() *http.Server {
	mux := http.NewServeMux()

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", s.handleStats)

	// Queue endpoints
	mux.HandleFunc("/api/v1/queue", s.handleQueue)
	mux.HandleFunc("/api/v1/queue/", s.handleQueueHost)

	// Seeds endpoint
	mux.HandleFunc("/api/v1/seeds", s.handleSeeds)

	// Health endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}
}

func (s *scheduler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get frontier stats
	frontierStats, err := s.frontier.Stats(ctx)
	if err != nil {
		s.logger.Error("failed to get frontier stats", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get OpenSearch count
	osCount, err := s.os.GetStats(ctx)
	if err != nil {
		s.logger.Warn("failed to get opensearch stats", zap.Error(err))
	}

	// Get active workers
	workers, err := s.redis.Keys(ctx, storage.KeyWorkerActivePrefix+"*")
	if err != nil {
		s.logger.Warn("failed to get active workers", zap.Error(err))
	}

	response := map[string]interface{}{
		"queue": map[string]interface{}{
			"pending":   frontierStats.TotalPending,
			"completed": frontierStats.TotalCompleted,
			"failed":    frontierStats.TotalFailed,
			"hosts":     len(frontierStats.HostQueueCounts),
		},
		"storage": map[string]interface{}{
			"indexed_pages": osCount,
		},
		"workers": map[string]interface{}{
			"active": len(workers),
		},
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *scheduler) handleQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	stats, err := s.frontier.Stats(ctx)
	if err != nil {
		s.logger.Error("failed to get queue stats", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *scheduler) handleQueueHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract host from path
	host := r.URL.Path[len("/api/v1/queue/"):]
	if host == "" {
		http.Error(w, "host required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get host queue length
	queueKey := storage.HostQueueKey(host)
	count, err := s.redis.LLen(ctx, queueKey)
	if err != nil {
		s.logger.Error("failed to get queue length", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get politeness data
	politenessKey := storage.PolitenessKey(host)
	politeness, _ := s.redis.HGetAll(ctx, politenessKey)

	response := map[string]interface{}{
		"host":       host,
		"pending":    count,
		"politeness": politeness,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type seedRequest struct {
	URLs     []string `json:"urls"`
	Priority int      `json:"priority"`
	Depth    int      `json:"depth"`
}

func (s *scheduler) handleSeeds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req seedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.URLs) == 0 {
		http.Error(w, "urls required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Convert to URL objects
	urls := make([]*models.URL, 0, len(req.URLs))
	for _, rawURL := range req.URLs {
		u, err := models.NewURL(rawURL, req.Depth, req.Priority, "")
		if err != nil {
			s.logger.Warn("invalid url", zap.String("url", rawURL), zap.Error(err))
			continue
		}
		urls = append(urls, u)
	}

	// Add to frontier
	added, err := s.frontier.AddURLs(ctx, urls)
	if err != nil {
		s.logger.Error("failed to add seeds", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"added":     added,
		"submitted": len(req.URLs),
		"invalid":   len(req.URLs) - len(urls),
	}

	s.logger.Info("seeds added",
		zap.Int("added", added),
		zap.Int("submitted", len(req.URLs)),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *scheduler) collectStats(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateMetrics(ctx)
		}
	}
}

func (s *scheduler) updateMetrics(ctx context.Context) {
	// Get frontier stats
	stats, err := s.frontier.Stats(ctx)
	if err != nil {
		s.logger.Warn("failed to get frontier stats", zap.Error(err))
		return
	}

	// Update queue depth metrics
	metrics.UpdateTotalQueueDepth(stats.TotalPending)
	for host, count := range stats.HostQueueCounts {
		metrics.UpdateQueueDepth(host, count)
	}

	// Update active workers
	workers, err := s.redis.Keys(ctx, storage.KeyWorkerActivePrefix+"*")
	if err == nil {
		metrics.UpdateActiveWorkers(len(workers))
	}
}
