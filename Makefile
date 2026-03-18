.PHONY: all build test clean docker-build docker-push kind-setup kind-load deploy-dev deploy-prod lint fmt

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Binary names
WORKER_BINARY=bin/worker
SCHEDULER_BINARY=bin/scheduler
SEED_API_BINARY=bin/seed-api
CLI_BINARY=bin/crawler-cli

# Docker parameters
DOCKER_REGISTRY?=crawler
VERSION?=latest

# Build all binaries
all: build

build: build-worker build-scheduler build-seed-api build-cli

build-worker:
	$(GOBUILD) -o $(WORKER_BINARY) ./cmd/worker

build-scheduler:
	$(GOBUILD) -o $(SCHEDULER_BINARY) ./cmd/scheduler

build-seed-api:
	$(GOBUILD) -o $(SEED_API_BINARY) ./cmd/seed-api

build-cli:
	$(GOBUILD) -o $(CLI_BINARY) ./cmd/cli

# Run tests
test:
	$(GOTEST) -v -race ./...

test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	$(GOFMT) -s -w .

# Lint code
lint:
	golangci-lint run ./...

# Docker builds
docker-build: docker-build-worker docker-build-scheduler docker-build-seed-api docker-build-cli

docker-build-worker:
	docker build -t $(DOCKER_REGISTRY)/worker:$(VERSION) -f docker/worker.Dockerfile .

docker-build-scheduler:
	docker build -t $(DOCKER_REGISTRY)/scheduler:$(VERSION) -f docker/scheduler.Dockerfile .

docker-build-seed-api:
	docker build -t $(DOCKER_REGISTRY)/seed-api:$(VERSION) -f docker/seed-api.Dockerfile .

docker-build-cli:
	docker build -t $(DOCKER_REGISTRY)/cli:$(VERSION) -f docker/cli.Dockerfile .

# Push to registry
docker-push:
	docker push $(DOCKER_REGISTRY)/worker:$(VERSION)
	docker push $(DOCKER_REGISTRY)/scheduler:$(VERSION)
	docker push $(DOCKER_REGISTRY)/seed-api:$(VERSION)
	docker push $(DOCKER_REGISTRY)/cli:$(VERSION)

# Kind cluster setup
kind-setup:
	kind create cluster --name crawler --config=scripts/kind-config.yaml || true
	kubectl cluster-info --context kind-crawler

kind-delete:
	kind delete cluster --name crawler

# Load images into kind
kind-load: docker-build
	kind load docker-image $(DOCKER_REGISTRY)/worker:$(VERSION) --name crawler
	kind load docker-image $(DOCKER_REGISTRY)/scheduler:$(VERSION) --name crawler
	kind load docker-image $(DOCKER_REGISTRY)/seed-api:$(VERSION) --name crawler
	kind load docker-image $(DOCKER_REGISTRY)/cli:$(VERSION) --name crawler

# Deploy to Kubernetes
deploy-dev:
	kubectl apply -k deploy/overlays/dev

deploy-prod:
	kubectl apply -k deploy/overlays/prod

undeploy:
	kubectl delete -k deploy/overlays/dev --ignore-not-found

# Port forwarding for local development
port-forward-redis:
	kubectl port-forward -n crawler-system svc/redis 6379:6379

port-forward-opensearch:
	kubectl port-forward -n crawler-system svc/opensearch 9200:9200

port-forward-minio:
	kubectl port-forward -n crawler-system svc/minio 9000:9000 9001:9001

port-forward-scheduler:
	kubectl port-forward -n crawler-system svc/scheduler 8081:8081

port-forward-seed-api:
	kubectl port-forward -n crawler-system svc/seed-api 8082:8082

# Logs
logs-worker:
	kubectl logs -n crawler-system -l app=crawler-worker -f

logs-scheduler:
	kubectl logs -n crawler-system -l app=scheduler -f

# Quick start (local kind deployment)
quickstart: kind-setup kind-load deploy-dev
	@echo "Waiting for pods to be ready..."
	kubectl wait --for=condition=ready pod -l app=redis -n crawler-system --timeout=120s
	kubectl wait --for=condition=ready pod -l app=opensearch -n crawler-system --timeout=180s
	kubectl wait --for=condition=ready pod -l app=minio -n crawler-system --timeout=120s
	kubectl wait --for=condition=ready pod -l app=crawler-worker -n crawler-system --timeout=120s
	@echo "Deployment complete! Use 'make port-forward-seed-api' to access the API"

# Add seed URLs
seed:
	@echo "Adding seed URLs..."
	kubectl exec -n crawler-system deploy/seed-api -- /app/seed-api seed add --url "$(URL)"

seed-file:
	kubectl cp $(FILE) crawler-system/seed-api-0:/tmp/seeds.txt
	kubectl exec -n crawler-system deploy/seed-api -- /app/crawler-cli seed add --file /tmp/seeds.txt

# Stats
stats:
	kubectl exec -n crawler-system deploy/scheduler -- wget -qO- http://localhost:8081/api/v1/stats

# Help
help:
	@echo "Web Crawler Makefile"
	@echo ""
	@echo "Build:"
	@echo "  make build          - Build all binaries"
	@echo "  make test           - Run tests"
	@echo "  make lint           - Run linter"
	@echo "  make clean          - Clean build artifacts"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build   - Build all Docker images"
	@echo "  make docker-push    - Push images to registry"
	@echo ""
	@echo "Kubernetes:"
	@echo "  make kind-setup     - Create kind cluster"
	@echo "  make kind-load      - Load images into kind"
	@echo "  make deploy-dev     - Deploy to dev environment"
	@echo "  make quickstart     - Full local deployment"
	@echo ""
	@echo "Operations:"
	@echo "  make logs-worker    - Tail worker logs"
	@echo "  make stats          - Show crawler stats"
