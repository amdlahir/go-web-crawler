package crawler

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"

	"github.com/amdlahir/go-web-crawler/internal/config"
	"github.com/amdlahir/go-web-crawler/internal/dedup"
	"github.com/amdlahir/go-web-crawler/internal/parser"
	"github.com/amdlahir/go-web-crawler/pkg/models"
)

// CollyCrawler uses Colly for static page crawling.
type CollyCrawler struct {
	cfg config.CrawlerConfig
}

// NewCollyCrawler creates a new Colly-based crawler.
func NewCollyCrawler(cfg config.CrawlerConfig) *CollyCrawler {
	return &CollyCrawler{cfg: cfg}
}

// Crawl fetches and parses a URL using Colly.
func (c *CollyCrawler) Crawl(ctx context.Context, u *models.URL) (*models.CrawlResult, error) {
	result := &models.CrawlResult{
		URL:       u.Normalized,
		CrawledAt: time.Now(),
		Depth:     u.Depth,
		Metadata:  make(map[string]string),
	}

	// Extract domain
	host, _ := models.ExtractHost(u.Normalized)
	result.Domain = host

	// Create a new collector for each request
	collector := colly.NewCollector(
		colly.UserAgent(c.cfg.UserAgent),
		colly.MaxDepth(0), // We manage depth externally
	)

	collector.SetRequestTimeout(c.cfg.Timeout)

	// Colly follows redirects by default
	// Final URL is captured in OnResponse

	var rawHTML []byte
	var parseErr error
	var fetchErr error

	collector.OnResponse(func(r *colly.Response) {
		result.StatusCode = r.StatusCode
		result.FinalURL = r.Request.URL.String()
		result.ContentLength = len(r.Body)

		// Check content type
		contentType := r.Headers.Get("Content-Type")
		if !strings.Contains(contentType, "text/html") &&
			!strings.Contains(contentType, "application/xhtml") {
			// Not HTML, skip parsing
			result.Error = fmt.Sprintf("unsupported content type: %s", contentType)
			return
		}

		// Check body size
		if int64(len(r.Body)) > c.cfg.MaxBodySize {
			result.Error = "body too large"
			return
		}

		rawHTML = r.Body

		// Parse HTML
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			parseErr = fmt.Errorf("parse html: %w", err)
			return
		}

		// Extract content
		parsed := parser.Parse(doc, result.FinalURL)
		result.Title = parsed.Title
		result.Content = parsed.Content
		result.Links = parsed.Links
		result.LinksCount = len(parsed.Links)
		result.Metadata = parsed.Metadata

		// Compute hashes
		result.ContentHash = models.ComputeContentHash(r.Body)
		result.SimHash = dedup.SimHash(result.Content)
		result.RawHTML = rawHTML
	})

	collector.OnError(func(r *colly.Response, err error) {
		fetchErr = err
		if r != nil {
			result.StatusCode = r.StatusCode
			result.FinalURL = r.Request.URL.String()
		}
	})

	// Set up context cancellation
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Note: Colly doesn't support context cancellation directly
			// The request will complete but we'll return the context error
		case <-done:
		}
	}()

	start := time.Now()
	err := collector.Visit(u.Normalized)
	result.FetchDuration = time.Since(start)
	close(done)

	// Check for context cancellation
	if ctx.Err() != nil {
		result.Error = ctx.Err().Error()
		return result, ctx.Err()
	}

	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	if fetchErr != nil {
		result.Error = fetchErr.Error()
		return result, fetchErr
	}

	if parseErr != nil {
		result.Error = parseErr.Error()
		return result, parseErr
	}

	// Set final URL if not set
	if result.FinalURL == "" {
		result.FinalURL = u.Normalized
	}

	return result, nil
}

// RequiresJS always returns false for Colly crawler.
func (c *CollyCrawler) RequiresJS(url string) bool {
	return false
}

// Close releases resources.
func (c *CollyCrawler) Close() error {
	return nil
}
