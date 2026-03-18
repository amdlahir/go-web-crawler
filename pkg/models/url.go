package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// URL represents a URL to be crawled.
type URL struct {
	Raw          string            `json:"raw"`
	Normalized   string            `json:"normalized"`
	Host         string            `json:"host"`
	Depth        int               `json:"depth"`
	Priority     int               `json:"priority"`
	DiscoveredAt time.Time         `json:"discovered_at"`
	SourceURL    string            `json:"source_url,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Hash returns SHA256 hash of normalized URL.
func (u *URL) Hash() string {
	h := sha256.Sum256([]byte(u.Normalized))
	return hex.EncodeToString(h[:])
}

// trackingParams are common tracking parameters to strip.
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"fbclid":       true,
	"gclid":        true,
	"ref":          true,
	"source":       true,
}

// sessionPatterns match session-like parameter names.
var sessionPattern = regexp.MustCompile(`(?i)(session|sid|jsession|phpsess)`)

// ErrUnsupportedScheme is returned for non-HTTP(S) URLs.
var ErrUnsupportedScheme = errors.New("unsupported scheme: only http and https are allowed")

// ErrEmptyHost is returned when URL has no host.
var ErrEmptyHost = errors.New("empty host")

// NormalizeURL normalizes a URL for deduplication.
func NormalizeURL(rawURL string) (string, error) {
	// Trim whitespace
	rawURL = strings.TrimSpace(rawURL)

	// Handle protocol-relative URLs
	if strings.HasPrefix(rawURL, "//") {
		rawURL = "https:" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Lowercase scheme and host
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	// Reject non-HTTP schemes
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrUnsupportedScheme
	}

	// Reject empty host
	if parsed.Host == "" {
		return "", ErrEmptyHost
	}

	// Strip userinfo (credentials)
	parsed.User = nil

	// Remove default ports
	if (parsed.Scheme == "http" && strings.HasSuffix(parsed.Host, ":80")) ||
		(parsed.Scheme == "https" && strings.HasSuffix(parsed.Host, ":443")) {
		parsed.Host = strings.TrimSuffix(parsed.Host, ":80")
		parsed.Host = strings.TrimSuffix(parsed.Host, ":443")
	}

	// Remove fragment
	parsed.Fragment = ""

	// Normalize path
	if parsed.Path == "" {
		parsed.Path = "/"
	} else {
		// Resolve dot segments (/../ and /./)
		parsed.Path = resolveDotSegments(parsed.Path)
		// Collapse duplicate slashes
		parsed.Path = collapseSlashes(parsed.Path)
	}

	// Remove trailing slash except for root
	if len(parsed.Path) > 1 && strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	// Filter and sort query parameters
	query := parsed.Query()
	filtered := url.Values{}
	for key, values := range query {
		// Skip tracking params
		if trackingParams[strings.ToLower(key)] {
			continue
		}
		// Skip session-like params
		if sessionPattern.MatchString(key) {
			continue
		}
		// Sort values within each key for consistency
		sortedValues := make([]string, len(values))
		copy(sortedValues, values)
		sort.Strings(sortedValues)
		filtered[key] = sortedValues
	}

	// Sort keys for consistent ordering
	var keys []string
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, k := range keys {
		for _, v := range filtered[k] {
			queryParts = append(queryParts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	parsed.RawQuery = strings.Join(queryParts, "&")

	return parsed.String(), nil
}

// resolveDotSegments resolves . and .. segments in a path per RFC 3986.
func resolveDotSegments(path string) string {
	if path == "" {
		return path
	}

	// Track if path started with /
	absolute := strings.HasPrefix(path, "/")

	segments := strings.Split(path, "/")
	var result []string

	for _, seg := range segments {
		switch seg {
		case ".":
			// Skip current directory references
			continue
		case "..":
			// Go up one directory
			if len(result) > 0 && result[len(result)-1] != "" {
				result = result[:len(result)-1]
			}
		default:
			result = append(result, seg)
		}
	}

	resolved := strings.Join(result, "/")

	// Preserve absolute path prefix
	if absolute && !strings.HasPrefix(resolved, "/") {
		resolved = "/" + resolved
	}

	if resolved == "" {
		return "/"
	}

	return resolved
}

// collapseSlashes replaces multiple consecutive slashes with a single slash.
func collapseSlashes(path string) string {
	var result strings.Builder
	prevSlash := false

	for _, c := range path {
		if c == '/' {
			if !prevSlash {
				result.WriteRune(c)
			}
			prevSlash = true
		} else {
			result.WriteRune(c)
			prevSlash = false
		}
	}

	return result.String()
}

// NewURL creates a new URL from a raw string.
func NewURL(rawURL string, depth int, priority int, sourceURL string) (*URL, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, err
	}

	return &URL{
		Raw:          rawURL,
		Normalized:   normalized,
		Host:         parsed.Host,
		Depth:        depth,
		Priority:     priority,
		DiscoveredAt: time.Now(),
		SourceURL:    sourceURL,
		Metadata:     make(map[string]string),
	}, nil
}

// ExtractHost returns the host from a URL string.
func ExtractHost(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return strings.ToLower(parsed.Host), nil
}
