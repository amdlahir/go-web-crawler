package dedup

import (
	"testing"
)

func TestSimHash(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "empty content",
			content: "",
		},
		{
			name:    "short content",
			content: "hello world",
		},
		{
			name:    "longer content",
			content: "The quick brown fox jumps over the lazy dog. This is a test sentence for simhash.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := SimHash(tt.content)
			// Same content should produce same hash
			hash2 := SimHash(tt.content)
			if hash != hash2 {
				t.Errorf("SimHash() not deterministic: %d != %d", hash, hash2)
			}
		})
	}
}

func TestSimHash_SimilarContent(t *testing.T) {
	content1 := "The quick brown fox jumps over the lazy dog"
	content2 := "The quick brown fox jumps over the lazy cat"
	content3 := "Completely different content about programming"

	hash1 := SimHash(content1)
	hash2 := SimHash(content2)
	hash3 := SimHash(content3)

	dist12 := HammingDistance(hash1, hash2)
	dist13 := HammingDistance(hash1, hash3)

	// Similar content should have smaller hamming distance
	if dist12 >= dist13 {
		t.Errorf("Similar content should have smaller distance: dist(1,2)=%d >= dist(1,3)=%d", dist12, dist13)
	}

	t.Logf("Distance between similar: %d", dist12)
	t.Logf("Distance between different: %d", dist13)
}

func TestHammingDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        uint64
		b        uint64
		expected int
	}{
		{
			name:     "same values",
			a:        0xFFFFFFFFFFFFFFFF,
			b:        0xFFFFFFFFFFFFFFFF,
			expected: 0,
		},
		{
			name:     "one bit difference",
			a:        0x0000000000000000,
			b:        0x0000000000000001,
			expected: 1,
		},
		{
			name:     "all bits different",
			a:        0x0000000000000000,
			b:        0xFFFFFFFFFFFFFFFF,
			expected: 64,
		},
		{
			name:     "half bits different",
			a:        0x00000000FFFFFFFF,
			b:        0xFFFFFFFF00000000,
			expected: 64,
		},
		{
			name:     "alternating bits",
			a:        0xAAAAAAAAAAAAAAAA,
			b:        0x5555555555555555,
			expected: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HammingDistance(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("HammingDistance(%x, %x) = %d, want %d", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestIsSimilar(t *testing.T) {
	tests := []struct {
		name      string
		a         uint64
		b         uint64
		threshold int
		expected  bool
	}{
		{
			name:      "identical",
			a:         0x1234567890ABCDEF,
			b:         0x1234567890ABCDEF,
			threshold: 3,
			expected:  true,
		},
		{
			name:      "within threshold",
			a:         0x0000000000000000,
			b:         0x0000000000000007, // 3 bits different
			threshold: 3,
			expected:  true,
		},
		{
			name:      "above threshold",
			a:         0x0000000000000000,
			b:         0x000000000000000F, // 4 bits different
			threshold: 3,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSimilar(tt.a, tt.b, tt.threshold)
			if got != tt.expected {
				t.Errorf("IsSimilar(%x, %x, %d) = %v, want %v", tt.a, tt.b, tt.threshold, got, tt.expected)
			}
		})
	}
}

func TestShingle(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []string
		n        int
		expected []string
	}{
		{
			name:     "3-gram shingles",
			tokens:   []string{"the", "quick", "brown", "fox"},
			n:        3,
			expected: []string{"the quick brown", "quick brown fox"},
		},
		{
			name:     "2-gram shingles",
			tokens:   []string{"hello", "world", "test"},
			n:        2,
			expected: []string{"hello world", "world test"},
		},
		{
			name:     "tokens less than n",
			tokens:   []string{"one", "two"},
			n:        3,
			expected: nil,
		},
		{
			name:     "empty tokens",
			tokens:   []string{},
			n:        2,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Shingle(tt.tokens, tt.n)
			if len(got) != len(tt.expected) {
				t.Errorf("Shingle() returned %d shingles, want %d", len(got), len(tt.expected))
				return
			}
			for i, s := range got {
				if s != tt.expected[i] {
					t.Errorf("Shingle()[%d] = %q, want %q", i, s, tt.expected[i])
				}
			}
		})
	}
}

func BenchmarkSimHash(b *testing.B) {
	content := `Lorem ipsum dolor sit amet, consectetur adipiscing elit.
	Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
	Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SimHash(content)
	}
}

func BenchmarkHammingDistance(b *testing.B) {
	a := uint64(0x123456789ABCDEF0)
	c := uint64(0xFEDCBA9876543210)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HammingDistance(a, c)
	}
}
