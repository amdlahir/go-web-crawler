package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the crawler.
type Config struct {
	// Worker settings
	Worker WorkerConfig

	// Redis settings
	Redis RedisConfig

	// OpenSearch settings
	OpenSearch OpenSearchConfig

	// MinIO settings
	MinIO MinIOConfig

	// Crawler settings
	Crawler CrawlerConfig

	// Frontier settings
	Frontier FrontierConfig

	// Chromedp settings
	Chromedp ChromedpConfig

	// Metrics settings
	Metrics MetricsConfig
}

// WorkerConfig holds worker-specific settings.
type WorkerConfig struct {
	ID             string
	Concurrency    int
	ShutdownTimeout time.Duration
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL      string
	Password string
	DB       int
	PoolSize int
}

// OpenSearchConfig holds OpenSearch connection settings.
type OpenSearchConfig struct {
	URLs     []string
	Username string
	Password string
	Index    string
}

// MinIOConfig holds MinIO connection settings.
type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// CrawlerConfig holds crawler behavior settings.
type CrawlerConfig struct {
	UserAgent       string
	Timeout         time.Duration
	MaxRetries      int
	MaxBodySize     int64
	RespectRobots   bool
	FollowRedirects bool
	MaxRedirects    int
	MaxDepth        int
}

// FrontierConfig holds URL frontier settings.
type FrontierConfig struct {
	DefaultDelay    time.Duration
	MaxDelay        time.Duration
	BloomSize       uint
	BloomHashCount  uint
	RobotsTTL       time.Duration
	BatchSize       int
}

// ChromedpConfig holds headless Chrome settings.
type ChromedpConfig struct {
	RemoteURL   string
	Timeout     time.Duration
	WaitTime    time.Duration
	PoolSize    int
}

// MetricsConfig holds Prometheus metrics settings.
type MetricsConfig struct {
	Enabled bool
	Port    int
	Path    string
}

// Load loads configuration from environment variables.
func Load() *Config {
	return &Config{
		Worker: WorkerConfig{
			ID:             getEnv("WORKER_ID", "worker-1"),
			Concurrency:    getEnvInt("WORKER_CONCURRENCY", 1),
			ShutdownTimeout: getEnvDuration("WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			PoolSize: getEnvInt("REDIS_POOL_SIZE", 10),
		},
		OpenSearch: OpenSearchConfig{
			URLs:     []string{getEnv("OPENSEARCH_URL", "http://localhost:9200")},
			Username: getEnv("OPENSEARCH_USERNAME", ""),
			Password: getEnv("OPENSEARCH_PASSWORD", ""),
			Index:    getEnv("OPENSEARCH_INDEX", "crawled_pages"),
		},
		MinIO: MinIOConfig{
			Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			Bucket:    getEnv("MINIO_BUCKET", "crawler-raw"),
			UseSSL:    getEnvBool("MINIO_USE_SSL", false),
		},
		Crawler: CrawlerConfig{
			UserAgent:       getEnv("CRAWLER_USER_AGENT", "WebCrawler/1.0 (+https://github.com/mhq-projects/web-crawler)"),
			Timeout:         getEnvDuration("CRAWLER_TIMEOUT", 30*time.Second),
			MaxRetries:      getEnvInt("CRAWLER_MAX_RETRIES", 3),
			MaxBodySize:     getEnvInt64("CRAWLER_MAX_BODY_SIZE", 10*1024*1024), // 10MB
			RespectRobots:   getEnvBool("CRAWLER_RESPECT_ROBOTS", true),
			FollowRedirects: getEnvBool("CRAWLER_FOLLOW_REDIRECTS", true),
			MaxRedirects:    getEnvInt("CRAWLER_MAX_REDIRECTS", 10),
			MaxDepth:        getEnvInt("CRAWLER_MAX_DEPTH", 10),
		},
		Frontier: FrontierConfig{
			DefaultDelay:   getEnvDuration("FRONTIER_DEFAULT_DELAY", 1*time.Second),
			MaxDelay:       getEnvDuration("FRONTIER_MAX_DELAY", 60*time.Second),
			BloomSize:      uint(getEnvInt("FRONTIER_BLOOM_SIZE", 10000000)),      // 10M
			BloomHashCount: uint(getEnvInt("FRONTIER_BLOOM_HASH_COUNT", 10)),
			RobotsTTL:      getEnvDuration("FRONTIER_ROBOTS_TTL", 24*time.Hour),
			BatchSize:      getEnvInt("FRONTIER_BATCH_SIZE", 100),
		},
		Chromedp: ChromedpConfig{
			RemoteURL: getEnv("CHROMEDP_REMOTE_URL", "ws://localhost:9222"),
			Timeout:   getEnvDuration("CHROMEDP_TIMEOUT", 60*time.Second),
			WaitTime:  getEnvDuration("CHROMEDP_WAIT_TIME", 2*time.Second),
			PoolSize:  getEnvInt("CHROMEDP_POOL_SIZE", 5),
		},
		Metrics: MetricsConfig{
			Enabled: getEnvBool("METRICS_ENABLED", true),
			Port:    getEnvInt("METRICS_PORT", 8080),
			Path:    getEnv("METRICS_PATH", "/metrics"),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
