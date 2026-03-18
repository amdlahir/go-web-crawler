package parser

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParse(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
	<title>Test Page</title>
	<meta name="description" content="A test page description">
	<meta property="og:title" content="OG Title">
</head>
<body>
	<h1>Main Heading</h1>
	<p>Some paragraph text.</p>
	<a href="/relative/link">Relative Link</a>
	<a href="https://example.com/absolute">Absolute Link</a>
	<a href="mailto:test@example.com">Email</a>
	<a href="javascript:void(0)">JS Link</a>
</body>
</html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	result := Parse(doc, "https://base.com/page")

	// Check title
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}

	// Check metadata
	if result.Metadata["description"] != "A test page description" {
		t.Errorf("description = %q, want %q", result.Metadata["description"], "A test page description")
	}
	if result.Metadata["og:title"] != "OG Title" {
		t.Errorf("og:title = %q, want %q", result.Metadata["og:title"], "OG Title")
	}

	// Check content contains expected text
	if !strings.Contains(result.Content, "Main Heading") {
		t.Error("Content should contain 'Main Heading'")
	}
	if !strings.Contains(result.Content, "paragraph text") {
		t.Error("Content should contain 'paragraph text'")
	}

	// Check links - should have 2 valid links (relative and absolute)
	if len(result.Links) != 2 {
		t.Errorf("Links count = %d, want 2", len(result.Links))
	}

	// Check relative link was resolved
	foundRelative := false
	foundAbsolute := false
	for _, link := range result.Links {
		if link == "https://base.com/relative/link" {
			foundRelative = true
		}
		if link == "https://example.com/absolute" {
			foundAbsolute = true
		}
	}
	if !foundRelative {
		t.Error("Should have resolved relative link")
	}
	if !foundAbsolute {
		t.Error("Should have absolute link")
	}
}

func TestExtractText(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
	<style>body { color: red; }</style>
	<script>console.log("test");</script>
</head>
<body>
	<nav>Navigation</nav>
	<main>
		<p>Main   content    with   spaces.</p>
		<div class="hidden">Hidden content</div>
	</main>
	<footer>Footer</footer>
</body>
</html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	text := extractText(doc)

	// Should not contain script or style content
	if strings.Contains(text, "console.log") {
		t.Error("Should not contain script content")
	}
	if strings.Contains(text, "color: red") {
		t.Error("Should not contain style content")
	}

	// Should contain main content
	if !strings.Contains(text, "Main content with spaces") {
		t.Errorf("Should contain normalized main content, got: %s", text)
	}
}

func TestExtractLinks(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
	<a href="https://example.com/page1">Link 1</a>
	<a href="https://example.com/page2">Link 2</a>
	<a href="https://example.com/page1">Duplicate</a>
	<a href="/relative">Relative</a>
	<a href="page.html">Relative file</a>
	<a href="mailto:test@test.com">Email</a>
	<a href="javascript:void(0)">JS</a>
	<a href="tel:123456">Phone</a>
	<a href="#section">Anchor</a>
	<a>No href</a>
</body>
</html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	links := extractLinks(doc, "https://base.com/dir/")

	// Should deduplicate
	expectedCount := 4 // page1, page2, /relative, page.html
	if len(links) != expectedCount {
		t.Errorf("Links count = %d, want %d. Links: %v", len(links), expectedCount, links)
	}

	// Check specific links
	expected := map[string]bool{
		"https://example.com/page1":    true,
		"https://example.com/page2":    true,
		"https://base.com/relative":    true,
		"https://base.com/dir/page.html": true,
	}

	for _, link := range links {
		if !expected[link] {
			t.Errorf("Unexpected link: %s", link)
		}
	}
}

func TestExtractMetadata(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
	<meta name="description" content="Page description">
	<meta name="keywords" content="test, keywords">
	<meta property="og:title" content="OG Title">
	<meta property="og:image" content="https://example.com/image.png">
	<meta name="robots" content="index, follow">
	<link rel="canonical" href="https://example.com/canonical">
</head>
<body></body>
</html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	meta := extractMetadata(doc)

	tests := map[string]string{
		"description": "Page description",
		"keywords":    "test, keywords",
		"og:title":    "OG Title",
		"og:image":    "https://example.com/image.png",
		"robots":      "index, follow",
		"canonical":   "https://example.com/canonical",
	}

	for key, expected := range tests {
		if meta[key] != expected {
			t.Errorf("meta[%s] = %q, want %q", key, meta[key], expected)
		}
	}
}

func TestExtractHeadings(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
	<h1>Main Title</h1>
	<h2>Section 1</h2>
	<p>Content</p>
	<h2>Section 2</h2>
	<h3>Subsection</h3>
	<h4></h4>
</body>
</html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	headings := ExtractHeadings(doc)

	expected := []string{"Main Title", "Section 1", "Section 2", "Subsection"}
	if len(headings) != len(expected) {
		t.Errorf("Headings count = %d, want %d", len(headings), len(expected))
	}

	for i, h := range headings {
		if h != expected[i] {
			t.Errorf("Heading[%d] = %q, want %q", i, h, expected[i])
		}
	}
}

func TestExtractImages(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
	<img src="/image1.png">
	<img src="https://cdn.example.com/image2.jpg">
	<img src="data:image/png;base64,ABC123">
	<img src="relative/image3.gif">
	<img>
</body>
</html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	images := ExtractImages(doc, "https://base.com/page/")

	// Should have 3 images (excluding data URL and empty)
	if len(images) != 3 {
		t.Errorf("Images count = %d, want 3. Images: %v", len(images), images)
	}

	expected := map[string]bool{
		"https://base.com/image1.png":           true,
		"https://cdn.example.com/image2.jpg":    true,
		"https://base.com/page/relative/image3.gif": true,
	}

	for _, img := range images {
		if !expected[img] {
			t.Errorf("Unexpected image: %s", img)
		}
	}
}

func TestCleanWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "  hello   world  ",
			expected: "hello world",
		},
		{
			input:    "line1\n\n\nline2",
			expected: "line1 line2",
		},
		{
			input:    "\t\ttab\tspaced\t\t",
			expected: "tab spaced",
		},
		{
			input:    "no extra spaces",
			expected: "no extra spaces",
		},
	}

	for _, tt := range tests {
		got := cleanWhitespace(tt.input)
		if got != tt.expected {
			t.Errorf("cleanWhitespace(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	html := `<!DOCTYPE html>
<html>
<head><title>Benchmark</title></head>
<body>
	<h1>Title</h1>
	<p>Content paragraph with text.</p>
	<a href="/link1">Link 1</a>
	<a href="/link2">Link 2</a>
	<a href="/link3">Link 3</a>
</body>
</html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(doc, "https://example.com/")
	}
}
