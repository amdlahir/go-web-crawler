package frontier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/amdlahir/go-web-crawler/internal/config"
	"github.com/amdlahir/go-web-crawler/internal/storage"
	"github.com/amdlahir/go-web-crawler/pkg/models"
)

var (
	// ErrNoURLs is returned when no URLs are available.
	ErrNoURLs = errors.New("no urls available")
)

// Frontier manages the URL queue with politeness and prioritization.
type Frontier struct {
	redis        *storage.RedisClient
	bloom        *BloomFilter
	politeness   *PolitenessManager
	cfg          config.FrontierConfig
	mu           sync.RWMutex
	activeHosts  map[string]time.Time
}

// New creates a new Frontier.
func New(redisClient *storage.RedisClient, cfg config.FrontierConfig) (*Frontier, error) {
	bloom, err := NewBloomFilter(redisClient, cfg.BloomSize, cfg.BloomHashCount)
	if err != nil {
		return nil, fmt.Errorf("create bloom filter: %w", err)
	}

	politeness := NewPolitenessManager(redisClient, cfg.DefaultDelay, cfg.MaxDelay)

	return &Frontier{
		redis:       redisClient,
		bloom:       bloom,
		politeness:  politeness,
		cfg:         cfg,
		activeHosts: make(map[string]time.Time),
	}, nil
}

// AddURLs adds URLs to the frontier.
func (f *Frontier) AddURLs(ctx context.Context, urls []*models.URL) (int, error) {
	added := 0

	for _, u := range urls {
		// Check if already seen
		seen, err := f.IsSeen(ctx, u.Normalized)
		if err != nil {
			return added, fmt.Errorf("check seen: %w", err)
		}
		if seen {
			continue
		}

		// Mark as seen
		if err := f.MarkSeen(ctx, u.Normalized); err != nil {
			return added, fmt.Errorf("mark seen: %w", err)
		}

		// Add to host queue
		if err := f.addToHostQueue(ctx, u); err != nil {
			return added, fmt.Errorf("add to host queue: %w", err)
		}

		added++
	}

	return added, nil
}

// addToHostQueue adds a URL to its host queue and updates priority set.
func (f *Frontier) addToHostQueue(ctx context.Context, u *models.URL) error {
	data, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("marshal url: %w", err)
	}

	// Add to host queue
	queueKey := storage.HostQueueKey(u.Host)
	if err := f.redis.LPush(ctx, queueKey, string(data)); err != nil {
		return fmt.Errorf("lpush: %w", err)
	}

	// Add host to priority set
	priority := float64(u.Priority)
	if err := f.redis.ZAdd(ctx, storage.KeyFrontierPriority, redis.Z{
		Score:  priority,
		Member: u.Host,
	}); err != nil {
		return fmt.Errorf("zadd: %w", err)
	}

	return nil
}

// NextURL returns the next URL to crawl, respecting politeness.
func (f *Frontier) NextURL(ctx context.Context) (*models.URL, error) {
	// Get hosts ordered by priority
	hosts, err := f.redis.ZRangeWithScores(ctx, storage.KeyFrontierPriority, 0, 100)
	if err != nil {
		return nil, fmt.Errorf("zrange: %w", err)
	}

	if len(hosts) == 0 {
		return nil, ErrNoURLs
	}

	now := time.Now()

	// Find a host that's ready to crawl
	for _, z := range hosts {
		host := z.Member.(string)

		// Check politeness
		ready, err := f.politeness.IsReady(ctx, host)
		if err != nil {
			continue
		}
		if !ready {
			continue
		}

		// Try to get URL from host queue
		queueKey := storage.HostQueueKey(host)
		data, err := f.redis.RPop(ctx, queueKey)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				// Queue empty, remove from priority set
				f.redis.ZRem(ctx, storage.KeyFrontierPriority, host)
				continue
			}
			return nil, fmt.Errorf("rpop: %w", err)
		}

		// Update last fetch time
		if err := f.politeness.Touch(ctx, host, now); err != nil {
			// Log but don't fail
		}

		// Check if queue is now empty
		queueLen, _ := f.redis.LLen(ctx, queueKey)
		if queueLen == 0 {
			f.redis.ZRem(ctx, storage.KeyFrontierPriority, host)
		}

		var u models.URL
		if err := json.Unmarshal([]byte(data), &u); err != nil {
			return nil, fmt.Errorf("unmarshal url: %w", err)
		}

		return &u, nil
	}

	return nil, ErrNoURLs
}

// IsSeen checks if a URL has been seen.
func (f *Frontier) IsSeen(ctx context.Context, url string) (bool, error) {
	normalized, err := models.NormalizeURL(url)
	if err != nil {
		return false, err
	}

	// Fast path: check bloom filter
	possibly, err := f.bloom.MightContain(ctx, normalized)
	if err != nil {
		return false, err
	}
	if !possibly {
		return false, nil
	}

	// Slow path: check Redis set
	hash := hashURL(normalized)
	return f.redis.SIsMember(ctx, storage.KeySeenURLs, hash)
}

// MarkSeen marks a URL as seen.
func (f *Frontier) MarkSeen(ctx context.Context, url string) error {
	normalized, err := models.NormalizeURL(url)
	if err != nil {
		return err
	}

	// Add to bloom filter
	if err := f.bloom.Add(ctx, normalized); err != nil {
		return err
	}

	// Add to Redis set
	hash := hashURL(normalized)
	return f.redis.SAdd(ctx, storage.KeySeenURLs, hash)
}

// Complete marks a URL crawl as completed.
func (f *Frontier) Complete(ctx context.Context, u *models.URL, result *models.CrawlResult) error {
	// Increment completed counter
	if _, err := f.redis.Incr(ctx, storage.KeyMetricsPagesTotal); err != nil {
		return err
	}
	return nil
}

// Fail marks a URL crawl as failed.
func (f *Frontier) Fail(ctx context.Context, u *models.URL, crawlErr error) error {
	// Increment error counter
	if _, err := f.redis.Incr(ctx, storage.KeyMetricsErrorsTotal); err != nil {
		return err
	}

	// Optionally re-queue for retry (based on error type)
	// For now, just mark as failed
	return nil
}

// Stats returns frontier statistics.
func (f *Frontier) Stats(ctx context.Context) (*models.FrontierStats, error) {
	stats := &models.FrontierStats{
		HostQueueCounts: make(map[string]int64),
	}

	// Get completed count
	completed, err := f.redis.Get(ctx, storage.KeyMetricsPagesTotal)
	if err == nil {
		fmt.Sscanf(completed, "%d", &stats.TotalCompleted)
	}

	// Get failed count
	failed, err := f.redis.Get(ctx, storage.KeyMetricsErrorsTotal)
	if err == nil {
		fmt.Sscanf(failed, "%d", &stats.TotalFailed)
	}

	// Get host queues
	hosts, err := f.redis.ZRangeWithScores(ctx, storage.KeyFrontierPriority, 0, -1)
	if err != nil {
		return stats, err
	}

	for _, z := range hosts {
		host := z.Member.(string)
		queueKey := storage.HostQueueKey(host)
		count, err := f.redis.LLen(ctx, queueKey)
		if err != nil {
			continue
		}
		stats.HostQueueCounts[host] = count
		stats.TotalPending += count
	}

	return stats, nil
}

// hashURL returns a hash of the URL for set storage.
func hashURL(url string) string {
	u, _ := models.NewURL(url, 0, 0, "")
	if u != nil {
		return u.Hash()
	}
	return url
}
