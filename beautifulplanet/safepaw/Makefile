.PHONY: all build test lint security vulncheck fuzz clean docker up down verify help install-hooks

GATEWAY_DIR := services/gateway
WIZARD_DIR  := services/wizard

all: build test lint ## Build, test, and lint everything

# ── Build ──────────────────────────────────────────────

build: build-gateway build-wizard ## Build all Go services

build-gateway:
	@echo "=== Building gateway ==="
	cd $(GATEWAY_DIR) && go build -v ./...

build-wizard:
	@echo "=== Building wizard ==="
	cd $(WIZARD_DIR) && go build -v ./...

# ── Test ───────────────────────────────────────────────

test: test-gateway test-wizard ## Run all tests with race detector
	@echo ""
	@echo "=== All tests passed ==="

test-gateway:
	@echo "=== Testing gateway ==="
	cd $(GATEWAY_DIR) && go test -race -count=1 ./...

test-wizard:
	@echo "=== Testing wizard ==="
	cd $(WIZARD_DIR) && go test -race -count=1 ./...

cover: ## Generate coverage report
	@echo "=== Gateway coverage ==="
	cd $(GATEWAY_DIR) && go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
	@echo ""
	@echo "=== Wizard coverage ==="
	cd $(WIZARD_DIR) && go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# ── Lint ───────────────────────────────────────────────

lint: lint-gateway lint-wizard ## Run golangci-lint on all services

lint-gateway:
	@echo "=== Linting gateway ==="
	cd $(GATEWAY_DIR) && golangci-lint run --timeout=5m

lint-wizard:
	@echo "=== Linting wizard ==="
	cd $(WIZARD_DIR) && golangci-lint run --timeout=5m

# ── Security ───────────────────────────────────────────

security: ## Run gosec security scanner
	@echo "=== Security scan: gateway ==="
	cd $(GATEWAY_DIR) && gosec -exclude-dir=tools ./...
	@echo ""
	@echo "=== Security scan: wizard ==="
	cd $(WIZARD_DIR) && gosec ./...

vulncheck: ## Run govulncheck for known CVEs
	@echo "=== Vulnerability check: gateway ==="
	cd $(GATEWAY_DIR) && govulncheck ./...
	@echo ""
	@echo "=== Vulnerability check: wizard ==="
	cd $(WIZARD_DIR) && govulncheck ./...

fuzz: ## Run fuzz tests (seed corpus, 30s each if -fuzztime set)
	@echo "=== Fuzz tests: gateway ==="
	cd $(GATEWAY_DIR) && go test -run "Fuzz" -v ./...

# ── Docker ─────────────────────────────────────────────

docker: ## Build Docker images
	docker compose build

up: ## Start all services
	docker compose up -d

down: ## Stop all services
	docker compose down

up-monitoring: ## Start with Prometheus + Grafana
	docker compose -f docker-compose.yml -f monitoring/docker-compose.monitoring.yml up -d

# ── E2E ────────────────────────────────────────────────

verify: ## Run E2E verification against live deployment
	./scripts/verify-deployment.sh

verify-gateway-mock: ## Run gateway + mock backend integration (no OpenClaw)
	./scripts/integration-gateway-mock.sh

# ── Git Hooks ──────────────────────────────────────────

install-hooks: ## Install pre-commit hook
	@ln -sf ../../beautifulplanet/safepaw/scripts/pre-commit.sh $(shell git rev-parse --show-toplevel)/.git/hooks/pre-commit
	@echo "Pre-commit hook installed."

# ── Clean ──────────────────────────────────────────────

clean: ## Remove build artifacts and coverage files
	rm -f $(GATEWAY_DIR)/coverage.out $(WIZARD_DIR)/coverage.out
	rm -f $(GATEWAY_DIR)/gateway $(WIZARD_DIR)/wizard
	rm -f services/mockbackend/mockbackend

# ── Help ───────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
