package frontier

import (
	"context"
	"fmt"
	"testing"
)

func TestBloomFilter(t *testing.T) {
	// Create bloom filter (nil redis for local-only testing)
	bf, err := NewBloomFilter(nil, 10000, 10)
	if err != nil {
		t.Fatalf("NewBloomFilter() error: %v", err)
	}

	ctx := context.Background()

	// Test adding and checking
	url1 := "https://example.com/page1"
	url2 := "https://example.com/page2"
	url3 := "https://example.com/page3"

	// Initially, nothing should be found
	found, _ := bf.MightContain(ctx, url1)
	if found {
		t.Error("url1 should not be found initially")
	}

	// Add url1
	if err := bf.Add(ctx, url1); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	// url1 should be found
	found, _ = bf.MightContain(ctx, url1)
	if !found {
		t.Error("url1 should be found after adding")
	}

	// url2 should not be found (with high probability)
	found, _ = bf.MightContain(ctx, url2)
	if found {
		t.Log("Warning: False positive for url2 (possible but unlikely)")
	}

	// Add url2
	bf.Add(ctx, url2)
	found, _ = bf.MightContain(ctx, url2)
	if !found {
		t.Error("url2 should be found after adding")
	}

	// url3 still not added
	found, _ = bf.MightContain(ctx, url3)
	if found {
		t.Log("Warning: False positive for url3")
	}
}

func TestBloomFilter_Count(t *testing.T) {
	bf, _ := NewBloomFilter(nil, 10000, 10)
	ctx := context.Background()

	initialCount := bf.Count()
	if initialCount != 0 {
		t.Errorf("Initial count should be 0, got %d", initialCount)
	}

	// Add some items
	for i := 0; i < 100; i++ {
		bf.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
	}

	count := bf.Count()
	// Approximate count should be close to 100
	if count < 90 || count > 110 {
		t.Errorf("Count after 100 adds should be ~100, got %d", count)
	}
}

func TestBloomFilter_Clear(t *testing.T) {
	bf, _ := NewBloomFilter(nil, 10000, 10)
	ctx := context.Background()

	// Add some items
	bf.Add(ctx, "https://example.com/1")
	bf.Add(ctx, "https://example.com/2")

	// Verify they exist
	found, _ := bf.MightContain(ctx, "https://example.com/1")
	if !found {
		t.Error("Item should exist before clear")
	}

	// Clear
	bf.Clear()

	// Should not find after clear
	found, _ = bf.MightContain(ctx, "https://example.com/1")
	if found {
		t.Error("Item should not exist after clear")
	}

	if bf.Count() != 0 {
		t.Errorf("Count should be 0 after clear, got %d", bf.Count())
	}
}

func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	// Create a bloom filter and add many items
	bf, _ := NewBloomFilter(nil, 100000, 10)
	ctx := context.Background()

	// Add 10000 items
	added := 10000
	for i := 0; i < added; i++ {
		bf.Add(ctx, fmt.Sprintf("added-%d", i))
	}

	// Check for false positives on items never added
	falsePositives := 0
	checks := 10000
	for i := 0; i < checks; i++ {
		found, _ := bf.MightContain(ctx, fmt.Sprintf("notadded-%d", i))
		if found {
			falsePositives++
		}
	}

	fpRate := float64(falsePositives) / float64(checks)
	t.Logf("False positive rate: %.4f%% (%d/%d)", fpRate*100, falsePositives, checks)

	// With our parameters, FP rate should be < 1%
	if fpRate > 0.01 {
		t.Errorf("False positive rate too high: %.4f%%", fpRate*100)
	}
}

func BenchmarkBloomFilter_Add(b *testing.B) {
	bf, _ := NewBloomFilter(nil, 1000000, 10)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bf.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
	}
}

func BenchmarkBloomFilter_MightContain(b *testing.B) {
	bf, _ := NewBloomFilter(nil, 1000000, 10)
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 100000; i++ {
		bf.Add(ctx, fmt.Sprintf("https://example.com/page%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bf.MightContain(ctx, fmt.Sprintf("https://example.com/page%d", i%100000))
	}
}
