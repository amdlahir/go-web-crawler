package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mhq-projects/web-crawler/internal/config"
)

// RedisClient wraps the Redis client with crawler-specific operations.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client.
func NewRedisClient(cfg config.RedisConfig) (*RedisClient, error) {
	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	if cfg.Password != "" {
		opt.Password = cfg.Password
	}
	opt.DB = cfg.DB
	opt.PoolSize = cfg.PoolSize

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Client returns the underlying Redis client.
func (r *RedisClient) Client() *redis.Client {
	return r.client
}

// Close closes the Redis connection.
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Key patterns for the crawler.
const (
	KeyFrontierPriority   = "frontier:priority"
	KeyFrontierHostPrefix = "frontier:host:"
	KeySeenURLs           = "seen:urls"
	KeySeenBloom          = "seen:bloom"
	KeyRobotsPrefix       = "robots:"
	KeyPolitenessPrefix   = "politeness:"
	KeyWorkerActivePrefix = "worker:active:"
	KeyWorkerStatsPrefix  = "worker:stats:"
	KeyMetricsPagesTotal  = "metrics:pages_total"
	KeyMetricsErrorsTotal = "metrics:errors_total"
	KeyMetricsQueueDepth  = "metrics:queue_depth"
	KeyContentHashPrefix  = "content:hash:"
)

// HostQueueKey returns the key for a host's URL queue.
func HostQueueKey(host string) string {
	return KeyFrontierHostPrefix + host
}

// RobotsKey returns the key for a host's robots.txt cache.
func RobotsKey(host string) string {
	return KeyRobotsPrefix + host
}

// PolitenessKey returns the key for a host's politeness data.
func PolitenessKey(host string) string {
	return KeyPolitenessPrefix + host
}

// WorkerActiveKey returns the key for a worker's current URL.
func WorkerActiveKey(workerID string) string {
	return KeyWorkerActivePrefix + workerID
}

// WorkerStatsKey returns the key for a worker's stats.
func WorkerStatsKey(workerID string) string {
	return KeyWorkerStatsPrefix + workerID
}

// ContentHashKey returns the key for content hash lookup.
func ContentHashKey(hash string) string {
	return KeyContentHashPrefix + hash
}

// SetWithTTL sets a key with TTL.
func (r *RedisClient) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Get retrieves a string value.
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

// Incr increments a counter.
func (r *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// LPush pushes to the left of a list.
func (r *RedisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return r.client.LPush(ctx, key, values...).Err()
}

// RPop pops from the right of a list.
func (r *RedisClient) RPop(ctx context.Context, key string) (string, error) {
	return r.client.RPop(ctx, key).Result()
}

// LLen returns the length of a list.
func (r *RedisClient) LLen(ctx context.Context, key string) (int64, error) {
	return r.client.LLen(ctx, key).Result()
}

// SAdd adds to a set.
func (r *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SAdd(ctx, key, members...).Err()
}

// SIsMember checks set membership.
func (r *RedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.client.SIsMember(ctx, key, member).Result()
}

// ZAdd adds to a sorted set.
func (r *RedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return r.client.ZAdd(ctx, key, members...).Err()
}

// ZRangeWithScores returns sorted set members with scores.
func (r *RedisClient) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRangeWithScores(ctx, key, start, stop).Result()
}

// ZRem removes from a sorted set.
func (r *RedisClient) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return r.client.ZRem(ctx, key, members...).Err()
}

// HSet sets hash fields.
func (r *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.client.HSet(ctx, key, values...).Err()
}

// HGet gets a hash field.
func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, key, field).Result()
}

// HGetAll gets all hash fields.
func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

// Keys returns keys matching a pattern.
func (r *RedisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return r.client.Keys(ctx, pattern).Result()
}

// Expire sets a key's TTL.
func (r *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}

// Del deletes keys.
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Exists checks if keys exist.
func (r *RedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	return r.client.Exists(ctx, keys...).Result()
}
