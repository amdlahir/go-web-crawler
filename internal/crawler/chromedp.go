package crawler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"

	"github.com/mhq-projects/web-crawler/internal/config"
	"github.com/mhq-projects/web-crawler/internal/dedup"
	"github.com/mhq-projects/web-crawler/internal/parser"
	"github.com/mhq-projects/web-crawler/pkg/models"
)

// ChromedpCrawler uses headless Chrome for JS-heavy pages.
type ChromedpCrawler struct {
	allocCtx context.Context
	cancel   context.CancelFunc
	cfg      config.ChromedpConfig
}

// NewChromedpCrawler creates a new Chrome-based crawler.
func NewChromedpCrawler(cfg config.ChromedpConfig) (*ChromedpCrawler, error) {
	var allocCtx context.Context
	var cancel context.CancelFunc

	if cfg.RemoteURL != "" {
		// Connect to remote Chrome instance
		allocCtx, cancel = chromedp.NewRemoteAllocator(context.Background(), cfg.RemoteURL)
	} else {
		// Use local Chrome
		allocCtx, cancel = chromedp.NewExecAllocator(context.Background(),
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
		)
	}

	// Test the allocator
	testCtx, testCancel := chromedp.NewContext(allocCtx)
	defer testCancel()

	testTimeout, testTimeoutCancel := context.WithTimeout(testCtx, 10*time.Second)
	defer testTimeoutCancel()

	if err := chromedp.Run(testTimeout); err != nil {
		cancel()
		return nil, fmt.Errorf("test chrome connection: %w", err)
	}

	return &ChromedpCrawler{
		allocCtx: allocCtx,
		cancel:   cancel,
		cfg:      cfg,
	}, nil
}

// Crawl fetches and parses a URL using headless Chrome.
func (c *ChromedpCrawler) Crawl(ctx context.Context, u *models.URL) (*models.CrawlResult, error) {
	result := &models.CrawlResult{
		URL:       u.Normalized,
		CrawledAt: time.Now(),
		Depth:     u.Depth,
		Metadata:  make(map[string]string),
	}

	// Extract domain
	host, _ := models.ExtractHost(u.Normalized)
	result.Domain = host

	// Create new browser context
	taskCtx, taskCancel := chromedp.NewContext(c.allocCtx)
	defer taskCancel()

	// Set timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(taskCtx, c.cfg.Timeout)
	defer timeoutCancel()

	var html string
	var title string
	var finalURL string

	start := time.Now()

	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(u.Normalized),
		chromedp.WaitReady("body"),
		chromedp.Sleep(c.cfg.WaitTime), // Wait for JS to render
		chromedp.Title(&title),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	)

	result.FetchDuration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		// Try to get partial results
		if title != "" {
			result.Title = title
		}
		if finalURL != "" {
			result.FinalURL = finalURL
		}
		return result, err
	}

	result.StatusCode = 200 // Chrome doesn't expose status code easily
	result.FinalURL = finalURL
	result.Title = title
	result.ContentLength = len(html)
	result.RawHTML = []byte(html)

	// Parse rendered HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		result.Error = fmt.Sprintf("parse html: %v", err)
		return result, err
	}

	// Extract content
	parsed := parser.Parse(doc, finalURL)
	result.Content = parsed.Content
	result.Links = parsed.Links
	result.LinksCount = len(parsed.Links)
	result.Metadata = parsed.Metadata

	// Use parsed title if we didn't get one from Chrome
	if result.Title == "" {
		result.Title = parsed.Title
	}

	// Compute hashes
	result.ContentHash = models.ComputeContentHash([]byte(html))
	result.SimHash = dedup.SimHash(result.Content)

	return result, nil
}

// RequiresJS always returns true for Chrome crawler.
func (c *ChromedpCrawler) RequiresJS(url string) bool {
	return true
}

// Close releases Chrome resources.
func (c *ChromedpCrawler) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}
