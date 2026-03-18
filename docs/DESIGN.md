# Web Crawler System - Design

## Design Principles

Based on established web crawler design patterns:

1. **Separation of Concerns** - Each component has a single responsibility
2. **Stateless Workers** - Crawl workers hold no local state; all state in Redis/storage
3. **Politeness First** - Rate limiting and robots.txt are non-negotiable
4. **Deduplication at Every Level** - URL dedup, content dedup, storage dedup
5. **Graceful Degradation** - System continues operating when components fail

## Core Components

### 1. URL Frontier

The central scheduling system for crawl URLs.

**Responsibilities:**
- Store pending URLs for crawling
- Enforce per-host politeness (rate limits)
- Prioritize URLs based on importance
- Track crawl state for resumability

**Design:**
```
┌─────────────────────────────────────────────────┐
│                 URL Frontier                     │
├─────────────────────────────────────────────────┤
│  Priority Queues     │  Per-Host Queues         │
│  ┌─────────────┐     │  ┌──────────────────┐    │
│  │ P0 (high)   │     │  │ host:example.com │    │
│  │ P1 (medium) │     │  │ host:site.org    │    │
│  │ P2 (low)    │     │  │ host:...         │    │
│  └─────────────┘     │  └──────────────────┘    │
├─────────────────────────────────────────────────┤
│  Politeness Tracker (last-fetch timestamps)     │
└─────────────────────────────────────────────────┘
```

### 2. HTML Downloader

Fetches web pages and respects crawl rules.

**Responsibilities:**
- HTTP/HTTPS fetching with proper headers
- robots.txt parsing and caching
- Connection pooling and keep-alive
- Timeout and retry handling
- DNS caching

**Two-Mode Operation:**
| Mode | Library | Use Case |
|------|---------|----------|
| Static | Colly | Standard HTML pages |
| Dynamic | chromedp | JS-rendered SPAs |

### 3. Content Parser

Extracts structured data from HTML.

**Responsibilities:**
- DOM parsing via goquery
- Link extraction and normalization
- Text content extraction
- Metadata extraction (title, description, etc.)

### 4. Deduplication Service

Prevents redundant crawling and storage.

**URL Deduplication:**
- Normalize URLs before comparison
- Bloom filter for O(1) membership check
- Persist seen URLs to Redis set

**Content Deduplication:**
- SimHash fingerprint of page content
- Detect near-duplicates (mirrors, tracking variants)
- Store fingerprints in Redis sorted set

### 5. Storage Layer

Dual storage for different access patterns.

| Store | Content | Access Pattern |
|-------|---------|----------------|
| MinIO | Raw HTML | Archival, replay |
| OpenSearch | Parsed text + metadata | Full-text search |

### 6. Monitoring Service

Real-time visibility into crawl operations.

**Metrics Tracked:**
- Pages crawled/sec
- Queue depth by host
- Error rates by type
- Worker utilization
- Storage growth

---

## Data Flow Pipeline

```
Seeds → URL Frontier → Dispatcher → Worker Pool
                                        │
                    ┌───────────────────┴───────────────────┐
                    │                                       │
              Static Pages                           Dynamic Pages
              (Colly)                                (chromedp)
                    │                                       │
                    └───────────────────┬───────────────────┘
                                        │
                                        ▼
                               Content Parser
                              (goquery)
                                        │
                    ┌───────────────────┼───────────────────┐
                    │                   │                   │
                    ▼                   ▼                   ▼
              URL Dedup          Content Dedup         Link Extract
              (Bloom)            (SimHash)             (normalize)
                    │                   │                   │
                    │                   ▼                   │
                    │            ┌─────────────┐           │
                    │            │   Storage   │           │
                    │            │ MinIO + OS  │           │
                    │            └─────────────┘           │
                    │                                       │
                    └──────────────────┬───────────────────┘
                                       │
                                       ▼
                              URL Frontier (new URLs)
```

---

## Politeness Strategy

### Per-Host Rate Limiting

Each host has a dedicated queue with enforced delays:

```go
type HostQueue struct {
    Host          string
    LastFetch     time.Time
    MinDelay      time.Duration  // from robots.txt or default
    PendingURLs   []string
}
```

**Default delays:**
- Standard sites: 1 second between requests
- robots.txt crawl-delay: honor if specified
- Adaptive backoff: increase delay on 429/503

### robots.txt Compliance

```go
type RobotsCache struct {
    Host      string
    Rules     *robotstxt.Group
    CachedAt  time.Time
    TTL       time.Duration  // 24 hours default
}
```

**Stored in Redis:**
- Key: `robots:{host}`
- Value: serialized rules + metadata
- TTL: 24 hours (auto-refresh)

---

## Deduplication Strategy

### URL Normalization

Before any URL comparison:

1. Lowercase scheme and host
2. Remove default ports (80/443)
3. Sort query parameters
4. Remove tracking params (utm_*, fbclid, etc.)
5. Remove fragment identifiers
6. Resolve relative paths

### URL Seen Check (Bloom Filter)

```
Parameters for ~10M URLs, 0.1% false positive:
- Bits: ~144M (18MB memory)
- Hash functions: 10
```

**Flow:**
1. Normalize URL
2. Check Bloom filter (fast path)
3. If possibly-seen, check Redis SET (slow path)
4. Add to both if new

### Content Fingerprinting (SimHash)

```go
type ContentFingerprint struct {
    URL       string
    SimHash   uint64
    Timestamp time.Time
}
```

**Near-duplicate detection:**
- Hamming distance < 3 bits = likely duplicate
- Store fingerprints in Redis sorted set by hash prefix
- O(1) lookup for exact match, O(k) for near-match

---

## Robustness

### Checkpointing

Crawl state persisted to Redis:

| Key Pattern | Data |
|-------------|------|
| `frontier:queue:{host}` | Pending URLs per host |
| `frontier:seen` | Bloom filter state |
| `frontier:progress` | Crawl statistics |
| `worker:state:{id}` | Current URL being processed |

### Error Handling

| Error Type | Action |
|------------|--------|
| DNS failure | Retry with backoff, mark host as problematic |
| HTTP 4xx | Log and skip (except 429 → backoff) |
| HTTP 5xx | Retry with exponential backoff |
| Timeout | Retry once, then skip |
| Parse error | Log, store raw HTML, continue |

### Circuit Breaker

Per-host circuit breaker to avoid hammering broken hosts:

```go
type HostCircuit struct {
    Host           string
    FailureCount   int
    LastFailure    time.Time
    State          CircuitState  // CLOSED, OPEN, HALF_OPEN
}
```

---

## Extensibility Points

### Custom Extractors

```go
type Extractor interface {
    Name() string
    Match(url string) bool
    Extract(doc *goquery.Document) (interface{}, error)
}
```

### Custom Filters

```go
type URLFilter interface {
    Name() string
    ShouldCrawl(url string) bool
}
```

### Output Handlers

```go
type OutputHandler interface {
    Name() string
    Handle(result *CrawlResult) error
}
```

---

## Scalability Considerations

### Horizontal Scaling

- Workers are stateless → add pods via HPA
- Redis handles coordination (single writer, multiple readers)
- OpenSearch cluster for index sharding

### Bottleneck Analysis

| Component | Bottleneck | Mitigation |
|-----------|-----------|------------|
| URL Frontier | Redis throughput | Pipeline operations, Lua scripts |
| DNS | Resolution latency | Local cache, dedicated resolver |
| Network | Bandwidth | Connection pooling, compression |
| Storage | Write throughput | Batch writes, async indexing |
| chromedp | Chrome instances | Separate pod pool, resource limits |

### Target: 100 pages/sec

**Worker calculation:**
- Avg page fetch: 500ms
- Avg parse: 100ms
- Per-worker throughput: ~1.5 pages/sec
- Required workers: ~70 for 100 pages/sec
- With buffer: 80-100 worker replicas
