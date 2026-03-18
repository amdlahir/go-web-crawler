package storage

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	"github.com/amdlahir/go-web-crawler/internal/config"
	"github.com/amdlahir/go-web-crawler/pkg/models"
)

// OpenSearchClient wraps the OpenSearch client.
type OpenSearchClient struct {
	client *opensearch.Client
	index  string
}

// NewOpenSearchClient creates a new OpenSearch client.
func NewOpenSearchClient(cfg config.OpenSearchConfig) (*OpenSearchClient, error) {
	osConfig := opensearch.Config{
		Addresses: cfg.URLs,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	if cfg.Username != "" {
		osConfig.Username = cfg.Username
		osConfig.Password = cfg.Password
	}

	client, err := opensearch.NewClient(osConfig)
	if err != nil {
		return nil, fmt.Errorf("create opensearch client: %w", err)
	}

	// Test connection
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("opensearch info: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("opensearch info error: %s", res.String())
	}

	return &OpenSearchClient{
		client: client,
		index:  cfg.Index,
	}, nil
}

// EnsureIndex creates the index with mappings if it doesn't exist.
func (o *OpenSearchClient) EnsureIndex(ctx context.Context) error {
	// Check if index exists
	res, err := o.client.Indices.Exists([]string{o.index})
	if err != nil {
		return fmt.Errorf("check index exists: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		return nil // Index exists
	}

	// Create index with mappings
	mapping := `{
		"settings": {
			"number_of_shards": 3,
			"number_of_replicas": 1,
			"analysis": {
				"analyzer": {
					"content_analyzer": {
						"type": "custom",
						"tokenizer": "standard",
						"filter": ["lowercase", "stop"]
					}
				}
			}
		},
		"mappings": {
			"properties": {
				"url": { "type": "keyword" },
				"final_url": { "type": "keyword" },
				"domain": { "type": "keyword" },
				"title": {
					"type": "text",
					"analyzer": "content_analyzer",
					"fields": {
						"raw": { "type": "keyword" }
					}
				},
				"content": {
					"type": "text",
					"analyzer": "content_analyzer"
				},
				"meta_description": { "type": "text" },
				"links_out": { "type": "keyword" },
				"links_count": { "type": "integer" },
				"content_hash": { "type": "keyword" },
				"simhash": { "type": "long" },
				"depth": { "type": "integer" },
				"status_code": { "type": "integer" },
				"crawled_at": { "type": "date" },
				"fetch_duration_ms": { "type": "integer" },
				"content_length": { "type": "integer" }
			}
		}
	}`

	res, err = o.client.Indices.Create(
		o.index,
		o.client.Indices.Create.WithBody(strings.NewReader(mapping)),
		o.client.Indices.Create.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("create index error: %s", res.String())
	}

	return nil
}

// pageDocument represents the document structure for OpenSearch.
type pageDocument struct {
	URL             string    `json:"url"`
	FinalURL        string    `json:"final_url"`
	Domain          string    `json:"domain"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	MetaDescription string    `json:"meta_description,omitempty"`
	LinksOut        []string  `json:"links_out,omitempty"`
	LinksCount      int       `json:"links_count"`
	ContentHash     string    `json:"content_hash"`
	SimHash         int64     `json:"simhash"`
	Depth           int       `json:"depth"`
	StatusCode      int       `json:"status_code"`
	CrawledAt       time.Time `json:"crawled_at"`
	FetchDurationMs int64     `json:"fetch_duration_ms"`
	ContentLength   int       `json:"content_length"`
}

// StorePage indexes a crawl result.
func (o *OpenSearchClient) StorePage(ctx context.Context, result *models.CrawlResult) error {
	doc := pageDocument{
		URL:             result.URL,
		FinalURL:        result.FinalURL,
		Domain:          result.Domain,
		Title:           result.Title,
		Content:         result.Content,
		MetaDescription: result.Metadata["description"],
		LinksOut:        result.Links,
		LinksCount:      result.LinksCount,
		ContentHash:     result.ContentHash,
		SimHash:         int64(result.SimHash),
		Depth:           result.Depth,
		StatusCode:      result.StatusCode,
		CrawledAt:       result.CrawledAt,
		FetchDurationMs: result.FetchDuration.Milliseconds(),
		ContentLength:   result.ContentLength,
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}

	req := opensearchapi.IndexRequest{
		Index:      o.index,
		DocumentID: result.ContentHash,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index document error: %s", res.String())
	}

	return nil
}

// Search performs a full-text search.
func (o *OpenSearchClient) Search(ctx context.Context, opts models.SearchOpts) ([]models.SearchResult, int64, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{
					map[string]interface{}{
						"multi_match": map[string]interface{}{
							"query":  opts.Query,
							"fields": []string{"title^2", "content", "meta_description"},
						},
					},
				},
			},
		},
		"from": opts.From,
		"size": opts.Size,
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"content": map[string]interface{}{
					"fragment_size": 150,
				},
			},
		},
	}

	// Add domain filter if specified
	if opts.Domain != "" {
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["filter"] = []interface{}{
			map[string]interface{}{
				"term": map[string]interface{}{
					"domain": opts.Domain,
				},
			},
		}
	}

	// Add sorting
	if opts.SortBy != "" {
		order := "desc"
		if opts.SortDir != "" {
			order = opts.SortDir
		}
		query["sort"] = []interface{}{
			map[string]interface{}{
				opts.SortBy: map[string]interface{}{
					"order": order,
				},
			},
		}
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal query: %w", err)
	}

	res, err := o.client.Search(
		o.client.Search.WithContext(ctx),
		o.client.Search.WithIndex(o.index),
		o.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, 0, fmt.Errorf("search error: %s", res.String())
	}

	var response struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source    pageDocument `json:"_source"`
				Score     float64      `json:"_score"`
				Highlight struct {
					Content []string `json:"content"`
				} `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}

	results := make([]models.SearchResult, len(response.Hits.Hits))
	for i, hit := range response.Hits.Hits {
		snippet := ""
		if len(hit.Highlight.Content) > 0 {
			snippet = hit.Highlight.Content[0]
		}
		results[i] = models.SearchResult{
			URL:       hit.Source.URL,
			Title:     hit.Source.Title,
			Snippet:   snippet,
			Domain:    hit.Source.Domain,
			CrawledAt: hit.Source.CrawledAt,
			Score:     hit.Score,
		}
	}

	return results, response.Hits.Total.Value, nil
}

// GetByURL retrieves a page by URL.
func (o *OpenSearchClient) GetByURL(ctx context.Context, url string) (*models.CrawlResult, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"url": url,
			},
		},
		"size": 1,
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	res, err := o.client.Search(
		o.client.Search.WithContext(ctx),
		o.client.Search.WithIndex(o.index),
		o.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search error: %s", res.String())
	}

	var response struct {
		Hits struct {
			Hits []struct {
				Source pageDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(response.Hits.Hits) == 0 {
		return nil, nil
	}

	doc := response.Hits.Hits[0].Source
	return &models.CrawlResult{
		URL:           doc.URL,
		FinalURL:      doc.FinalURL,
		Domain:        doc.Domain,
		Title:         doc.Title,
		Content:       doc.Content,
		ContentHash:   doc.ContentHash,
		SimHash:       uint64(doc.SimHash),
		Links:         doc.LinksOut,
		LinksCount:    doc.LinksCount,
		StatusCode:    doc.StatusCode,
		CrawledAt:     doc.CrawledAt,
		FetchDuration: time.Duration(doc.FetchDurationMs) * time.Millisecond,
		ContentLength: doc.ContentLength,
		Depth:         doc.Depth,
	}, nil
}

// GetStats returns index statistics.
func (o *OpenSearchClient) GetStats(ctx context.Context) (int64, error) {
	res, err := o.client.Count(
		o.client.Count.WithContext(ctx),
		o.client.Count.WithIndex(o.index),
	)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, fmt.Errorf("count error: %s", res.String())
	}

	var response struct {
		Count int64 `json:"count"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	return response.Count, nil
}

// Close closes the OpenSearch client.
func (o *OpenSearchClient) Close() error {
	return nil // OpenSearch client doesn't need explicit close
}
