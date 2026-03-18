package dedup

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/mhq-projects/web-crawler/internal/storage"
)

// ContentDedup handles content-level deduplication using SimHash.
type ContentDedup struct {
	redis     *storage.RedisClient
	threshold int
}

// NewContentDedup creates a new content deduplication service.
func NewContentDedup(redis *storage.RedisClient, threshold int) *ContentDedup {
	if threshold <= 0 {
		threshold = DefaultSimilarityThreshold
	}
	return &ContentDedup{
		redis:     redis,
		threshold: threshold,
	}
}

// Key for storing content hashes.
const keyContentHashes = "content:simhashes"

// CheckAndStore checks if content is a near-duplicate and stores its hash.
// Returns true if content is a duplicate, false otherwise.
func (c *ContentDedup) CheckAndStore(ctx context.Context, contentHash string, simhash uint64) (bool, error) {
	// First check exact content hash
	exists, err := c.redis.SIsMember(ctx, storage.KeyContentHashPrefix+"exact", contentHash)
	if err != nil {
		return false, fmt.Errorf("check exact hash: %w", err)
	}
	if exists {
		return true, nil // Exact duplicate
	}

	// Check for near-duplicates using simhash
	isDupe, err := c.findNearDuplicate(ctx, simhash)
	if err != nil {
		return false, fmt.Errorf("find near duplicate: %w", err)
	}
	if isDupe {
		return true, nil
	}

	// Not a duplicate, store hashes
	if err := c.store(ctx, contentHash, simhash); err != nil {
		return false, fmt.Errorf("store hashes: %w", err)
	}

	return false, nil
}

// findNearDuplicate checks if a simhash has a near-duplicate in storage.
func (c *ContentDedup) findNearDuplicate(ctx context.Context, simhash uint64) (bool, error) {
	// Get all stored simhashes
	// In production, use a more efficient approach (e.g., LSH or bucketing)
	hashes, err := c.redis.Client().SMembers(ctx, keyContentHashes).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}

	for _, hashStr := range hashes {
		stored, err := strconv.ParseUint(hashStr, 10, 64)
		if err != nil {
			continue
		}

		if IsSimilar(simhash, stored, c.threshold) {
			return true, nil
		}
	}

	return false, nil
}

// store saves content and simhash.
func (c *ContentDedup) store(ctx context.Context, contentHash string, simhash uint64) error {
	// Store exact hash
	if err := c.redis.SAdd(ctx, storage.KeyContentHashPrefix+"exact", contentHash); err != nil {
		return err
	}

	// Store simhash
	return c.redis.SAdd(ctx, keyContentHashes, strconv.FormatUint(simhash, 10))
}

// IsExactDuplicate checks if content hash exists.
func (c *ContentDedup) IsExactDuplicate(ctx context.Context, contentHash string) (bool, error) {
	return c.redis.SIsMember(ctx, storage.KeyContentHashPrefix+"exact", contentHash)
}

// GetStats returns dedup statistics.
func (c *ContentDedup) GetStats(ctx context.Context) (exactCount, simhashCount int64, err error) {
	client := c.redis.Client()

	exactCount, err = client.SCard(ctx, storage.KeyContentHashPrefix+"exact").Result()
	if err != nil && err != redis.Nil {
		return 0, 0, err
	}

	simhashCount, err = client.SCard(ctx, keyContentHashes).Result()
	if err != nil && err != redis.Nil {
		return 0, 0, err
	}

	return exactCount, simhashCount, nil
}
