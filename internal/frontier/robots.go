package frontier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/temoto/robotstxt"

	"github.com/mhq-projects/web-crawler/internal/config"
	"github.com/mhq-projects/web-crawler/internal/storage"
	"github.com/mhq-projects/web-crawler/pkg/models"
)

// RobotsManager handles robots.txt caching and parsing.
type RobotsManager struct {
	redis     *storage.RedisClient
	client    *http.Client
	userAgent string
	ttl       time.Duration
}

// NewRobotsManager creates a new robots manager.
func NewRobotsManager(redis *storage.RedisClient, cfg config.CrawlerConfig, ttl time.Duration) *RobotsManager {
	return &RobotsManager{
		redis:     redis,
		client:    &http.Client{Timeout: 10 * time.Second},
		userAgent: cfg.UserAgent,
		ttl:       ttl,
	}
}

// CanFetch checks if a URL can be fetched according to robots.txt.
func (r *RobotsManager) CanFetch(ctx context.Context, urlStr string) (bool, error) {
	host, err := models.ExtractHost(urlStr)
	if err != nil {
		return false, err
	}

	group, err := r.getGroup(ctx, host)
	if err != nil {
		// If we can't get robots.txt, allow crawling
		return true, nil
	}

	return group.Test(urlStr), nil
}

// GetCrawlDelay returns the crawl delay from robots.txt.
func (r *RobotsManager) GetCrawlDelay(ctx context.Context, host string) (time.Duration, error) {
	group, err := r.getGroup(ctx, host)
	if err != nil {
		return 0, err
	}

	delay := group.CrawlDelay
	if delay > 0 {
		return delay, nil
	}

	return 0, nil
}

// getGroup fetches or retrieves cached robots.txt rules.
func (r *RobotsManager) getGroup(ctx context.Context, host string) (*robotstxt.Group, error) {
	key := storage.RobotsKey(host)

	// Try to get from cache
	data, err := r.redis.HGetAll(ctx, key)
	if err == nil && len(data) > 0 {
		if expiresStr, ok := data["expires"]; ok {
			var expires int64
			fmt.Sscanf(expiresStr, "%d", &expires)
			if time.Now().Before(time.UnixMilli(expires)) {
				// Cache hit
				var robotsData models.RobotsData
				if rulesStr, ok := data["rules"]; ok {
					if err := json.Unmarshal([]byte(rulesStr), &robotsData); err == nil {
						robots, err := robotstxt.FromBytes(robotsData.Rules)
						if err == nil {
							return robots.FindGroup(r.userAgent), nil
						}
					}
				}
			}
		}
	}

	// Cache miss, fetch robots.txt
	robotsURL := fmt.Sprintf("https://%s/robots.txt", host)
	resp, err := r.client.Get(robotsURL)
	if err != nil {
		// Try HTTP if HTTPS fails
		robotsURL = fmt.Sprintf("http://%s/robots.txt", host)
		resp, err = r.client.Get(robotsURL)
		if err != nil {
			return nil, fmt.Errorf("fetch robots.txt: %w", err)
		}
	}
	defer resp.Body.Close()

	var rules []byte
	if resp.StatusCode == http.StatusOK {
		rules, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read robots.txt: %w", err)
		}
	} else {
		// No robots.txt, allow all
		rules = []byte("")
	}

	// Parse rules
	robots, err := robotstxt.FromBytes(rules)
	if err != nil {
		return nil, fmt.Errorf("parse robots.txt: %w", err)
	}

	group := robots.FindGroup(r.userAgent)

	// Cache the rules
	robotsData := models.RobotsData{
		Host:      host,
		Rules:     rules,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(r.ttl),
	}
	if group.CrawlDelay > 0 {
		robotsData.CrawlDelay = int(group.CrawlDelay.Seconds())
	}

	rulesJSON, _ := json.Marshal(robotsData)
	r.redis.HSet(ctx, key,
		"rules", string(rulesJSON),
		"expires", robotsData.ExpiresAt.UnixMilli(),
	)
	r.redis.Expire(ctx, key, r.ttl)

	return group, nil
}

// InvalidateCache removes cached robots.txt for a host.
func (r *RobotsManager) InvalidateCache(ctx context.Context, host string) error {
	key := storage.RobotsKey(host)
	return r.redis.Del(ctx, key)
}

// PreFetch fetches robots.txt for multiple hosts in parallel.
func (r *RobotsManager) PreFetch(ctx context.Context, hosts []string) error {
	for _, host := range hosts {
		_, _ = r.getGroup(ctx, host)
	}
	return nil
}

// IsAllowed checks if crawling is allowed and returns relevant data.
func (r *RobotsManager) IsAllowed(ctx context.Context, urlStr string) (allowed bool, crawlDelay time.Duration, err error) {
	host, err := models.ExtractHost(urlStr)
	if err != nil {
		return false, 0, err
	}

	group, err := r.getGroup(ctx, host)
	if err != nil {
		// If we can't get robots.txt, allow with default delay
		return true, 0, nil
	}

	allowed = group.Test(urlStr)
	crawlDelay = group.CrawlDelay

	return allowed, crawlDelay, nil
}
