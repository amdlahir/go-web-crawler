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

	logger.Info("starting seed API")

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

	// Initialize frontier
	front, err := frontier.New(redisClient, cfg.Frontier)
	if err != nil {
		logger.Fatal("failed to create frontier", zap.Error(err))
	}

	// Create API
	api := &seedAPI{
		logger:   logger,
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
	server := api.newServer()
	go func() {
		logger.Info("starting API server on :8082")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("API server error", zap.Error(err))
		}
	}()

	logger.Info("seed API started")

	// Wait for shutdown
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("API server shutdown error", zap.Error(err))
	}

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", zap.Error(err))
	}

	logger.Info("seed API stopped")
}

type seedAPI struct {
	logger   *zap.Logger
	frontier *frontier.Frontier
}

func (a *seedAPI) newServer() *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/seeds", a.handleSeeds)
	mux.HandleFunc("/api/v1/seeds/bulk", a.handleSeedsBulk)
	mux.HandleFunc("/api/v1/stats", a.handleStats)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	return &http.Server{
		Addr:    ":8082",
		Handler: mux,
	}
}

type seedRequest struct {
	URL      string            `json:"url"`
	Priority int               `json:"priority"`
	Depth    int               `json:"depth"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type seedBulkRequest struct {
	URLs     []string `json:"urls"`
	Priority int      `json:"priority"`
	Depth    int      `json:"depth"`
}

type seedResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

type seedBulkResponse struct {
	Added     int      `json:"added"`
	Submitted int      `json:"submitted"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
}

func (a *seedAPI) handleSeeds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.addSeed(w, r)
	case http.MethodGet:
		a.getStats(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *seedAPI) addSeed(w http.ResponseWriter, r *http.Request) {
	var req seedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, seedResponse{
			Success: false,
			Message: "invalid request body",
		})
		return
	}

	if req.URL == "" {
		sendJSON(w, http.StatusBadRequest, seedResponse{
			Success: false,
			Message: "url required",
		})
		return
	}

	ctx := r.Context()

	u, err := models.NewURL(req.URL, req.Depth, req.Priority, "")
	if err != nil {
		sendJSON(w, http.StatusBadRequest, seedResponse{
			Success: false,
			Message: "invalid url: " + err.Error(),
		})
		return
	}

	if req.Metadata != nil {
		u.Metadata = req.Metadata
	}

	added, err := a.frontier.AddURLs(ctx, []*models.URL{u})
	if err != nil {
		a.logger.Error("failed to add seed", zap.Error(err))
		sendJSON(w, http.StatusInternalServerError, seedResponse{
			Success: false,
			Message: "internal error",
		})
		return
	}

	if added == 0 {
		sendJSON(w, http.StatusOK, seedResponse{
			Success: true,
			Message: "url already exists",
			URL:     u.Normalized,
		})
		return
	}

	a.logger.Info("seed added", zap.String("url", u.Normalized))

	sendJSON(w, http.StatusCreated, seedResponse{
		Success: true,
		Message: "seed added",
		URL:     u.Normalized,
	})
}

func (a *seedAPI) handleSeedsBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req seedBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, seedBulkResponse{
			Errors: []string{"invalid request body"},
		})
		return
	}

	if len(req.URLs) == 0 {
		sendJSON(w, http.StatusBadRequest, seedBulkResponse{
			Errors: []string{"urls required"},
		})
		return
	}

	ctx := r.Context()

	var urls []*models.URL
	var errors []string

	for _, rawURL := range req.URLs {
		u, err := models.NewURL(rawURL, req.Depth, req.Priority, "")
		if err != nil {
			errors = append(errors, rawURL+": "+err.Error())
			continue
		}
		urls = append(urls, u)
	}

	added, err := a.frontier.AddURLs(ctx, urls)
	if err != nil {
		a.logger.Error("failed to add seeds", zap.Error(err))
		sendJSON(w, http.StatusInternalServerError, seedBulkResponse{
			Errors: []string{"internal error"},
		})
		return
	}

	a.logger.Info("bulk seeds added",
		zap.Int("added", added),
		zap.Int("submitted", len(req.URLs)),
	)

	sendJSON(w, http.StatusCreated, seedBulkResponse{
		Added:     added,
		Submitted: len(req.URLs),
		Skipped:   len(urls) - added,
		Errors:    errors,
	})
}

func (a *seedAPI) getStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := a.frontier.Stats(ctx)
	if err != nil {
		a.logger.Error("failed to get stats", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sendJSON(w, http.StatusOK, stats)
}

func (a *seedAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.getStats(w, r)
}

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
