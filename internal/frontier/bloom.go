package frontier

import (
	"context"
	"hash/fnv"
	"sync"

	"github.com/bits-and-blooms/bloom/v3"

	"github.com/mhq-projects/web-crawler/internal/storage"
)

// BloomFilter provides probabilistic URL deduplication.
type BloomFilter struct {
	redis     *storage.RedisClient
	local     *bloom.BloomFilter
	mu        sync.RWMutex
	size      uint
	hashCount uint
}

// NewBloomFilter creates a new bloom filter.
func NewBloomFilter(redis *storage.RedisClient, size, hashCount uint) (*BloomFilter, error) {
	// Create local bloom filter
	local := bloom.NewWithEstimates(size, 0.001)

	return &BloomFilter{
		redis:     redis,
		local:     local,
		size:      size,
		hashCount: hashCount,
	}, nil
}

// Add adds an item to the bloom filter.
func (b *BloomFilter) Add(ctx context.Context, item string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add to local filter
	b.local.AddString(item)

	return nil
}

// MightContain checks if an item might be in the filter.
func (b *BloomFilter) MightContain(ctx context.Context, item string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.local.TestString(item), nil
}

// Count returns an estimate of items in the filter.
func (b *BloomFilter) Count() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.local.ApproximatedSize()
}

// Clear resets the bloom filter.
func (b *BloomFilter) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.local.ClearAll()
}

// hash returns a hash of the string.
func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
