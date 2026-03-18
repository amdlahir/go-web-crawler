package parser

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ParsedPage holds extracted data from HTML.
type ParsedPage struct {
	Title       string
	Content     string
	Description string
	Links       []string
	Metadata    map[string]string
}

// Parse extracts content from an HTML document.
func Parse(doc *goquery.Document, baseURL string) *ParsedPage {
	result := &ParsedPage{
		Metadata: make(map[string]string),
	}

	// Extract title
	result.Title = strings.TrimSpace(doc.Find("title").First().Text())

	// Extract metadata
	result.Metadata = extractMetadata(doc)
	if desc, ok := result.Metadata["description"]; ok {
		result.Description = desc
	}

	// Extract text content
	result.Content = extractText(doc)

	// Extract links
	result.Links = extractLinks(doc, baseURL)

	return result
}

// extractMetadata extracts meta tags from the document.
func extractMetadata(doc *goquery.Document) map[string]string {
	meta := make(map[string]string)

	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		name, nameExists := s.Attr("name")
		property, propExists := s.Attr("property")
		content, contentExists := s.Attr("content")

		if !contentExists || content == "" {
			return
		}

		key := ""
		if nameExists && name != "" {
			key = strings.ToLower(name)
		} else if propExists && property != "" {
			key = strings.ToLower(property)
		}

		if key != "" {
			meta[key] = content
		}
	})

	// Extract canonical URL
	doc.Find("link[rel='canonical']").Each(func(_ int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			meta["canonical"] = href
		}
	})

	return meta
}

// extractText extracts visible text from the document.
func extractText(doc *goquery.Document) string {
	// Clone the document to avoid modifying the original
	docCopy := doc.Clone()

	// Remove non-content elements
	docCopy.Find("script, style, noscript, iframe, nav, footer, header, aside").Remove()
	docCopy.Find("[style*='display:none'], [style*='display: none']").Remove()
	docCopy.Find(".hidden, .hide, .invisible").Remove()

	// Get text from body
	body := docCopy.Find("body")
	if body.Length() == 0 {
		body = docCopy.Find("html")
	}

	text := body.Text()

	// Clean whitespace
	text = cleanWhitespace(text)

	return text
}

// cleanWhitespace normalizes whitespace in text.
func cleanWhitespace(text string) string {
	// Replace multiple whitespace with single space
	var result strings.Builder
	prevSpace := false

	for _, r := range text {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if isSpace {
			if !prevSpace {
				result.WriteRune(' ')
			}
			prevSpace = true
		} else {
			result.WriteRune(r)
			prevSpace = false
		}
	}

	return strings.TrimSpace(result.String())
}

// extractLinks extracts and normalizes links from the document.
func extractLinks(doc *goquery.Document, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var links []string

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		// Skip non-HTTP links
		href = strings.TrimSpace(href)
		if shouldSkipLink(href) {
			return
		}

		// Resolve relative URLs
		parsed, err := url.Parse(href)
		if err != nil {
			return
		}

		resolved := base.ResolveReference(parsed)

		// Only keep HTTP/HTTPS links
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return
		}

		// Normalize the URL
		normalized := normalizeLink(resolved)

		// Deduplicate
		if !seen[normalized] {
			seen[normalized] = true
			links = append(links, normalized)
		}
	})

	return links
}

// shouldSkipLink checks if a link should be skipped.
func shouldSkipLink(href string) bool {
	prefixes := []string{
		"javascript:",
		"mailto:",
		"tel:",
		"data:",
		"#",
		"about:",
		"blob:",
	}

	hrefLower := strings.ToLower(href)
	for _, prefix := range prefixes {
		if strings.HasPrefix(hrefLower, prefix) {
			return true
		}
	}

	return false
}

// normalizeLink normalizes a URL for consistency.
func normalizeLink(u *url.URL) string {
	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove default ports
	if (u.Scheme == "http" && strings.HasSuffix(u.Host, ":80")) ||
		(u.Scheme == "https" && strings.HasSuffix(u.Host, ":443")) {
		u.Host = strings.TrimSuffix(u.Host, ":80")
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}

	// Remove fragment
	u.Fragment = ""

	// Normalize path
	if u.Path == "" {
		u.Path = "/"
	}

	return u.String()
}

// ExtractMainContent attempts to extract the main content area.
func ExtractMainContent(doc *goquery.Document) string {
	// Try common main content selectors
	selectors := []string{
		"article",
		"main",
		"[role='main']",
		".content",
		".post-content",
		".article-content",
		".entry-content",
		"#content",
		"#main",
	}

	for _, selector := range selectors {
		content := doc.Find(selector).First()
		if content.Length() > 0 {
			text := content.Text()
			text = cleanWhitespace(text)
			if len(text) > 100 { // Minimum content length
				return text
			}
		}
	}

	// Fallback to full body text
	return extractText(doc)
}

// ExtractHeadings extracts all headings from the document.
func ExtractHeadings(doc *goquery.Document) []string {
	var headings []string

	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			headings = append(headings, text)
		}
	})

	return headings
}

// ExtractImages extracts image URLs from the document.
func ExtractImages(doc *goquery.Document, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var images []string

	doc.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || src == "" {
			return
		}

		// Skip data URLs
		if strings.HasPrefix(src, "data:") {
			return
		}

		// Resolve relative URLs
		parsed, err := url.Parse(src)
		if err != nil {
			return
		}

		resolved := base.ResolveReference(parsed).String()

		if !seen[resolved] {
			seen[resolved] = true
			images = append(images, resolved)
		}
	})

	return images
}
