package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/amdlahir/go-web-crawler/internal/config"
)

var (
	// PagesFetched counts total pages fetched.
	PagesFetched = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_pages_fetched_total",
			Help: "Total number of pages fetched",
		},
		[]string{"worker_id", "status"},
	)

	// FetchDuration tracks page fetch duration.
	FetchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "crawler_fetch_duration_seconds",
			Help:    "Page fetch duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		[]string{"worker_id"},
	)

	// QueueDepth tracks the URL queue depth.
	QueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "crawler_queue_depth",
			Help: "Number of URLs in the queue",
		},
		[]string{"host"},
	)

	// TotalQueueDepth tracks total queue depth across all hosts.
	TotalQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "crawler_total_queue_depth",
			Help: "Total number of URLs in all queues",
		},
	)

	// ActiveWorkers tracks number of active workers.
	ActiveWorkers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "crawler_active_workers",
			Help: "Number of active crawler workers",
		},
	)

	// ErrorsTotal counts crawl errors.
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_errors_total",
			Help: "Total number of crawl errors",
		},
		[]string{"worker_id", "error_type"},
	)

	// ContentSize tracks content size.
	ContentSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "crawler_content_size_bytes",
			Help:    "Content size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 15),
		},
		[]string{"worker_id"},
	)

	// LinksExtracted counts extracted links.
	LinksExtracted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_links_extracted_total",
			Help: "Total number of links extracted",
		},
		[]string{"worker_id"},
	)

	// DuplicatesDetected counts detected duplicates.
	DuplicatesDetected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_duplicates_detected_total",
			Help: "Total number of duplicates detected",
		},
		[]string{"type"},
	)

	// StorageOperations tracks storage operations.
	StorageOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_storage_operations_total",
			Help: "Total storage operations",
		},
		[]string{"storage", "operation", "status"},
	)

	// RobotsBlocked counts URLs blocked by robots.txt.
	RobotsBlocked = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "crawler_robots_blocked_total",
			Help: "Total URLs blocked by robots.txt",
		},
	)

	// JSRenderRequired counts URLs requiring JS rendering.
	JSRenderRequired = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "crawler_js_render_required_total",
			Help: "Total URLs requiring JavaScript rendering",
		},
	)
)

// Server provides HTTP metrics endpoint.
type Server struct {
	server *http.Server
	cfg    config.MetricsConfig
}

// NewServer creates a new metrics server.
func NewServer(cfg config.MetricsConfig) *Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.Path, promhttp.Handler())
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)

	return &Server{
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Port),
			Handler: mux,
		},
		cfg: cfg,
	}
}

// Start starts the metrics server.
func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the metrics server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// RecordFetch records a page fetch.
func RecordFetch(workerID string, success bool, duration float64, contentSize int) {
	status := "success"
	if !success {
		status = "error"
	}

	PagesFetched.WithLabelValues(workerID, status).Inc()
	FetchDuration.WithLabelValues(workerID).Observe(duration)
	ContentSize.WithLabelValues(workerID).Observe(float64(contentSize))
}

// RecordError records a crawl error.
func RecordError(workerID, errorType string) {
	ErrorsTotal.WithLabelValues(workerID, errorType).Inc()
}

// RecordLinks records extracted links.
func RecordLinks(workerID string, count int) {
	LinksExtracted.WithLabelValues(workerID).Add(float64(count))
}

// RecordDuplicate records a detected duplicate.
func RecordDuplicate(dupType string) {
	DuplicatesDetected.WithLabelValues(dupType).Inc()
}

// RecordStorage records a storage operation.
func RecordStorage(storage, operation string, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	StorageOperations.WithLabelValues(storage, operation, status).Inc()
}

// UpdateQueueDepth updates queue depth gauge.
func UpdateQueueDepth(host string, depth int64) {
	QueueDepth.WithLabelValues(host).Set(float64(depth))
}

// UpdateTotalQueueDepth updates total queue depth.
func UpdateTotalQueueDepth(depth int64) {
	TotalQueueDepth.Set(float64(depth))
}

// UpdateActiveWorkers updates active workers count.
func UpdateActiveWorkers(count int) {
	ActiveWorkers.Set(float64(count))
}
