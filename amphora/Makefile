# Amphora - local development make targets.

GO      ?= go
APP     := ./cmd/app
BIN     := amphora
IMG     := olympus/olympus

# Default connection settings (match docker-compose.yml).
POSTGRES_DSN  ?= host=localhost port=15432 user=olympus password=olympus_secret dbname=olympus_storage sslmode=disable
REDIS_URL     ?= localhost:16379
MINIO_ENDPOINT?= localhost:9000
MINIO_ACCESS  ?= olympus_admin
MINIO_SECRET  ?= olympus_secret_admin
MINIO_BUCKET  ?= olympus
BACKEND       ?= minio

ENV = STORAGE_BACKEND=$(BACKEND) POSTGRES_DSN='$(POSTGRES_DSN)' REDIS_URL=$(REDIS_URL) \
      MINIO_ENDPOINT=$(MINIO_ENDPOINT) MINIO_ACCESS_KEY=$(MINIO_ACCESS) \
      MINIO_SECRET_KEY=$(MINIO_SECRET) MINIO_BUCKET=$(MINIO_BUCKET)

.PHONY: help build run fmt vet test test-it infra-up infra-down \
        reconcile reconcile-repair backfill-etag docker-build up down image

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Compile the binary
	$(GO) build -o $(BIN) $(APP)
	$(GO) build ./...

run: ## Run the gateway against the local docker-compose stack
	$(ENV) $(GO) run $(APP)

fmt: ## Format all Go sources
	$(GO) fmt ./...

vet: ## Static analysis
	$(GO) vet ./...

test: ## Unit tests (cmd/app)
	$(GO) test ./cmd/app/...

test-it: ## Integration tests (spins up Postgres+Redis+MinIO via testcontainers; needs Docker)
	$(GO) test -v ./tests/integration/...

infra-up: ## Boot Postgres, Redis, MinIO via docker-compose
	docker compose up -d

infra-down: ## Stop the docker-compose stack
	docker compose down

up: infra-up ## Alias for infra-up
down: infra-down ## Alias for infra-down

reconcile: ## Reconciliation report only (no changes)
	$(ENV) $(GO) run $(APP) -reconcile

reconcile-repair: ## Reconcile and repair (mark missing + fix drift)
	$(ENV) $(GO) run $(APP) -reconcile -reconcile-mark-missing -reconcile-drift

backfill-etag: ## Stamp x-olympus-etag on pre-existing MinIO objects
	$(ENV) $(GO) run $(APP) -backfill-etag

clean: ## Remove the local binary
	rm -f $(BIN)

docker-build: ## Build the gateway image
	docker build -t $(IMG):latest .

# Tiny alias so `make image` also works.
image: docker-build ## Alias for docker-build