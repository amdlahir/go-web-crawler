# Web Crawler System - Implementation

## Project Structure

```
web-crawler/
├── cmd/
│   ├── worker/          # Crawl worker binary
│   │   └── main.go
│   ├── scheduler/       # Scheduler service binary
│   │   └── main.go
│   ├── seed-api/        # Seed API service binary
│   │   └── main.go
│   └── cli/             # Management CLI
│       └── main.go
├── internal/
│   ├── frontier/        # URL frontier logic
│   │   ├── frontier.go
│   │   ├── priority.go
│   │   ├── politeness.go
│   │   └── bloom.go
│   ├── crawler/         # Crawling logic
│   │   ├── crawler.go
│   │   ├── colly.go
│   │   ├── chromedp.go
│   │   └── robots.go
│   ├── parser/          # Content parsing
│   │   ├── parser.go
│   │   ├── extractor.go
│   │   └── normalizer.go
│   ├── dedup/           # Deduplication
│   │   ├── url.go
│   │   ├── content.go
│   │   └── simhash.go
│   ├── storage/         # Storage adapters
│   │   ├── opensearch.go
│   │   ├── minio.go
│   │   └── redis.go
│   ├── metrics/         # Prometheus metrics
│   │   └── metrics.go
│   └── config/          # Configuration
│       └── config.go
├── pkg/
│   └── models/          # Shared types
│       ├── url.go
│       ├── page.go
│       └── result.go
├── deploy/
│   ├── base/            # Base K8s manifests
│   ├── overlays/
│   │   ├── dev/         # Development (kind)
│   │   └── prod/        # Production
│   └── helm/            # Helm chart
├── monitoring/
│   └── ui/              # Next.js dashboard
│       ├── pages/
│       ├── components/
│       └── lib/
├── scripts/
│   ├── setup-kind.sh
│   └── seed-urls.sh
├── go.mod
├── go.sum
└── Makefile
```

---

## Go Module Dependencies

```go
// go.mod
module github.com/org/web-crawler

go 1.22

require (
    // Web scraping
    github.com/gocolly/colly/v2 v2.1.0
    github.com/PuerkitoBio/goquery v1.9.0
    github.com/chromedp/chromedp v0.9.5

    // Storage
    github.com/redis/go-redis/v9 v9.5.0
    github.com/opensearch-project/opensearch-go/v2 v2.3.0
    github.com/minio/minio-go/v7 v7.0.70

    // Robots.txt
    github.com/temoto/robotstxt v1.1.2

    // Bloom filter
    github.com/bits-and-blooms/bloom/v3 v3.7.0

    // SimHash
    github.com/mfonda/simhash v0.0.0-20151007195837-79f94a1100d6

    // HTTP/networking
    golang.org/x/net v0.24.0
    golang.org/x/time v0.5.0  // rate limiting

    // Observability
    github.com/prometheus/client_golang v1.19.0
    go.uber.org/zap v1.27.0

    // Configuration
    github.com/spf13/viper v1.18.0
    github.com/spf13/cobra v1.8.0
)
```

---

## Key Interfaces

### Frontier Interface

```go
// internal/frontier/frontier.go
package frontier

import (
    "context"
    "time"

    "github.com/org/web-crawler/pkg/models"
)

type Frontier interface {
    // Add URLs to the frontier
    AddURLs(ctx context.Context, urls []models.URL, priority int) error

    // Get next URL to crawl (blocks if none available)
    NextURL(ctx context.Context) (*models.URL, error)

    // Mark URL as completed
    Complete(ctx context.Context, url *models.URL, result *models.CrawlResult) error

    // Mark URL as failed
    Fail(ctx context.Context, url *models.URL, err error) error

    // Check if URL has been seen
    IsSeen(ctx context.Context, url string) (bool, error)

    // Get queue statistics
    Stats(ctx context.Context) (*FrontierStats, error)
}

type FrontierStats struct {
    TotalPending    int64
    TotalCompleted  int64
    TotalFailed     int64
    HostQueueCounts map[string]int64
}
```

### Crawler Interface

```go
// internal/crawler/crawler.go
package crawler

import (
    "context"

    "github.com/org/web-crawler/pkg/models"
)

type Crawler interface {
    // Fetch and parse a URL
    Crawl(ctx context.Context, url *models.URL) (*models.CrawlResult, error)

    // Check if URL requires JS rendering
    RequiresJS(url string) bool

    // Close resources
    Close() error
}

type CrawlerConfig struct {
    UserAgent       string
    Timeout         time.Duration
    MaxRetries      int
    RespectRobots   bool
    FollowRedirects bool
    MaxBodySize     int64
}
```

### Parser Interface

```go
// internal/parser/parser.go
package parser

import (
    "github.com/PuerkitoBio/goquery"
    "github.com/org/web-crawler/pkg/models"
)

type Parser interface {
    // Parse HTML document
    Parse(doc *goquery.Document, baseURL string) (*models.ParsedPage, error)

    // Extract links from document
    ExtractLinks(doc *goquery.Document, baseURL string) ([]string, error)

    // Extract text content
    ExtractText(doc *goquery.Document) string
}

type ParsedPage struct {
    Title       string
    Content     string
    Description string
    Links       []string
    Metadata    map[string]string
}
```

### Storage Interface

```go
// internal/storage/storage.go
package storage

import (
    "context"
    "io"

    "github.com/org/web-crawler/pkg/models"
)

type PageStore interface {
    // Store parsed page in OpenSearch
    StorePage(ctx context.Context, page *models.CrawlResult) error

    // Search pages
    Search(ctx context.Context, query string, opts SearchOpts) ([]models.SearchResult, error)

    // Get page by URL
    GetByURL(ctx context.Context, url string) (*models.CrawlResult, error)
}

type BlobStore interface {
    // Store raw HTML
    StoreHTML(ctx context.Context, contentHash string, body io.Reader) error

    // Retrieve raw HTML
    GetHTML(ctx context.Context, contentHash string) (io.ReadCloser, error)

    // Check if exists
    Exists(ctx context.Context, contentHash string) (bool, error)
}
```

---

## Core Models

```go
// pkg/models/url.go
package models

import "time"

type URL struct {
    Raw         string            `json:"raw"`
    Normalized  string            `json:"normalized"`
    Host        string            `json:"host"`
    Depth       int               `json:"depth"`
    Priority    int               `json:"priority"`
    DiscoveredAt time.Time        `json:"discovered_at"`
    SourceURL   string            `json:"source_url,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
}

// pkg/models/page.go
type CrawlResult struct {
    URL           string            `json:"url"`
    FinalURL      string            `json:"final_url"`  // after redirects
    StatusCode    int               `json:"status_code"`
    Title         string            `json:"title"`
    Content       string            `json:"content"`
    ContentHash   string            `json:"content_hash"`
    SimHash       uint64            `json:"simhash"`
    Links         []string          `json:"links"`
    CrawledAt     time.Time         `json:"crawled_at"`
    FetchDuration time.Duration     `json:"fetch_duration"`
    Metadata      map[string]string `json:"metadata"`
    Error         string            `json:"error,omitempty"`
}
```

---

## Library Usage Patterns

### Colly (Static Pages)

```go
// internal/crawler/colly.go
package crawler

import (
    "context"
    "time"

    "github.com/gocolly/colly/v2"
    "github.com/org/web-crawler/pkg/models"
)

type CollyCrawler struct {
    collector *colly.Collector
    config    CrawlerConfig
}

func NewCollyCrawler(cfg CrawlerConfig) *CollyCrawler {
    c := colly.NewCollector(
        colly.UserAgent(cfg.UserAgent),
        colly.MaxDepth(0),  // We manage depth externally
        colly.Async(false), // One URL at a time per worker
    )

    c.SetRequestTimeout(cfg.Timeout)

    // Limit rules handled by frontier, not here
    c.Limit(&colly.LimitRule{
        DomainGlob:  "*",
        Parallelism: 1,
        Delay:       0,  // Frontier handles delay
    })

    return &CollyCrawler{collector: c, config: cfg}
}

func (c *CollyCrawler) Crawl(ctx context.Context, url *models.URL) (*models.CrawlResult, error) {
    result := &models.CrawlResult{
        URL:       url.Normalized,
        CrawledAt: time.Now(),
    }

    var parseErr error

    c.collector.OnResponse(func(r *colly.Response) {
        result.StatusCode = r.StatusCode
        result.FinalURL = r.Request.URL.String()

        doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
        if err != nil {
            parseErr = err
            return
        }

        result.Title = doc.Find("title").Text()
        result.Content = extractText(doc)
        result.ContentHash = hashContent(r.Body)
        result.SimHash = computeSimHash(result.Content)
        result.Links = extractLinks(doc, result.FinalURL)
    })

    c.collector.OnError(func(r *colly.Response, err error) {
        result.Error = err.Error()
        result.StatusCode = r.StatusCode
    })

    start := time.Now()
    err := c.collector.Visit(url.Normalized)
    result.FetchDuration = time.Since(start)

    if err != nil {
        return result, err
    }
    if parseErr != nil {
        return result, parseErr
    }

    return result, nil
}
```

### chromedp (JS-Heavy Pages)

```go
// internal/crawler/chromedp.go
package crawler

import (
    "context"
    "time"

    "github.com/chromedp/chromedp"
    "github.com/org/web-crawler/pkg/models"
)

type ChromedpCrawler struct {
    allocCtx context.Context
    cancel   context.CancelFunc
    timeout  time.Duration
}

func NewChromedpCrawler(remoteURL string, timeout time.Duration) (*ChromedpCrawler, error) {
    allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), remoteURL)

    return &ChromedpCrawler{
        allocCtx: allocCtx,
        cancel:   cancel,
        timeout:  timeout,
    }, nil
}

func (c *ChromedpCrawler) Crawl(ctx context.Context, url *models.URL) (*models.CrawlResult, error) {
    result := &models.CrawlResult{
        URL:       url.Normalized,
        CrawledAt: time.Now(),
    }

    ctx, cancel := context.WithTimeout(ctx, c.timeout)
    defer cancel()

    taskCtx, taskCancel := chromedp.NewContext(c.allocCtx)
    defer taskCancel()

    var html string
    var title string

    start := time.Now()
    err := chromedp.Run(taskCtx,
        chromedp.Navigate(url.Normalized),
        chromedp.WaitReady("body"),
        chromedp.Sleep(2*time.Second),  // Wait for JS
        chromedp.Title(&title),
        chromedp.OuterHTML("html", &html),
    )
    result.FetchDuration = time.Since(start)

    if err != nil {
        result.Error = err.Error()
        return result, err
    }

    result.Title = title
    result.StatusCode = 200
    result.FinalURL = url.Normalized

    // Parse rendered HTML
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
    if err != nil {
        return result, err
    }

    result.Content = extractText(doc)
    result.ContentHash = hashContent([]byte(html))
    result.SimHash = computeSimHash(result.Content)
    result.Links = extractLinks(doc, url.Normalized)

    return result, nil
}

func (c *ChromedpCrawler) Close() error {
    c.cancel()
    return nil
}
```

### goquery (DOM Parsing)

```go
// internal/parser/extractor.go
package parser

import (
    "net/url"
    "strings"

    "github.com/PuerkitoBio/goquery"
)

func ExtractLinks(doc *goquery.Document, baseURL string) ([]string, error) {
    base, err := url.Parse(baseURL)
    if err != nil {
        return nil, err
    }

    var links []string
    seen := make(map[string]bool)

    doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
        href, exists := s.Attr("href")
        if !exists || href == "" {
            return
        }

        // Skip non-HTTP links
        if strings.HasPrefix(href, "javascript:") ||
           strings.HasPrefix(href, "mailto:") ||
           strings.HasPrefix(href, "#") {
            return
        }

        // Resolve relative URLs
        parsed, err := url.Parse(href)
        if err != nil {
            return
        }
        resolved := base.ResolveReference(parsed)

        // Normalize
        normalized := normalizeURL(resolved)

        if !seen[normalized] {
            seen[normalized] = true
            links = append(links, normalized)
        }
    })

    return links, nil
}

func ExtractText(doc *goquery.Document) string {
    // Remove script and style elements
    doc.Find("script, style, noscript").Remove()

    // Get text from body
    body := doc.Find("body")
    text := body.Text()

    // Clean whitespace
    text = strings.Join(strings.Fields(text), " ")

    return text
}

func ExtractMetadata(doc *goquery.Document) map[string]string {
    meta := make(map[string]string)

    doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
        name, _ := s.Attr("name")
        property, _ := s.Attr("property")
        content, _ := s.Attr("content")

        key := name
        if key == "" {
            key = property
        }
        if key != "" && content != "" {
            meta[key] = content
        }
    })

    return meta
}
```

---

## Redis Queue Schema

### Key Patterns

```
# URL Frontier
frontier:priority                    # ZSET: priority queue
frontier:host:{hostname}             # LIST: per-host URL queue
frontier:host:meta:{hostname}        # HASH: host metadata

# Deduplication
seen:bloom                           # Bloom filter (RedisBloom)
seen:urls                            # SET: exact URL hashes

# Robots.txt Cache
robots:{hostname}                    # HASH: rules, expires

# Politeness
politeness:{hostname}                # HASH: last_fetch, delay_ms

# Worker State
worker:active:{worker_id}            # STRING: current URL (with TTL)
worker:stats:{worker_id}             # HASH: pages, errors, etc.

# Metrics
metrics:pages_total                  # Counter
metrics:errors_total:{type}          # Counter
metrics:queue_depth                  # Gauge (updated periodically)
```

### Lua Scripts

```lua
-- scripts/dequeue.lua
-- Atomically get next URL respecting politeness

local priority_key = KEYS[1]
local host_prefix = ARGV[1]
local min_delay_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

-- Get hosts with pending URLs
local hosts = redis.call('ZRANGE', priority_key, 0, 100)

for _, host in ipairs(hosts) do
    local politeness_key = 'politeness:' .. host
    local last_fetch = redis.call('HGET', politeness_key, 'last_fetch')

    if not last_fetch or (now_ms - tonumber(last_fetch)) >= min_delay_ms then
        local queue_key = host_prefix .. host
        local url_json = redis.call('LPOP', queue_key)

        if url_json then
            -- Update last fetch time
            redis.call('HSET', politeness_key, 'last_fetch', now_ms)

            -- Update priority queue if empty
            local remaining = redis.call('LLEN', queue_key)
            if remaining == 0 then
                redis.call('ZREM', priority_key, host)
            end

            return url_json
        end
    end
end

return nil
```

---

## OpenSearch Index Mappings

```json
{
  "settings": {
    "number_of_shards": 3,
    "number_of_replicas": 1,
    "analysis": {
      "analyzer": {
        "content_analyzer": {
          "type": "custom",
          "tokenizer": "standard",
          "filter": ["lowercase", "stop", "snowball"]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "url": {
        "type": "keyword"
      },
      "final_url": {
        "type": "keyword"
      },
      "domain": {
        "type": "keyword"
      },
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
      "meta_description": {
        "type": "text",
        "analyzer": "content_analyzer"
      },
      "links_out": {
        "type": "keyword"
      },
      "links_out_count": {
        "type": "integer"
      },
      "content_hash": {
        "type": "keyword"
      },
      "simhash": {
        "type": "long"
      },
      "depth": {
        "type": "integer"
      },
      "status_code": {
        "type": "integer"
      },
      "crawled_at": {
        "type": "date"
      },
      "fetch_duration_ms": {
        "type": "integer"
      },
      "content_length": {
        "type": "integer"
      },
      "metadata": {
        "type": "object",
        "enabled": false
      }
    }
  }
}
```

---

## Kubernetes Manifests

### Base Deployment (Kustomize)

```yaml
# deploy/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: crawler-system

resources:
  - namespace.yaml
  - redis/
  - opensearch/
  - minio/
  - scheduler/
  - worker/
  - seed-api/
  - monitoring/

configMapGenerator:
  - name: crawler-config
    files:
      - config.yaml
```

### Worker Deployment

```yaml
# deploy/base/worker/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crawler-worker
  labels:
    app: crawler-worker
spec:
  replicas: 10
  selector:
    matchLabels:
      app: crawler-worker
  template:
    metadata:
      labels:
        app: crawler-worker
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      containers:
      - name: worker
        image: crawler/worker:latest
        ports:
        - containerPort: 8080
          name: metrics
        env:
        - name: WORKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: REDIS_URL
          value: "redis://redis:6379"
        - name: OPENSEARCH_URL
          value: "http://opensearch:9200"
        - name: MINIO_ENDPOINT
          value: "minio:9000"
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /etc/crawler
      volumes:
      - name: config
        configMap:
          name: crawler-config
```

### Dev Overlay (kind)

```yaml
# deploy/overlays/dev/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

patches:
  - path: worker-patch.yaml

replicas:
  - name: crawler-worker
    count: 3
  - name: chromedp-pool
    count: 1
```

---

## Next.js Monitoring UI

### Project Structure

```
monitoring/ui/
├── pages/
│   ├── index.tsx           # Dashboard overview
│   ├── queue.tsx           # Queue status
│   ├── workers.tsx         # Worker status
│   ├── domains/
│   │   └── [domain].tsx    # Domain detail
│   ├── search.tsx          # Search crawled content
│   └── api/
│       ├── stats.ts
│       ├── queue/
│       │   └── [host].ts
│       ├── workers.ts
│       └── search.ts
├── components/
│   ├── Dashboard/
│   │   ├── StatsCard.tsx
│   │   ├── QueueChart.tsx
│   │   └── ErrorsTable.tsx
│   ├── Layout.tsx
│   └── common/
├── lib/
│   ├── redis.ts
│   ├── opensearch.ts
│   └── api.ts
├── types/
│   └── index.ts
└── styles/
```

### API Route Example

```typescript
// pages/api/stats.ts
import type { NextApiRequest, NextApiResponse } from 'next'
import { getRedisClient } from '@/lib/redis'
import { getOpenSearchClient } from '@/lib/opensearch'

interface CrawlerStats {
  pagesTotal: number
  pagesPerSecond: number
  queueDepth: number
  activeWorkers: number
  errorRate: number
  topDomains: Array<{ domain: string; count: number }>
}

export default async function handler(
  req: NextApiRequest,
  res: NextApiResponse<CrawlerStats>
): Promise<void> {
  const redis = getRedisClient()
  const os = getOpenSearchClient()

  const [
    pagesTotal,
    queueDepth,
    activeWorkers,
    errorsTotal
  ] = await Promise.all([
    redis.get('metrics:pages_total'),
    redis.get('metrics:queue_depth'),
    redis.keys('worker:active:*').then(k => k.length),
    redis.get('metrics:errors_total')
  ])

  // Calculate pages/sec from recent window
  const recentPages = await redis.lrange('metrics:pages_window', 0, 59)
  const pagesPerSecond = recentPages.reduce((a, b) => a + parseInt(b), 0) / 60

  // Top domains from OpenSearch
  const domainAgg = await os.search({
    index: 'crawled_pages',
    body: {
      size: 0,
      aggs: {
        top_domains: {
          terms: { field: 'domain', size: 10 }
        }
      }
    }
  })

  res.status(200).json({
    pagesTotal: parseInt(pagesTotal || '0'),
    pagesPerSecond,
    queueDepth: parseInt(queueDepth || '0'),
    activeWorkers,
    errorRate: parseInt(errorsTotal || '0') / parseInt(pagesTotal || '1'),
    topDomains: domainAgg.body.aggregations.top_domains.buckets.map(
      (b: { key: string; doc_count: number }) => ({
        domain: b.key,
        count: b.doc_count
      })
    )
  })
}
```

### Dashboard Component

```typescript
// components/Dashboard/StatsCard.tsx
import { FC } from 'react'

interface StatsCardProps {
  title: string
  value: string | number
  change?: number
  unit?: string
}

export const StatsCard: FC<StatsCardProps> = ({
  title,
  value,
  change,
  unit
}) => {
  return (
    <div className="bg-white rounded-lg shadow p-6">
      <h3 className="text-sm font-medium text-gray-500">{title}</h3>
      <div className="mt-2 flex items-baseline">
        <span className="text-3xl font-semibold text-gray-900">
          {typeof value === 'number' ? value.toLocaleString() : value}
        </span>
        {unit && (
          <span className="ml-1 text-sm text-gray-500">{unit}</span>
        )}
      </div>
      {change !== undefined && (
        <div className={`mt-2 text-sm ${change >= 0 ? 'text-green-600' : 'text-red-600'}`}>
          {change >= 0 ? '+' : ''}{change}% from last hour
        </div>
      )}
    </div>
  )
}
```

---

## Build and Deploy

### Makefile

```makefile
.PHONY: build test deploy

# Build
build:
	go build -o bin/worker ./cmd/worker
	go build -o bin/scheduler ./cmd/scheduler
	go build -o bin/seed-api ./cmd/seed-api
	go build -o bin/cli ./cmd/cli

# Docker
docker-build:
	docker build -t crawler/worker:latest -f docker/worker.Dockerfile .
	docker build -t crawler/scheduler:latest -f docker/scheduler.Dockerfile .
	docker build -t crawler/seed-api:latest -f docker/seed-api.Dockerfile .

# Kind cluster
kind-setup:
	kind create cluster --name crawler
	kubectl apply -k deploy/overlays/dev

kind-load:
	kind load docker-image crawler/worker:latest --name crawler
	kind load docker-image crawler/scheduler:latest --name crawler
	kind load docker-image crawler/seed-api:latest --name crawler

# Deploy
deploy-dev:
	kubectl apply -k deploy/overlays/dev

deploy-prod:
	kubectl apply -k deploy/overlays/prod

# Test
test:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

# Monitoring UI
ui-dev:
	cd monitoring/ui && npm run dev

ui-build:
	cd monitoring/ui && npm run build
```

### Quick Start Commands

```bash
# 1. Setup kind cluster
make kind-setup

# 2. Build and load images
make docker-build
make kind-load

# 3. Deploy
make deploy-dev

# 4. Add seed URLs
kubectl exec -it deploy/seed-api -- \
  /app/cli seed --url "https://example.com" --priority 0

# 5. Port-forward monitoring
kubectl port-forward svc/monitoring-ui 3000:3000

# 6. Watch logs
kubectl logs -f -l app=crawler-worker
```
