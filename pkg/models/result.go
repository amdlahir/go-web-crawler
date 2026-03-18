package models

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// CrawlResult represents the result of crawling a URL.
type CrawlResult struct {
	URL           string            `json:"url"`
	FinalURL      string            `json:"final_url"`
	StatusCode    int               `json:"status_code"`
	Title         string            `json:"title"`
	Content       string            `json:"content"`
	RawHTML       []byte            `json:"-"`
	ContentHash   string            `json:"content_hash"`
	SimHash       uint64            `json:"simhash"`
	Links         []string          `json:"links"`
	LinksCount    int               `json:"links_count"`
	CrawledAt     time.Time         `json:"crawled_at"`
	FetchDuration time.Duration     `json:"fetch_duration"`
	ContentLength int               `json:"content_length"`
	Metadata      map[string]string `json:"metadata"`
	Error         string            `json:"error,omitempty"`
	Depth         int               `json:"depth"`
	Domain        string            `json:"domain"`
}

// Success returns true if crawl completed without error.
func (r *CrawlResult) Success() bool {
	return r.Error == "" && r.StatusCode >= 200 && r.StatusCode < 400
}

// ComputeContentHash calculates SHA256 hash of raw HTML.
func ComputeContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// ParsedPage holds extracted data from HTML.
type ParsedPage struct {
	Title       string
	Content     string
	Description string
	Links       []string
	Metadata    map[string]string
}

// SearchResult represents a search hit from OpenSearch.
type SearchResult struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	Domain    string    `json:"domain"`
	CrawledAt time.Time `json:"crawled_at"`
	Score     float64   `json:"score"`
}

// SearchOpts configures search behavior.
type SearchOpts struct {
	Query   string
	Domain  string
	From    int
	Size    int
	SortBy  string
	SortDir string
}

// FrontierStats holds queue statistics.
type FrontierStats struct {
	TotalPending    int64            `json:"total_pending"`
	TotalCompleted  int64            `json:"total_completed"`
	TotalFailed     int64            `json:"total_failed"`
	HostQueueCounts map[string]int64 `json:"host_queue_counts"`
	PagesPerSecond  float64          `json:"pages_per_second"`
}

// WorkerStats holds worker statistics.
type WorkerStats struct {
	WorkerID      string    `json:"worker_id"`
	PagesTotal    int64     `json:"pages_total"`
	PagesFailed   int64     `json:"pages_failed"`
	CurrentURL    string    `json:"current_url,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	LastActiveAt  time.Time `json:"last_active_at"`
	AvgFetchTime  float64   `json:"avg_fetch_time_ms"`
}

// RobotsData holds cached robots.txt data.
type RobotsData struct {
	Host      string    `json:"host"`
	Rules     []byte    `json:"rules"`
	CrawlDelay int      `json:"crawl_delay"`
	CachedAt  time.Time `json:"cached_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
