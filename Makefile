.PHONY: build dev test test-sdk test-ui test-integration test-demo-smoke test-demo-e2e test-e2e test-smoke test-browser-full test-full test-all test-everything test-api-smoke lint clean ui demos release docker help sync-openapi

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

# Demo source dependencies (src + build config, not tests)
KANBAN_DEPS := $(shell find examples/kanban/src -type f) \
	examples/kanban/index.html examples/kanban/package-lock.json \
	examples/kanban/vite.config.ts examples/kanban/tsconfig.json \
	examples/kanban/tailwind.config.js examples/kanban/postcss.config.js
POLLS_DEPS := $(shell find examples/live-polls/src -type f) \
	examples/live-polls/index.html examples/live-polls/package-lock.json \
	examples/live-polls/vite.config.ts examples/live-polls/tsconfig.json \
	examples/live-polls/tailwind.config.js examples/live-polls/postcss.config.js

examples/kanban/dist/.stamp: $(KANBAN_DEPS)
	cd examples/kanban && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

examples/live-polls/dist/.stamp: $(POLLS_DEPS)
	cd examples/live-polls && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

build: examples/kanban/dist/.stamp examples/live-polls/dist/.stamp ## Build the ayb binary (rebuilds demos if sources changed)
	go build $(LDFLAGS) -o ayb ./cmd/ayb

dev: ## Build and run with a test database URL (set DATABASE_URL)
	go run $(LDFLAGS) ./cmd/ayb start --database-url "$(DATABASE_URL)"

test: ## Run Go unit tests (no DB, fast)
	go tool gotestsum --format testdox -- -count=1 ./...

test-sdk: ## Run SDK unit tests (vitest, no browser)
	cd sdk && npm test

test-ui: ## Run UI component tests (vitest + jsdom, no browser)
	cd ui && pnpm test

test-integration: ## Run integration tests (uses AYB's managed Postgres — no Docker needed)
	go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./...

test-demo-smoke: ## Run demo smoke tests only — schema apply, tables, RLS, CRUD (needs managed Postgres)
	go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 -run TestDemoSmoke ./internal/e2e/

test-smoke: build ## Run Playwright smoke tests — 8 critical paths, ~5 min (builds + starts server)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test --project=smoke; \

test-browser-full: build ## Run Playwright full browser suite, ~15 min (builds + starts server)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test --project=full; \

test-e2e: build ## Run all Playwright tests — smoke + full (builds + starts server)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test; \

test-all: test test-integration test-sdk test-ui ## Run all fast tests: Go unit + integration + SDK + UI components

test-full: test-all test-e2e ## Run every automated test: unit + integration + SDK + UI + all browser tests (~1.5 hrs)

test-demo-e2e: build ## Run demo app E2E tests — Playwright suites for kanban + live-polls (starts demo, runs tests, stops)
	@cd _dev/manual_smoke_tests && AYB_BIN=$(CURDIR)/ayb bash 18_demo_e2e.test.sh

test-api-smoke: build ## Run API smoke tests against a live server (starts server, runs tests 5-16, stops server)
	@echo "Starting server for API smoke tests..."
	@./ayb start; \
	cd _dev/manual_smoke_tests && ./run_all_tests.sh; \
	RESULT=$$?; \
	../../ayb stop 2>/dev/null || true; \
	exit $$RESULT

test-everything: build ## Run absolutely everything: unit + integration + SDK + UI + browser + API smoke tests
	@failed=""; passed=""; \
	run_step() { \
		printf "\n\033[1;34m━━━ $$1 ━━━\033[0m\n"; \
		if ( eval "$$2" ); then \
			passed="$$passed\n  ✓ $$1"; \
		else \
			failed="$$failed\n  ✗ $$1"; \
		fi; \
	}; \
	run_step "Go unit tests"      "go tool gotestsum --format testdox -- -count=1 ./..."; \
	run_step "Integration tests"  "go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./..."; \
	run_step "SDK tests"          "cd sdk && npm test"; \
	run_step "UI component tests" "cd ui && pnpm test"; \
	run_step "Playwright e2e"     "./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=\$$!; trap \"kill \$$AYB_PID 2>/dev/null\" EXIT; until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; cd ui && npx playwright test"; \
	run_step "Demo app E2E"       "cd _dev/manual_smoke_tests && AYB_BIN=$(CURDIR)/ayb bash 18_demo_e2e.test.sh"; \
	run_step "API smoke tests"    "./ayb start; cd _dev/manual_smoke_tests && ./run_all_tests.sh; R=\$$?; cd ../.. && ./ayb stop 2>/dev/null || true; exit \$$R"; \
	printf "\n\033[1m━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"; \
	printf "\033[1m  TEST SUMMARY\033[0m\n"; \
	printf "\033[1m━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"; \
	if [ -n "$$passed" ]; then printf "\033[32m%b\033[0m\n" "$$passed"; fi; \
	if [ -n "$$failed" ]; then printf "\033[31m%b\033[0m\n" "$$failed"; fi; \
	printf "\033[1m━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"; \
	if [ -n "$$failed" ]; then exit 1; fi

lint: ## Run linters (requires golangci-lint)
	golangci-lint run ./...

ui: ## Build the admin dashboard SPA
	cd ui && pnpm install && pnpm build

demos: ## Build demo apps (force rebuild, pre-built for go:embed)
	cd examples/kanban && npm ci && VITE_AYB_URL="" npx vite build
	cd examples/live-polls && npm ci && VITE_AYB_URL="" npx vite build
	@touch examples/kanban/dist/.stamp examples/live-polls/dist/.stamp

docker: ## Build Docker image locally
	docker build -t allyourbase/ayb:latest -t allyourbase/ayb:$(VERSION) .

clean: ## Remove build artifacts
	rm -f ayb
	rm -rf dist/
	rm -f examples/kanban/dist/.stamp examples/live-polls/dist/.stamp

release: ## Build release binaries via goreleaser (dry run)
	goreleaser release --snapshot --clean

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

sync-openapi: ## Copy OpenAPI spec to docs-site public dir
	cp openapi/openapi.yaml docs-site/public/openapi.yaml
