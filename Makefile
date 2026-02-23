# ekaya-engine Makefile
.PHONY: help install install-hooks install-air clean test fmt lint check run debug build build-ui build-release dev-ui dev-server dev-build-docker ping dev-up dev-down dev-build-container connect-postgres DANGER-recreate-database build-test-image push-test-image pull-test-image build-quickstart run-quickstart push-quickstart

# Variables
# Note: All images are published to GitHub Container Registry
REGISTRY_URL := ghcr.io/ekaya-inc
IMAGE_NAME := ekaya-engine
QUICKSTART_IMAGE_NAME := ekaya-engine-quickstart
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
VERSION_NO_V := $(shell echo $(VERSION) | sed 's/^v//')
PORT ?= 3443

# Build tags for datasource adapters (postgres, all_adapters)
BUILD_TAGS ?= all_adapters

# Auto-detect DOCKER_HOST from Docker context (needed for OrbStack, colima, etc.)
DOCKER_HOST ?= $(shell docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null)
export DOCKER_HOST

# Git commit and date for dev tags
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "no-git")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DATE_TAG := $(shell date +%Y%m%d)

# Full image paths
IMAGE_PATH := $(REGISTRY_URL)/$(IMAGE_NAME)
QUICKSTART_IMAGE_PATH := $(REGISTRY_URL)/$(QUICKSTART_IMAGE_NAME)

# Test container image (used for integration tests)
TEST_IMAGE_NAME := ekaya-engine-test-image
TEST_IMAGE_PATH := $(REGISTRY_URL)/$(TEST_IMAGE_NAME)

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "ekaya-engine - Engine service for Ekaya platform"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'

install: install-hooks install-air ## Install git hooks and air

install-hooks:
	@echo "$(YELLOW)Installing git hooks...$(NC)"
	@./scripts/install-hooks.sh
	@echo "$(GREEN)✓ Git hooks installed$(NC)"

install-air:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	@if [ ! -f .air.toml ]; then \
		echo "Creating .air.toml configuration..."; \
		air init; \
	else \
		echo ".air.toml already exists."; \
	fi

clean: ## Clean build artifacts and Go cache
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	@rm -rf bin/
	@go clean -cache
	@cd ui && npm run clean
	@echo "$(GREEN)✓ Clean complete$(NC)"

test: ## Run all tests including integration tests (requires Docker)
	@echo "$(YELLOW)Running full test suite (unit + integration)...$(NC)"
	@go test -tags="$(BUILD_TAGS)" ./... -v -cover -timeout 5m
	@echo "$(GREEN)✓ All tests passed$(NC)"

test-short: ## Run only unit tests (skip integration tests)
	@echo "$(YELLOW)Running unit tests only...$(NC)"
	@go test -tags="$(BUILD_TAGS)" ./... -short -v -cover -timeout 2m
	@echo "$(GREEN)✓ Unit tests passed$(NC)"

test-integration: ## Run integration tests (requires Docker)
	@echo "$(YELLOW)Running integration tests (requires Docker)...$(NC)"
	@go test -tags="integration,$(BUILD_TAGS)" ./... -timeout 5m
	@echo "$(GREEN)✓ Integration tests passed$(NC)"

test-race: ## Run tests with race detector (slow, not run automatically)
	@echo "$(YELLOW)Running tests with race detector...$(NC)"
	@go test -tags="$(BUILD_TAGS)" -race ./... -v -timeout 10m
	@echo "$(GREEN)✓ Race detection tests passed$(NC)"

fmt: ## Format Go code
	@echo "$(YELLOW)Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

lint: ## Run linter
	@echo "$(YELLOW)Running linter...$(NC)"
	@if which golangci-lint > /dev/null 2>&1; then \
		golangci-lint run --timeout=5m || true; \
		echo "$(GREEN)✓ Linting complete$(NC)"; \
	else \
		echo "$(YELLOW)⚠️  golangci-lint not installed$(NC)"; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

check: ## Run strict quality checks (fails on any issue)
	@echo "$(YELLOW)Running STRICT quality checks...$(NC)"
	@echo ""
	@echo "$(YELLOW)Step 1: Checking go.mod...$(NC)"
	@cp go.mod go.mod.check.tmp
	@cp go.sum go.sum.check.tmp
	@go mod tidy
	@if diff -q go.mod go.mod.check.tmp > /dev/null 2>&1 && diff -q go.sum go.sum.check.tmp > /dev/null 2>&1; then \
		echo "$(GREEN)✓ go.mod is tidy$(NC)"; \
		rm -f go.mod.check.tmp go.sum.check.tmp; \
	else \
		echo "$(RED)❌ go.mod/go.sum not tidy$(NC)"; \
		echo "Run 'go mod tidy' to fix"; \
		rm -f go.mod.check.tmp go.sum.check.tmp; \
		exit 1; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 2: Checking formatting...$(NC)"
	@if gofmt -l . | grep -q .; then \
		echo "$(RED)❌ Formatting issues found:$(NC)"; \
		gofmt -l .; \
		exit 1; \
	else \
		echo "$(GREEN)✓ Code properly formatted$(NC)"; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 3: Running frontend linting...$(NC)"
	@(cd ui && npm run lint) > /dev/null 2>&1; \
	if [ $$? -ne 0 ]; then \
		echo "$(RED)❌ Frontend linting failed:$(NC)"; \
		(cd ui && npm run lint); \
		exit 1; \
	else \
		echo "$(GREEN)✓ No frontend linting issues$(NC)"; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 4: Running TypeScript typecheck...$(NC)"
	@(cd ui && npm run typecheck) > /dev/null 2>&1; \
	if [ $$? -ne 0 ]; then \
		echo "$(RED)❌ TypeScript typecheck failed:$(NC)"; \
		(cd ui && npm run typecheck); \
		exit 1; \
	else \
		echo "$(GREEN)✓ No TypeScript errors$(NC)"; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 5: Building UI...$(NC)"
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@BUILD_OUTPUT=$$(cd ui && npm run build 2>&1); \
	BUILD_EXIT_CODE=$$?; \
	if [ $$BUILD_EXIT_CODE -ne 0 ]; then \
		echo "$(RED)❌ UI build failed:$(NC)"; \
		echo "$$BUILD_OUTPUT"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ UI built$(NC)"; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 6: Running strict linter...$(NC)"
	@if which golangci-lint > /dev/null 2>&1; then \
		LINT_OUTPUT=$$(golangci-lint run --timeout=5m 2>&1); \
		LINT_EXIT_CODE=$$?; \
		if [ $$LINT_EXIT_CODE -ne 0 ]; then \
			echo "$$LINT_OUTPUT"; \
			exit 1; \
		else \
			echo "$(GREEN)✓ No linting issues$(NC)"; \
		fi \
	else \
		echo "$(RED)❌ golangci-lint not installed$(NC)"; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 7: Running backend unit tests...$(NC)"
	@TEST_OUTPUT=$$(go test -tags="$(BUILD_TAGS)" ./... -short -cover -timeout 5m 2>&1); \
	TEST_EXIT_CODE=$$?; \
	if [ $$TEST_EXIT_CODE -ne 0 ]; then \
		echo "$(RED)❌ Backend tests failed:$(NC)"; \
		echo "$$TEST_OUTPUT"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ All backend tests passed$(NC)"; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 8: Running frontend tests...$(NC)"
	@(cd ui && npm test -- --run) > /dev/null 2>&1; \
	if [ $$? -ne 0 ]; then \
		echo "$(RED)❌ Frontend tests failed:$(NC)"; \
		(cd ui && npm test -- --run); \
		exit 1; \
	else \
		echo "$(GREEN)✓ All frontend tests passed$(NC)"; \
	fi
	@echo ""
	@echo "$(YELLOW)Step 9: Running integration tests (requires Docker)...$(NC)"
	@TEST_OUTPUT=$$(go test -tags="integration,$(BUILD_TAGS)" ./... -timeout 5m 2>&1); \
	TEST_EXIT_CODE=$$?; \
	if [ $$TEST_EXIT_CODE -ne 0 ]; then \
		echo "$(RED)❌ Integration tests failed:$(NC)"; \
		echo "$$TEST_OUTPUT"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ All integration tests passed$(NC)"; \
	fi
	@echo ""
	@echo "$(GREEN)✅ All strict checks passed!$(NC)"

run: ## Build website and run the server (no watch)
	@echo "$(YELLOW)Building UI...$(NC)"
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@cd ui && npm run build
	@echo "$(GREEN)✓ UI built$(NC)"
	@echo "$(YELLOW)Starting ekaya-engine on port $(PORT)...$(NC)"
	@PORT=$(PORT) go run -tags="$(BUILD_TAGS)" main.go

debug: ## Build and run with LLM conversation logging enabled
	@echo "$(YELLOW)Building UI...$(NC)"
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@cd ui && npm run build
	@echo "$(GREEN)✓ UI built$(NC)"
	@echo "$(YELLOW)Starting ekaya-engine in DEBUG mode on port $(PORT)...$(NC)"
	@PORT=$(PORT) go run -tags="debug,$(BUILD_TAGS)" main.go

build: check ## Build binary to bin/ekaya-engine
	@echo "$(YELLOW)Building UI...$(NC)"
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@cd ui && npm run build
	@echo "$(GREEN)✓ UI built$(NC)"
	@echo "$(YELLOW)Building ekaya-engine binary...$(NC)"
	@mkdir -p bin
	@go build -tags="$(BUILD_TAGS)" -ldflags="-X main.Version=$(VERSION)" -o bin/ekaya-engine .
	@echo "$(GREEN)✓ Binary built: bin/ekaya-engine$(NC)"

dev-ui: ## Watch UI files and rebuild to dist/ for Go server
	@echo "$(YELLOW)Watching UI files and rebuilding to ui/dist/...$(NC)"
	@echo "Changes will be served by the Go server on http://localhost:3443"
	@echo "Refresh browser to see changes."
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@cd ui && npm run watch

dev-server: ## Start development mode with auto-reload (using air)
	@echo "$(YELLOW)Building UI for Go server...$(NC)"
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@cd ui && npm run build
	@echo "$(GREEN)✓ UI built and available at http://localhost:3443$(NC)"
	@echo "$(YELLOW)Starting development mode with auto-reload...$(NC)"
	@echo "The server will restart automatically when you save changes."
	@echo "$(YELLOW)Run 'make dev-ui' separately for hot reload at http://localhost:5173$(NC)"
	@echo "$(YELLOW)Starting Go server...$(NC)"
	@air -c .air.toml

dev-build-docker: ## Build Docker image locally for testing (no push)
	@echo "$(YELLOW)Building Docker image locally...$(NC)"
	@echo "This will build using the same multi-stage process as CI/CD"
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TAGS=$(BUILD_TAGS) \
		-t $(IMAGE_NAME):local \
		.
	@echo "$(GREEN)✓ Docker image built: $(IMAGE_NAME):local$(NC)"
	@echo "Run with: docker run -p 3443:3443 $(IMAGE_NAME):local"

# Development helpers


# Quick test of the ping endpoint
ping: ## Test the /ping endpoint locally and on deployed environments
	@echo "$(YELLOW)Testing /ping endpoints...$(NC)"
	@echo ""
	@echo "Local (http://localhost:$(PORT)/ping):"
	@curl -s http://localhost:$(PORT)/ping | jq . 2>/dev/null || echo "  Local service not running"
	@echo ""
	@echo "Dev (https://us-central1.dev.ekaya.ai/api/v1/ping):"
	@curl -s https://us-central1.dev.ekaya.ai/api/v1/ping | jq . 2>/dev/null || echo "  Dev service not accessible"
	@echo ""
	@echo "Production (https://us-central1.ekaya.ai/api/v1/ping):"
	@curl -s https://us-central1.ekaya.ai/api/v1/ping | jq . 2>/dev/null || echo "  Production service not accessible"
	@echo ""
	@echo "$(GREEN)✓ Ping tests complete$(NC)"

# Development environment commands
dev-up: ## Start Ekaya Engine + PostgreSQL via deploy/docker
	@if [ ! -f deploy/docker/config.yaml ]; then \
		echo "$(RED)Error: deploy/docker/config.yaml not found$(NC)"; \
		echo "Run: cp config.yaml.example deploy/docker/config.yaml"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Starting Ekaya Engine + PostgreSQL...$(NC)"
	@docker-compose -f deploy/docker/docker-compose.yml up -d --build
	@echo "$(GREEN)✓ Ekaya Engine available at: https://localhost:3443$(NC)"

dev-down: ## Stop Ekaya Engine + PostgreSQL containers
	@echo "$(YELLOW)Stopping containers...$(NC)"
	@docker-compose -f deploy/docker/docker-compose.yml down
	@echo "$(GREEN)✓ Environment stopped$(NC)"

connect-postgres: ## Connect to PostgreSQL via psql (uses .env variables)
	@PGPASSWORD=$${PGPASSWORD} psql \
		-h $${PGHOST:-localhost} \
		-p $${PGPORT:-5432} \
		-U $${PGUSER:-ekaya} \
		-d $${PGDATABASE:-ekaya_engine}

# DANGEROUS: Drop and recreate the engine database
# Uses the default system PostgreSQL user (not $$PGUSER) for admin operations,
# then creates/updates the application role ($$PGUSER) as non-superuser with RLS enforcement.
DANGER-recreate-database: ## Drop and recreate $${PGDATABASE:-ekaya_engine} (requires CONFIRM=YES)
	@echo "$(RED)⚠️  This will DROP and RECREATE the database '$${PGDATABASE:-ekaya_engine}' on $${PGHOST:-localhost}:$${PGPORT:-5432}$(NC)"
	@[ "$${CONFIRM}" = "YES" ] || (echo "$(RED)Aborting. Re-run with CONFIRM=YES$(NC)"; exit 1)
	@APP_USER="$${PGUSER:-ekaya}" && \
	APP_PASS="$${PGPASSWORD}" && \
	DB_NAME="$${PGDATABASE:-ekaya_engine}" && \
	DB_HOST="$${PGHOST:-localhost}" && \
	DB_PORT="$${PGPORT:-5432}" && \
	unset PGUSER PGPASSWORD && \
	echo "$(YELLOW)Terminating active sessions...$(NC)" && \
	psql -v ON_ERROR_STOP=1 -h "$$DB_HOST" -p "$$DB_PORT" -d postgres \
		-c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$$DB_NAME' AND pid <> pg_backend_pid();" && \
	echo "$(YELLOW)Dropping database if it exists...$(NC)" && \
	psql -v ON_ERROR_STOP=1 -h "$$DB_HOST" -p "$$DB_PORT" -d postgres \
		-c "DROP DATABASE IF EXISTS \"$$DB_NAME\";" && \
	echo "$(YELLOW)Creating database...$(NC)" && \
	psql -v ON_ERROR_STOP=1 -h "$$DB_HOST" -p "$$DB_PORT" -d postgres \
		-c "CREATE DATABASE \"$$DB_NAME\";" && \
	echo "$(YELLOW)Setting up application role '$$APP_USER' (non-superuser, RLS enforced)...$(NC)" && \
	(psql -h "$$DB_HOST" -p "$$DB_PORT" -d postgres \
		-c "CREATE ROLE \"$$APP_USER\" WITH LOGIN PASSWORD '$$APP_PASS' NOSUPERUSER NOBYPASSRLS;" 2>/dev/null || true) && \
	psql -v ON_ERROR_STOP=1 -h "$$DB_HOST" -p "$$DB_PORT" -d postgres \
		-c "ALTER ROLE \"$$APP_USER\" WITH LOGIN PASSWORD '$$APP_PASS' NOSUPERUSER NOBYPASSRLS;" \
		-c "GRANT ALL PRIVILEGES ON DATABASE \"$$DB_NAME\" TO \"$$APP_USER\";" && \
	psql -v ON_ERROR_STOP=1 -h "$$DB_HOST" -p "$$DB_PORT" -d "$$DB_NAME" \
		-c "GRANT ALL ON SCHEMA public TO \"$$APP_USER\";" && \
	echo "$(GREEN)✓ Recreated database: $$DB_NAME (role '$$APP_USER' configured with RLS enforcement)$(NC)"

dev-build-container: ## Build devcontainer environment
	@echo "$(YELLOW)Building devcontainer environment...$(NC)"
	@docker-compose -f docker-compose.dev.yml -f .devcontainer/docker-compose.devcontainer.yml build
	@echo "$(GREEN)✓ Devcontainer built$(NC)"

# Test database image targets
build-test-image: ## Build the ekaya-engine-test-image for integration tests (local arch only)
	@echo "$(YELLOW)Building test database image...$(NC)"
	@docker build -t $(TEST_IMAGE_NAME):latest test/docker/engine-test-db/
	@docker tag $(TEST_IMAGE_NAME):latest $(TEST_IMAGE_PATH):latest
	@echo "$(GREEN)✓ Test image built: $(TEST_IMAGE_NAME):latest$(NC)"
	@echo "$(GREEN)✓ Tagged as: $(TEST_IMAGE_PATH):latest$(NC)"

push-test-image: ## Build and push multi-arch test image to ghcr.io (requires GITHUB_TOKEN)
	@echo "$(YELLOW)Building and pushing multi-arch test image to ghcr.io...$(NC)"
	@if [ -z "$$GITHUB_TOKEN" ]; then echo "$(RED)Error: GITHUB_TOKEN not set$(NC)"; exit 1; fi
	@echo "$$GITHUB_TOKEN" | docker login ghcr.io -u $(shell git config user.email || echo "user") --password-stdin
	@docker buildx create --name multiarch --driver docker-container --use 2>/dev/null || docker buildx use multiarch
	@docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(TEST_IMAGE_PATH):latest \
		--push \
		test/docker/engine-test-db/
	@echo "$(GREEN)✓ Test image pushed to: $(TEST_IMAGE_PATH):latest (linux/amd64, linux/arm64)$(NC)"

pull-test-image: ## Pull test image from ghcr.io
	@echo "$(YELLOW)Pulling test image from ghcr.io...$(NC)"
	@docker pull $(TEST_IMAGE_PATH):latest
	@echo "$(GREEN)✓ Test image pulled: $(TEST_IMAGE_PATH):latest$(NC)"

# Quickstart image targets
build-quickstart: ## Build the all-in-one quickstart Docker image
	@echo "$(YELLOW)Building quickstart image...$(NC)"
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TAGS=$(BUILD_TAGS) \
		-f deploy/quickstart/Dockerfile \
		-t $(QUICKSTART_IMAGE_NAME):local \
		-t $(QUICKSTART_IMAGE_PATH):$(VERSION) \
		-t $(QUICKSTART_IMAGE_PATH):latest \
		.
	@echo "$(GREEN)✓ Quickstart image built: $(QUICKSTART_IMAGE_NAME):local$(NC)"
	@echo "$(GREEN)✓ Tagged as: $(QUICKSTART_IMAGE_PATH):$(VERSION)$(NC)"
	@echo "$(GREEN)✓ Tagged as: $(QUICKSTART_IMAGE_PATH):latest$(NC)"
	@echo ""
	@echo "Run with:"
	@echo "  docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data $(QUICKSTART_IMAGE_NAME):local"

run-quickstart: ## Pull and run the quickstart image from ghcr.io
	@echo "$(YELLOW)Starting quickstart container...$(NC)"
	@docker run --pull always -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data $(QUICKSTART_IMAGE_PATH):latest

push-quickstart: build-quickstart ## Build and push quickstart image to ghcr.io (requires GITHUB_TOKEN)
	@echo "$(YELLOW)Pushing quickstart image to ghcr.io...$(NC)"
	@if [ -z "$$GITHUB_TOKEN" ]; then echo "$(RED)Error: GITHUB_TOKEN not set$(NC)"; exit 1; fi
	@echo "$$GITHUB_TOKEN" | docker login ghcr.io -u $(shell git config user.email || echo "user") --password-stdin
	@docker push $(QUICKSTART_IMAGE_PATH):$(VERSION)
	@docker push $(QUICKSTART_IMAGE_PATH):latest
	@echo "$(GREEN)✓ Quickstart image pushed to: $(QUICKSTART_IMAGE_PATH):$(VERSION)$(NC)"
	@echo "$(GREEN)✓ Quickstart image pushed to: $(QUICKSTART_IMAGE_PATH):latest$(NC)"


# Release binary targets (cross-compilation)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64
RELEASE_DIR := dist

build-ui: ## Build the frontend UI (prerequisite for binary builds)
	@echo "$(YELLOW)Building UI...$(NC)"
	@cd ui && if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then echo "$(YELLOW)Installing UI dependencies...$(NC)" && npm install; fi
	@cd ui && npm run build
	@echo "$(GREEN)✓ UI built$(NC)"

build-release: build-ui ## Build release binaries for all platforms
	@echo "$(YELLOW)Building release binaries for all platforms...$(NC)"
	@mkdir -p $(RELEASE_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		OUTPUT_NAME=ekaya-engine; \
		ARCHIVE_NAME=ekaya-engine_$(VERSION_NO_V)_$${GOOS}_$${GOARCH}; \
		if [ "$$GOOS" = "windows" ]; then OUTPUT_NAME=ekaya-engine.exe; fi; \
		echo "  Building $$GOOS/$$GOARCH..."; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH go build \
			-tags="$(BUILD_TAGS)" \
			-trimpath \
			-ldflags="-s -w -X main.Version=$(VERSION)" \
			-o $(RELEASE_DIR)/$$ARCHIVE_NAME/$$OUTPUT_NAME . || exit 1; \
		cp config.yaml.example $(RELEASE_DIR)/$$ARCHIVE_NAME/config.yaml.example; \
		cp LICENSE $(RELEASE_DIR)/$$ARCHIVE_NAME/ 2>/dev/null || true; \
		if [ "$$GOOS" = "windows" ]; then \
			(cd $(RELEASE_DIR) && zip -q $$ARCHIVE_NAME.zip -r $$ARCHIVE_NAME/); \
		else \
			tar -czf $(RELEASE_DIR)/$$ARCHIVE_NAME.tar.gz -C $(RELEASE_DIR) $$ARCHIVE_NAME; \
		fi; \
		rm -rf $(RELEASE_DIR)/$$ARCHIVE_NAME; \
	done
	@echo ""
	@echo "$(YELLOW)Generating checksums...$(NC)"
	@cd $(RELEASE_DIR) && shasum -a 256 *.tar.gz *.zip 2>/dev/null > checksums.txt
	@echo "$(GREEN)✓ Release binaries built in $(RELEASE_DIR)/$(NC)"
	@ls -lh $(RELEASE_DIR)/

.DEFAULT_GOAL := help
