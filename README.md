## Project Overview

Distributed web crawler in Go with stateless workers, Redis-backed frontier, and multi-storage (OpenSearch, MinIO, Redis). Kubernetes-native with HPA scaling.

## Commands

```bash
# Build & Test
make build              # Build all binaries to bin/
make test               # Run tests with race detector
make test-coverage      # Generate coverage HTML
make lint               # golangci-lint
make fmt                # gofmt

# Docker
make docker-build       # Build worker, scheduler, seed-api, cli images
make docker-push        # Push to registry

# Kubernetes (kind)
make quickstart         # Full setup: cluster + images + deploy
make kind-setup         # Create kind cluster
make kind-load          # Load images
make deploy-dev         # Apply dev overlay
make undeploy           # Clean up

# Operations
make port-forward-redis
make port-forward-opensearch
make port-forward-scheduler
make logs-worker
make stats
make seed FILE=urls.txt
```

Run single test:
```bash
go test -v -run TestBloomFilter ./internal/frontier/
```

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  seed-api   │────▶│   Redis     │◀────│  scheduler  │
│  (8082)     │     │  Frontier   │     │  (8081)     │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                    ┌──────▼──────┐
                    │   Workers   │──┬──▶ OpenSearch (parsed)
                    │   (8080)    │  └──▶ MinIO (raw HTML)
                    └─────────────┘
```

**Entry Points (cmd/):**
- `worker` - Fetches URLs, extracts links, stores results
- `scheduler` - Queue stats API, worker coordination
- `seed-api` - REST API for adding seed URLs
- `cli` - Local tool for seed/stats/search/queue ops

**Internal Packages:**
- `config/` - Env-based config loading
- `frontier/` - URL queue, Bloom filter dedup, politeness, robots.txt
- `crawler/` - Colly (static) + chromedp (JS) routing
- `dedup/` - SimHash near-duplicate detection
- `storage/` - Redis, OpenSearch, MinIO wrappers
- `parser/` - HTML parsing via goquery
- `metrics/` - Prometheus instrumentation

**pkg/models/:**
- `url.go` - URL normalization (strips UTM/session params)
- `result.go` - CrawlResult, SearchResult, FrontierStats types

## Worker Flow

1. Poll Redis frontier → Get next URL
2. Check robots.txt (24h cache)
3. Crawl via Colly or chromedp (auto-detects JS-heavy sites)
4. Deduplicate via SimHash + Bloom filter
5. Store: raw HTML → MinIO, parsed → OpenSearch
6. Extract links, queue new URLs (respects max depth)

## Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| REDIS_URL | redis://localhost:6379 | Queue/frontier |
| OPENSEARCH_URL | http://localhost:9200 | Search index |
| MINIO_ENDPOINT | localhost:9000 | Raw HTML storage |
| CRAWLER_MAX_DEPTH | 10 | Link discovery limit |
| CRAWLER_RESPECT_ROBOTS | true | robots.txt enforcement |
| FRONTIER_DEFAULT_DELAY | 1s | Per-host politeness |
| CHROMEDP_POOL_SIZE | 5 | Concurrent headless browsers |

## API Endpoints

**Scheduler (8081):**
- `GET /api/v1/stats` - Queue/worker stats
- `POST /api/v1/seeds` - Add seed URLs

**Seed API (8082):**
- `POST /api/v1/seeds` - Add single URL
- `POST /api/v1/seeds/bulk` - Bulk add

## Testing

Test files exist for:
- `internal/frontier/bloom_test.go`
- `internal/dedup/simhash_test.go`
- `internal/parser/parser_test.go`
- `pkg/models/url_test.go`

## Dependencies

Core: gocolly/colly, chromedp, goquery, go-redis, opensearch-go, minio-go, bloom, robotstxt, cobra, zap, prometheus
