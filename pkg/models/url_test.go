package models

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "lowercase scheme and host",
			input:    "HTTPS://EXAMPLE.COM/Path",
			expected: "https://example.com/Path",
		},
		{
			name:     "remove default https port",
			input:    "https://example.com:443/path",
			expected: "https://example.com/path",
		},
		{
			name:     "remove default http port",
			input:    "http://example.com:80/path",
			expected: "http://example.com/path",
		},
		{
			name:     "keep non-default port",
			input:    "https://example.com:8080/path",
			expected: "https://example.com:8080/path",
		},
		{
			name:     "remove fragment",
			input:    "https://example.com/path#section",
			expected: "https://example.com/path",
		},
		{
			name:     "add trailing slash for root",
			input:    "https://example.com",
			expected: "https://example.com/",
		},
		{
			name:     "remove trailing slash for paths",
			input:    "https://example.com/path/",
			expected: "https://example.com/path",
		},
		{
			name:     "remove utm params",
			input:    "https://example.com/path?utm_source=google&utm_medium=cpc&id=123",
			expected: "https://example.com/path?id=123",
		},
		{
			name:     "remove fbclid",
			input:    "https://example.com/path?fbclid=abc123&page=1",
			expected: "https://example.com/path?page=1",
		},
		{
			name:     "sort query params",
			input:    "https://example.com/path?z=1&a=2&m=3",
			expected: "https://example.com/path?a=2&m=3&z=1",
		},
		{
			name:    "invalid url",
			input:   "://invalid",
			wantErr: true,
		},
		// Edge cases - whitespace
		{
			name:     "trim leading whitespace",
			input:    "  https://example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "trim trailing whitespace",
			input:    "https://example.com/path  ",
			expected: "https://example.com/path",
		},
		// Edge cases - protocol-relative
		{
			name:     "protocol-relative URL",
			input:    "//example.com/path",
			expected: "https://example.com/path",
		},
		// Edge cases - unsupported schemes
		{
			name:    "javascript scheme rejected",
			input:   "javascript:alert(1)",
			wantErr: true,
		},
		{
			name:    "data scheme rejected",
			input:   "data:text/html,<h1>hi</h1>",
			wantErr: true,
		},
		{
			name:    "ftp scheme rejected",
			input:   "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "mailto scheme rejected",
			input:   "mailto:user@example.com",
			wantErr: true,
		},
		// Edge cases - empty host
		{
			name:    "relative URL rejected",
			input:   "/path/to/page",
			wantErr: true,
		},
		{
			name:    "relative URL with dots rejected",
			input:   "../page.html",
			wantErr: true,
		},
		// Edge cases - userinfo stripped
		{
			name:     "strip userinfo credentials",
			input:    "https://user:pass@example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "strip user only",
			input:    "https://user@example.com/path",
			expected: "https://example.com/path",
		},
		// Edge cases - dot segments
		{
			name:     "resolve single dot",
			input:    "https://example.com/a/./b",
			expected: "https://example.com/a/b",
		},
		{
			name:     "resolve double dot",
			input:    "https://example.com/a/b/../c",
			expected: "https://example.com/a/c",
		},
		{
			name:     "resolve multiple double dots",
			input:    "https://example.com/a/b/c/../../d",
			expected: "https://example.com/a/d",
		},
		{
			name:     "double dot at root stays at root",
			input:    "https://example.com/../path",
			expected: "https://example.com/path",
		},
		// Edge cases - duplicate slashes
		{
			name:     "collapse duplicate slashes",
			input:    "https://example.com/a//b",
			expected: "https://example.com/a/b",
		},
		{
			name:     "collapse multiple duplicate slashes",
			input:    "https://example.com/a///b////c",
			expected: "https://example.com/a/b/c",
		},
		// Edge cases - combined
		{
			name:     "combined edge cases",
			input:    "  HTTPS://user:pass@EXAMPLE.COM:443/a/../b//c?utm_source=x&id=1#frag  ",
			expected: "https://example.com/b/c?id=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NormalizeURL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NormalizeURL() error = %v", err)
				return
			}
			if got != tt.expected {
				t.Errorf("NormalizeURL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		depth     int
		priority  int
		sourceURL string
		wantHost  string
		wantErr   bool
	}{
		{
			name:      "valid url",
			rawURL:    "https://example.com/path",
			depth:     1,
			priority:  0,
			sourceURL: "https://source.com",
			wantHost:  "example.com",
		},
		{
			name:      "url with port",
			rawURL:    "https://example.com:8080/path",
			depth:     0,
			priority:  1,
			sourceURL: "",
			wantHost:  "example.com:8080",
		},
		{
			name:    "invalid url - no scheme",
			rawURL:  "://missing-scheme",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewURL(tt.rawURL, tt.depth, tt.priority, tt.sourceURL)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewURL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewURL() error = %v", err)
				return
			}
			if got.Host != tt.wantHost {
				t.Errorf("NewURL().Host = %v, want %v", got.Host, tt.wantHost)
			}
			if got.Depth != tt.depth {
				t.Errorf("NewURL().Depth = %v, want %v", got.Depth, tt.depth)
			}
			if got.Priority != tt.priority {
				t.Errorf("NewURL().Priority = %v, want %v", got.Priority, tt.priority)
			}
			if got.SourceURL != tt.sourceURL {
				t.Errorf("NewURL().SourceURL = %v, want %v", got.SourceURL, tt.sourceURL)
			}
		})
	}
}

func TestURL_Hash(t *testing.T) {
	u1, _ := NewURL("https://example.com/path", 0, 0, "")
	u2, _ := NewURL("https://example.com/path", 0, 0, "")
	u3, _ := NewURL("https://example.com/other", 0, 0, "")

	if u1.Hash() != u2.Hash() {
		t.Error("Same URLs should have same hash")
	}

	if u1.Hash() == u3.Hash() {
		t.Error("Different URLs should have different hashes")
	}

	if len(u1.Hash()) != 64 {
		t.Errorf("Hash should be 64 chars (SHA256 hex), got %d", len(u1.Hash()))
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple host",
			url:      "https://example.com/path",
			expected: "example.com",
		},
		{
			name:     "host with port",
			url:      "https://example.com:8080/path",
			expected: "example.com:8080",
		},
		{
			name:     "subdomain",
			url:      "https://www.example.com/path",
			expected: "www.example.com",
		},
		{
			name:     "uppercase normalized",
			url:      "https://EXAMPLE.COM/path",
			expected: "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractHost(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractHost() expected error")
				}
				return
			}
			if err != nil {
				t.Errorf("ExtractHost() error = %v", err)
				return
			}
			if got != tt.expected {
				t.Errorf("ExtractHost() = %v, want %v", got, tt.expected)
			}
		})
	}
}
