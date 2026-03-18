# Web Crawler System - Overview

## Purpose

A distributed web crawler system designed for scalable, polite web crawling with search indexing capabilities. The system fetches, parses, deduplicates, and indexes web content while respecting robots.txt rules and rate limits.

## Goals

- **Scalability**: Support ~100 pages/sec with horizontal scaling
- **Politeness**: Per-host rate limiting, robots.txt compliance
- **Reliability**: Fault-tolerant, resumable crawl state
- **Extensibility**: Modular design for custom parsers/handlers
- **Observability**: Real-time monitoring and metrics

## Key Features

| Feature | Description |
|---------|-------------|
| Distributed Crawling | Multiple stateless workers coordinated via Redis |
| URL Deduplication | Bloom filter-based URL seen detection |
| Content Deduplication | SimHash for near-duplicate content detection |
| JS Rendering | chromedp pool for dynamic/SPA content |
| Full-Text Search | OpenSearch indexing with parsed content |
| Raw Storage | MinIO for original HTML archival |
| Monitoring Dashboard | Real-time Next.js UI for crawl metrics |

## Technology Stack

### Core Services (Go)
- **Colly** - Primary web scraping framework
- **goquery** - HTML DOM parsing and traversal
- **chromedp** - Headless Chrome for JS-heavy pages
- **go-redis** - Redis client for queue operations

### Infrastructure
| Component | Technology |
|-----------|------------|
| Queue/Frontier | Redis |
| Search Index | OpenSearch |
| Blob Storage | MinIO |
| Orchestration | Kubernetes (kind cluster) |
| Monitoring UI | Next.js |

### Deployment
- Kubernetes with Horizontal Pod Autoscaler (HPA)
- Stateless workers for easy scaling
- Persistent storage for Redis, OpenSearch, MinIO

## Use Cases

1. **Search Indexing** - Build searchable index of web content
2. **Content Archival** - Store historical snapshots of web pages
3. **Change Monitoring** - Detect content changes over time
4. **Data Extraction** - Structured data scraping from websites
5. **Link Analysis** - Map site structure and relationships

## System Boundaries

### In Scope
- HTML content crawling and parsing
- Text extraction and indexing
- Link discovery and following
- Robots.txt compliance
- Rate limiting per host

### Out of Scope (v1)
- Media file (image/video/PDF) processing
- JavaScript execution for all pages (selective only)
- Authentication/login-required pages
- CAPTCHA solving
- Proxy rotation

## Performance Targets

| Metric | Target |
|--------|--------|
| Crawl Rate | ~100 pages/sec |
| Worker Pods | 10-50 (auto-scaled) |
| URL Frontier Capacity | 10M+ URLs |
| Content Latency | < 5s from fetch to index |
| Dedup False Positive | < 0.1% |

## Quick Start

```bash
# Start local kind cluster
kind create cluster --name crawler

# Deploy infrastructure
kubectl apply -k deploy/base

# Add seed URLs
kubectl exec -it crawler-api -- /app/cli seed --file seeds.txt

# Monitor dashboard
kubectl port-forward svc/monitoring-ui 3000:3000
```
