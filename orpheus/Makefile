# Orpheus - local development make targets.

GO   ?= go
APP  := ./cmd/app
BIN  := orpheus
IMG  := olympus/orpheus

# Default connection settings (match docker-compose.yml).
POSTGRES_DSN ?= host=localhost port=15435 user=olympus password=olympus_secret dbname=olympus_orchestration sslmode=disable
PROVISIONER ?= mock

ENV = POSTGRES_DSN='$(POSTGRES_DSN)' PROVISIONER='$(PROVISIONER)'

.PHONY: help build run fmt vet test test-it infra-up infra-down \
        docker-build up down clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Compile the binary
	$(GO) build -o $(BIN) $(APP)
	$(GO) build ./...

run: ## Run the service against the local docker-compose stack (listen :8086)
	$(ENV) $(GO) run $(APP)

fmt: ## Format all Go sources
	$(GO) fmt ./...

vet: ## Static analysis
	$(GO) vet ./...

test: ## Unit tests
	$(GO) test ./cmd/... ./pkg/...

test-it: ## Integration tests (spins up Postgres (+ K3s for the kube test); needs Docker)
	$(GO) test -v ./tests/integration/...

infra-up: ## Boot Postgres via docker-compose
	docker compose up -d

infra-down: ## Stop the docker-compose stack
	docker compose down

up: infra-up ## Alias for infra-up
down: infra-down ## Alias for infra-down

clean: ## Remove the local binary
	rm -f $(BIN)

docker-build: ## Build the service image
	docker build -t $(IMG):latest .