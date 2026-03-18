# Web Crawler System - Architecture

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              KUBERNETES CLUSTER                                  │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                           CONTROL PLANE                                  │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │    │
│  │  │  Scheduler  │  │  Seed API   │  │  Metrics    │  │  Config     │     │    │
│  │  │  Service    │  │  Service    │  │  Exporter   │  │  Manager    │     │    │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────────────┘     │    │
│  └─────────┼────────────────┼────────────────┼─────────────────────────────┘    │
│            │                │                │                                   │
│  ┌─────────▼────────────────▼────────────────▼─────────────────────────────┐    │
│  │                           DATA PLANE                                     │    │
│  │                                                                          │    │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │    │
│  │  │                      URL FRONTIER (Redis)                        │    │    │
│  │  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐         │    │    │
│  │  │  │Priority Q│  │Host Queue│  │Robots    │  │Bloom     │         │    │    │
│  │  │  │(sorted)  │  │(per-host)│  │Cache     │  │Filter    │         │    │    │
│  │  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘         │    │    │
│  │  └──────────────────────────────┬──────────────────────────────────┘    │    │
│  │                                 │                                        │    │
│  │  ┌──────────────────────────────▼──────────────────────────────────┐    │    │
│  │  │                      WORKER POOL (HPA)                           │    │    │
│  │  │  ┌──────────┐  ┌──────────┐  ┌──────────┐       ┌──────────┐    │    │    │
│  │  │  │ Worker 1 │  │ Worker 2 │  │ Worker 3 │  ...  │ Worker N │    │    │    │
│  │  │  │ (Colly)  │  │ (Colly)  │  │ (Colly)  │       │ (Colly)  │    │    │    │
│  │  │  └──────────┘  └──────────┘  └──────────┘       └──────────┘    │    │    │
│  │  └──────────────────────────────┬──────────────────────────────────┘    │    │
│  │                                 │                                        │    │
│  │  ┌──────────────────────────────▼──────────────────────────────────┐    │    │
│  │  │                    CHROMEDP POOL (for JS)                        │    │    │
│  │  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                       │    │    │
│  │  │  │ Chrome 1 │  │ Chrome 2 │  │ Chrome 3 │                       │    │    │
│  │  │  │ (headless│  │ (headless│  │ (headless│                       │    │    │
│  │  │  └──────────┘  └──────────┘  └──────────┘                       │    │    │
│  │  └──────────────────────────────┬──────────────────────────────────┘    │    │
│  │                                 │                                        │    │
│  └─────────────────────────────────┼────────────────────────────────────────┘    │
│                                    │                                             │
│  ┌─────────────────────────────────▼────────────────────────────────────────┐    │
│  │                          STORAGE LAYER                                    │    │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐        │    │
│  │  │    OpenSearch    │  │      MinIO       │  │      Redis       │        │    │
│  │  │   (parsed text)  │  │   (raw HTML)     │  │   (state/dedup)  │        │    │
│  │  │                  │  │                  │  │                  │        │    │
│  │  │  - crawled_pages │  │  - html/{hash}   │  │  - frontier:*    │        │    │
│  │  │  - metadata      │  │  - screenshots/  │  │  - seen:*        │        │    │
│  │  │  - links         │  │                  │  │  - robots:*      │        │    │
│  │  └──────────────────┘  └──────────────────┘  └──────────────────┘        │    │
│  └──────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌──────────────────────────────────────────────────────────────────────────┐    │
│  │                         MONITORING                                        │    │
│  │  ┌──────────────────┐  ┌──────────────────┐                              │    │
│  │  │   Next.js UI     │  │   Prometheus     │                              │    │
│  │  │   (Dashboard)    │  │   (Metrics)      │                              │    │
│  │  └──────────────────┘  └──────────────────┘                              │    │
│  └──────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
└───────────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Breakdown

### 1. Control Plane Services

#### Scheduler Service
**Purpose:** Coordinates URL dispatch to workers

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: scheduler
spec:
  replicas: 2  # HA pair
  template:
    spec:
      containers:
      - name: scheduler
        image: crawler/scheduler:latest
        resources:
          limits:
            memory: "512Mi"
            cpu: "500m"
```

**Responsibilities:**
- Monitor worker availability
- Dispatch URLs from frontier to workers
- Enforce global rate limits
- Handle worker health checks

#### Seed API Service
**Purpose:** HTTP API for adding seed URLs

```
POST /api/v1/seeds
{
  "urls": ["https://example.com", "https://site.org"],
  "priority": 0,
  "options": {
    "depth": 3,
    "follow_subdomains": true
  }
}
```

#### Metrics Exporter
**Purpose:** Expose Prometheus metrics

**Key Metrics:**
```
crawler_pages_fetched_total
crawler_queue_depth{host="..."}
crawler_worker_active_count
crawler_errors_total{type="dns|http|parse"}
crawler_fetch_duration_seconds
```

---

### 2. URL Frontier (Redis)

#### Data Structures

```
# Priority Queue (Sorted Set)
ZADD frontier:priority <score> <url_hash>

# Per-Host Queues (Lists)
LPUSH frontier:host:{hostname} <url_json>

# URL Seen (Set + Bloom)
SADD seen:urls <url_hash>
BF.ADD seen:bloom <url_hash>

# Robots Cache (Hash)
HSET robots:{hostname} rules <serialized> expires <timestamp>

# Host Politeness (Hash)
HSET politeness:{hostname} last_fetch <timestamp> delay <ms>
```

#### Frontier Flow

```
                    ┌─────────────────┐
                    │  New URL Input  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ URL Normalize   │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
              ┌─────│ Bloom Filter?   │─────┐
              │     └─────────────────┘     │
           MAYBE                          NO (new)
              │                             │
              ▼                             │
     ┌─────────────────┐                   │
     │ Redis SET Check │                   │
     └────────┬────────┘                   │
              │                             │
        FOUND │ NOT FOUND                   │
              │     │                       │
              │     └───────────────────────┤
              │                             │
              ▼                             ▼
          (skip)                   ┌─────────────────┐
                                   │ Calculate       │
                                   │ Priority Score  │
                                   └────────┬────────┘
                                            │
                                            ▼
                                   ┌─────────────────┐
                                   │ Add to Host Q   │
                                   │ + Priority Set  │
                                   └─────────────────┘
```

---

### 3. Worker Pool

#### Static Content Worker (Colly)

```go
type Worker struct {
    ID          string
    Collector   *colly.Collector
    RedisClient *redis.Client
    Parser      *Parser
}

func (w *Worker) Run(ctx context.Context) {
    for {
        url := w.fetchNextURL(ctx)
        if url == "" {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        w.crawl(ctx, url)
    }
}
```

**Pod Spec:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crawler-worker
spec:
  replicas: 10
  template:
    spec:
      containers:
      - name: worker
        image: crawler/worker:latest
        resources:
          limits:
            memory: "256Mi"
            cpu: "200m"
        env:
        - name: WORKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
```

#### HPA Configuration

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: crawler-worker-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: crawler-worker
  minReplicas: 10
  maxReplicas: 100
  metrics:
  - type: External
    external:
      metric:
        name: redis_frontier_queue_depth
      target:
        type: AverageValue
        averageValue: "1000"
```

---

### 4. Chromedp Pool

**Purpose:** Render JavaScript-heavy pages

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chromedp-pool
spec:
  replicas: 5
  template:
    spec:
      containers:
      - name: chrome
        image: chromedp/headless-shell:latest
        resources:
          limits:
            memory: "1Gi"
            cpu: "1000m"
        ports:
        - containerPort: 9222
```

**Usage Pattern:**
1. Worker detects JS-heavy page (heuristics or config)
2. Sends render request to chromedp pool
3. Receives fully rendered HTML
4. Proceeds with normal parsing

---

### 5. Storage Layer

#### OpenSearch Index

```json
{
  "mappings": {
    "properties": {
      "url": { "type": "keyword" },
      "domain": { "type": "keyword" },
      "title": { "type": "text", "analyzer": "standard" },
      "content": { "type": "text", "analyzer": "standard" },
      "meta_description": { "type": "text" },
      "links_out": { "type": "keyword" },
      "crawled_at": { "type": "date" },
      "content_hash": { "type": "keyword" },
      "simhash": { "type": "long" },
      "depth": { "type": "integer" },
      "status_code": { "type": "integer" }
    }
  }
}
```

#### MinIO Structure

```
bucket: crawler-raw
├── html/
│   ├── {content_hash_prefix}/
│   │   └── {content_hash}.html.gz
├── screenshots/
│   └── {url_hash}.png
└── metadata/
    └── {url_hash}.json
```

---

### 6. Monitoring Dashboard (Next.js)

#### Pages

| Route | Purpose |
|-------|---------|
| `/` | Overview dashboard |
| `/queue` | Frontier queue status |
| `/workers` | Worker health/stats |
| `/domains` | Per-domain statistics |
| `/search` | Search crawled content |
| `/config` | Runtime configuration |

#### API Routes

```typescript
// pages/api/stats.ts
GET /api/stats
{
  "pages_crawled": 1234567,
  "pages_per_second": 98.5,
  "queue_depth": 45000,
  "active_workers": 45,
  "error_rate": 0.02
}

// pages/api/queue/[host].ts
GET /api/queue/example.com
{
  "pending": 150,
  "last_fetch": "2024-01-15T10:30:00Z",
  "politeness_delay_ms": 1000
}
```

---

## Kubernetes Deployment Topology

### Namespace Organization

```
crawler-system/
├── control-plane/
│   ├── scheduler
│   ├── seed-api
│   └── metrics-exporter
├── workers/
│   ├── crawler-worker (HPA)
│   └── chromedp-pool
├── storage/
│   ├── redis (StatefulSet)
│   ├── opensearch (StatefulSet)
│   └── minio (StatefulSet)
└── monitoring/
    ├── next-ui
    └── prometheus
```

### Network Policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: worker-egress
spec:
  podSelector:
    matchLabels:
      app: crawler-worker
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: crawler-system
  - to: []  # Allow external (internet) access
    ports:
    - protocol: TCP
      port: 80
    - protocol: TCP
      port: 443
```

---

## Scaling Strategy

### Single Node (Development)

```yaml
# All-in-one for local kind cluster
scheduler: 1 replica
worker: 3 replicas
chromedp: 1 replica
redis: 1 replica (no persistence)
opensearch: 1 replica (single node)
minio: 1 replica
```

**Expected throughput:** ~5-10 pages/sec

### Multi-Node (Production)

```yaml
scheduler: 2 replicas (HA)
worker: 10-100 replicas (HPA)
chromedp: 5-20 replicas
redis: 3 replicas (cluster mode)
opensearch: 3 replicas (1 master, 2 data)
minio: 4 replicas (distributed)
```

**Expected throughput:** ~100+ pages/sec

### Scaling Triggers

| Metric | Scale Up | Scale Down |
|--------|----------|------------|
| Queue depth | > 1000/worker | < 100/worker |
| CPU utilization | > 70% | < 30% |
| Memory | > 80% | < 40% |
| Error rate | N/A (investigate) | N/A |

---

## Failure Modes and Recovery

### Redis Failure

**Impact:** URL frontier unavailable
**Recovery:**
- Redis Sentinel for automatic failover
- Workers retry with exponential backoff
- In-memory queue buffer (small)

### OpenSearch Failure

**Impact:** Cannot index new content
**Recovery:**
- Queue writes in Redis (temporary)
- Replay queue after recovery
- Multi-node cluster with replicas

### Worker Crash

**Impact:** In-flight URL may be lost
**Recovery:**
- URL marked "in-progress" in Redis with TTL
- Unacked URLs return to queue after timeout
- HPA replaces failed pods

### Network Partition

**Impact:** Workers can't reach targets
**Recovery:**
- Per-host circuit breaker
- Mark affected hosts as degraded
- Continue crawling reachable hosts
