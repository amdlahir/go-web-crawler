package dedup

import (
	"hash/fnv"
	"strings"
	"unicode"
)

// SimHash computes a locality-sensitive hash for text content.
// Similar content will produce similar hashes.
func SimHash(content string) uint64 {
	// Tokenize content
	tokens := tokenize(content)
	if len(tokens) == 0 {
		return 0
	}

	// Build feature vector
	var v [64]int

	for _, token := range tokens {
		h := hashToken(token)
		for i := 0; i < 64; i++ {
			bit := (h >> i) & 1
			if bit == 1 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}

	// Build fingerprint
	var fingerprint uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			fingerprint |= 1 << i
		}
	}

	return fingerprint
}

// HammingDistance computes the hamming distance between two simhashes.
func HammingDistance(a, b uint64) int {
	xor := a ^ b
	count := 0
	for xor != 0 {
		count++
		xor &= xor - 1
	}
	return count
}

// IsSimilar checks if two simhashes are similar (within threshold).
func IsSimilar(a, b uint64, threshold int) bool {
	return HammingDistance(a, b) <= threshold
}

// DefaultSimilarityThreshold is the default hamming distance threshold.
// Content with distance <= 3 is considered near-duplicate.
const DefaultSimilarityThreshold = 3

// tokenize splits content into tokens.
func tokenize(content string) []string {
	// Normalize: lowercase and split on non-alphanumeric
	content = strings.ToLower(content)

	var tokens []string
	var current strings.Builder

	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			token := current.String()
			// Skip very short tokens
			if len(token) > 2 {
				tokens = append(tokens, token)
			}
			current.Reset()
		}
	}

	// Don't forget last token
	if current.Len() > 2 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// hashToken hashes a token using FNV-1a.
func hashToken(token string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(token))
	return h.Sum64()
}

// Shingle creates n-gram shingles from tokens.
func Shingle(tokens []string, n int) []string {
	if len(tokens) < n {
		return nil
	}

	shingles := make([]string, 0, len(tokens)-n+1)
	for i := 0; i <= len(tokens)-n; i++ {
		shingle := strings.Join(tokens[i:i+n], " ")
		shingles = append(shingles, shingle)
	}

	return shingles
}

// SimHashWithShingles computes simhash using word shingles.
func SimHashWithShingles(content string, shingleSize int) uint64 {
	tokens := tokenize(content)
	if len(tokens) < shingleSize {
		return SimHash(content)
	}

	shingles := Shingle(tokens, shingleSize)

	var v [64]int
	for _, shingle := range shingles {
		h := hashToken(shingle)
		for i := 0; i < 64; i++ {
			bit := (h >> i) & 1
			if bit == 1 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}

	var fingerprint uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			fingerprint |= 1 << i
		}
	}

	return fingerprint
}
