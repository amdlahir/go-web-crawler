package frontier

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mhq-projects/web-crawler/internal/storage"
)

// PolitenessManager enforces per-host rate limiting.
type PolitenessManager struct {
	redis        *storage.RedisClient
	defaultDelay time.Duration
	maxDelay     time.Duration
}

// NewPolitenessManager creates a new politeness manager.
func NewPolitenessManager(redis *storage.RedisClient, defaultDelay, maxDelay time.Duration) *PolitenessManager {
	return &PolitenessManager{
		redis:        redis,
		defaultDelay: defaultDelay,
		maxDelay:     maxDelay,
	}
}

// IsReady checks if a host is ready to be crawled.
func (p *PolitenessManager) IsReady(ctx context.Context, host string) (bool, error) {
	key := storage.PolitenessKey(host)

	lastFetchStr, err := p.redis.HGet(ctx, key, "last_fetch")
	if err != nil {
		if err == redis.Nil {
			return true, nil // Never crawled
		}
		return false, fmt.Errorf("hget last_fetch: %w", err)
	}

	lastFetch, err := strconv.ParseInt(lastFetchStr, 10, 64)
	if err != nil {
		return true, nil
	}

	delay := p.getDelay(ctx, host)
	nextAllowed := time.UnixMilli(lastFetch).Add(delay)

	return time.Now().After(nextAllowed), nil
}

// Touch updates the last fetch time for a host.
func (p *PolitenessManager) Touch(ctx context.Context, host string, t time.Time) error {
	key := storage.PolitenessKey(host)
	return p.redis.HSet(ctx, key, "last_fetch", t.UnixMilli())
}

// SetDelay sets a custom delay for a host (from robots.txt).
func (p *PolitenessManager) SetDelay(ctx context.Context, host string, delay time.Duration) error {
	if delay > p.maxDelay {
		delay = p.maxDelay
	}
	key := storage.PolitenessKey(host)
	return p.redis.HSet(ctx, key, "delay_ms", delay.Milliseconds())
}

// getDelay returns the delay for a host.
func (p *PolitenessManager) getDelay(ctx context.Context, host string) time.Duration {
	key := storage.PolitenessKey(host)

	delayStr, err := p.redis.HGet(ctx, key, "delay_ms")
	if err != nil || delayStr == "" {
		return p.defaultDelay
	}

	delayMs, err := strconv.ParseInt(delayStr, 10, 64)
	if err != nil {
		return p.defaultDelay
	}

	return time.Duration(delayMs) * time.Millisecond
}

// RecordError records a crawl error and adjusts delay.
func (p *PolitenessManager) RecordError(ctx context.Context, host string, statusCode int) error {
	if statusCode == 429 || statusCode >= 500 {
		// Increase delay on rate limit or server errors
		currentDelay := p.getDelay(ctx, host)
		newDelay := currentDelay * 2
		if newDelay > p.maxDelay {
			newDelay = p.maxDelay
		}
		return p.SetDelay(ctx, host, newDelay)
	}
	return nil
}

// ResetDelay resets a host's delay to default.
func (p *PolitenessManager) ResetDelay(ctx context.Context, host string) error {
	key := storage.PolitenessKey(host)
	return p.redis.HSet(ctx, key, "delay_ms", p.defaultDelay.Milliseconds())
}

// GetHostStats returns stats for a host.
func (p *PolitenessManager) GetHostStats(ctx context.Context, host string) (map[string]string, error) {
	key := storage.PolitenessKey(host)
	return p.redis.HGetAll(ctx, key)
}
