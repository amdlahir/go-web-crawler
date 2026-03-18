package crawler

import (
	"context"
	"regexp"
	"strings"

	"github.com/amdlahir/go-web-crawler/internal/config"
	"github.com/amdlahir/go-web-crawler/pkg/models"
)

// Crawler defines the interface for crawling URLs.
type Crawler interface {
	// Crawl fetches and parses a URL.
	Crawl(ctx context.Context, url *models.URL) (*models.CrawlResult, error)

	// RequiresJS checks if a URL likely requires JS rendering.
	RequiresJS(url string) bool

	// Close releases resources.
	Close() error
}

// Manager manages static and dynamic crawlers.
type Manager struct {
	static  *CollyCrawler
	dynamic *ChromedpCrawler
	cfg     config.CrawlerConfig
}

// NewManager creates a new crawler manager.
func NewManager(cfg config.CrawlerConfig, chromedpCfg config.ChromedpConfig) (*Manager, error) {
	static := NewCollyCrawler(cfg)

	var dynamic *ChromedpCrawler
	if chromedpCfg.RemoteURL != "" {
		var err error
		dynamic, err = NewChromedpCrawler(chromedpCfg)
		if err != nil {
			// Log warning but continue without JS support
			dynamic = nil
		}
	}

	return &Manager{
		static:  static,
		dynamic: dynamic,
		cfg:     cfg,
	}, nil
}

// Crawl fetches a URL using the appropriate crawler.
func (m *Manager) Crawl(ctx context.Context, url *models.URL) (*models.CrawlResult, error) {
	if m.RequiresJS(url.Normalized) && m.dynamic != nil {
		return m.dynamic.Crawl(ctx, url)
	}
	return m.static.Crawl(ctx, url)
}

// RequiresJS checks if a URL likely requires JavaScript rendering.
func (m *Manager) RequiresJS(url string) bool {
	// Check for known SPA frameworks/patterns
	jsPatterns := []string{
		"angular",
		"react",
		"vue",
		"ember",
		"backbone",
	}

	urlLower := strings.ToLower(url)
	for _, pattern := range jsPatterns {
		if strings.Contains(urlLower, pattern) {
			return true
		}
	}

	// Check for hash-based routing
	if strings.Contains(url, "/#/") || strings.Contains(url, "#!/") {
		return true
	}

	return false
}

// Close releases all crawler resources.
func (m *Manager) Close() error {
	if m.static != nil {
		m.static.Close()
	}
	if m.dynamic != nil {
		m.dynamic.Close()
	}
	return nil
}

// jsHeavyDomains are domains known to require JS rendering.
var jsHeavyDomains = map[string]bool{
	"twitter.com":   true,
	"x.com":         true,
	"instagram.com": true,
	"facebook.com":  true,
	"linkedin.com":  true,
}

// jsHeavyPattern matches URLs that likely need JS.
var jsHeavyPattern = regexp.MustCompile(`(?i)(app\.|spa\.|dashboard\.)`)

// RequiresJSAdvanced performs more thorough JS detection.
func RequiresJSAdvanced(url string) bool {
	// Extract domain
	host, err := models.ExtractHost(url)
	if err != nil {
		return false
	}

	// Check known JS-heavy domains
	if jsHeavyDomains[host] {
		return true
	}

	// Check subdomain patterns
	if jsHeavyPattern.MatchString(host) {
		return true
	}

	return false
}
