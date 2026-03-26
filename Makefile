.PHONY: build dev test test-sdk test-ui test-integration test-demo-smoke test-demo-e2e test-e2e test-smoke test-browser-full test-full test-all test-everything test-api-smoke lint check check-sizes check-ui-lint check-browser-tests-lint check-func-sizes check-installer check-sync-pipeline check-sdk-build release-candidate-check clean ui demos release docker docker-runtime-smoke help sync-openapi build-postgres load-admin-status load-admin-status-local load-auth-request-path load-auth-request-path-local load-data-path load-data-path-local load-data-pool-pressure load-data-pool-pressure-local load-http-100 load-http-500 load-http-1000 load-realtime-ws load-realtime-ws-local load-realtime-ws-1000 load-realtime-ws-5000 load-realtime-ws-10000 load-sustained-soak load-sustained-soak-local

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

LOAD_K6_BIN ?= k6
LOAD_DEFAULT_VUS ?= 1
LOAD_DEFAULT_ITERATIONS ?= 1
LOAD_DEFAULT_BASE_URL ?= http://127.0.0.1:8090
LOAD_ADMIN_STATUS_SCENARIO := tests/load/scenarios/admin_status.js
LOAD_AUTH_REQUEST_PATH_SCENARIO := tests/load/scenarios/auth_register_login_refresh.js
LOAD_DATA_PATH_SCENARIO := tests/load/scenarios/data_path_crud_batch.js
LOAD_DATA_POOL_PRESSURE_SCENARIO := tests/load/scenarios/data_pool_pressure.js
LOAD_REALTIME_WS_SCENARIO := tests/load/scenarios/realtime_ws_subscribe.js
LOAD_SUSTAINED_SOAK_SCENARIO := tests/load/scenarios/sustained_soak.js
LOAD_AUTH_ENABLED_DEFAULT := true
LOAD_AUTH_RATE_LIMIT_DEFAULT := 10000
LOAD_API_RATE_LIMIT_DEFAULT := 10000/min
LOAD_API_ANON_RATE_LIMIT_DEFAULT := 10000/min
BROWSER_AUTH_ENABLED_DEFAULT := true
LOAD_ADMIN_STATUS_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_ADMIN_STATUS_SCENARIO)
LOAD_AUTH_REQUEST_PATH_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_AUTH_REQUEST_PATH_SCENARIO)
LOAD_DATA_PATH_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_DATA_PATH_SCENARIO)
LOAD_DATA_POOL_PRESSURE_K6_COMMAND := AYB_POOL_PRESSURE_VUS=$${K6_VUS:-$(LOAD_DEFAULT_VUS)} AYB_POOL_PRESSURE_ITERATIONS=$${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} env -u K6_VUS -u K6_ITERATIONS $(LOAD_K6_BIN) run $(LOAD_DATA_POOL_PRESSURE_SCENARIO)
LOAD_REALTIME_WS_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_REALTIME_WS_SCENARIO)
LOAD_SUSTAINED_SOAK_K6_COMMAND := $(LOAD_K6_BIN) run $(LOAD_SUSTAINED_SOAK_SCENARIO)

define LOAD_BOOTSTRAP_FUNCTIONS
set -euo pipefail; \
load_export_env() { \
	export AYB_AUTH_RATE_LIMIT="$${AYB_AUTH_RATE_LIMIT:-$(LOAD_AUTH_RATE_LIMIT_DEFAULT)}"; \
	export AYB_RATE_LIMIT_API="$${AYB_RATE_LIMIT_API:-$(LOAD_API_RATE_LIMIT_DEFAULT)}"; \
	export AYB_RATE_LIMIT_API_ANONYMOUS="$${AYB_RATE_LIMIT_API_ANONYMOUS:-$(LOAD_API_ANON_RATE_LIMIT_DEFAULT)}"; \
	export AYB_BASE_URL="$${AYB_BASE_URL:-$(LOAD_DEFAULT_BASE_URL)}"; \
}; \
load_export_auth_env() { \
	local resolved_auth_jwt_secret="$${AYB_AUTH_JWT_SECRET:-}"; \
	if [ -z "$$resolved_auth_jwt_secret" ]; then \
		resolved_auth_jwt_secret="$$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")"; \
	fi; \
	export AYB_AUTH_ENABLED="$${AYB_AUTH_ENABLED:-$(LOAD_AUTH_ENABLED_DEFAULT)}"; \
	export AYB_AUTH_JWT_SECRET="$$resolved_auth_jwt_secret"; \
}; \
load_resolve_admin_token() { \
	local resolved_admin_token="$${AYB_ADMIN_TOKEN:-}"; \
	if [ -z "$$resolved_admin_token" ] && [ -f "$${HOME}/.ayb/admin-token" ]; then \
		local admin_password login_payload login_response; \
		admin_password="$$(head -n 1 "$${HOME}/.ayb/admin-token" | sed 's/\r$$//')"; \
		if [ -n "$$admin_password" ]; then \
			login_payload="$$(python3 -c "import json,sys; print(json.dumps(dict(password=sys.argv[1])))" "$$admin_password")"; \
			login_response="$$(curl -fsS -H "Content-Type: application/json" --data "$$login_payload" "$${AYB_BASE_URL%/}/api/admin/auth" 2>/dev/null || true)"; \
			if [ -n "$$login_response" ]; then \
				resolved_admin_token="$$(printf "%s" "$$login_response" | python3 -c "import json,sys; print(json.load(sys.stdin).get(\"token\", \"\"))" 2>/dev/null || true)"; \
			fi; \
		fi; \
	fi; \
	export AYB_ADMIN_TOKEN="$$resolved_admin_token"; \
}
endef

define BROWSER_EXPORT_AUTH_ENV
set -euo pipefail; \
export AYB_AUTH_ENABLED="$${AYB_AUTH_ENABLED:-$(BROWSER_AUTH_ENABLED_DEFAULT)}"; \
if [ -z "$${AYB_AUTH_JWT_SECRET:-}" ]; then \
	export AYB_AUTH_JWT_SECRET="$$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")"; \
fi
endef

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

# Demo source dependencies (src + build config, not tests)
KANBAN_DEPS := $(shell find examples/kanban/src -type f) \
	examples/kanban/index.html examples/kanban/package-lock.json \
	examples/kanban/vite.config.ts examples/kanban/tsconfig.json \
	examples/kanban/tailwind.config.js examples/kanban/postcss.config.js
POLLS_DEPS := $(shell find examples/live-polls/src -type f) \
	examples/live-polls/index.html examples/live-polls/package-lock.json \
	examples/live-polls/vite.config.ts examples/live-polls/tsconfig.json \
	examples/live-polls/tailwind.config.js examples/live-polls/postcss.config.js
UI_DEPS := $(shell find ui/src -type f) \
	ui/index.html ui/package.json ui/pnpm-lock.yaml \
	ui/vite.config.ts ui/tsconfig.json ui/postcss.config.js ui/tailwind.config.ts

examples/kanban/dist/.stamp: $(KANBAN_DEPS)
	cd examples/kanban && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

examples/live-polls/dist/.stamp: $(POLLS_DEPS)
	cd examples/live-polls && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

ui/dist/.stamp: $(UI_DEPS)
	cd ui && pnpm install && pnpm build
	@touch $@

build: ui/dist/.stamp examples/kanban/dist/.stamp examples/live-polls/dist/.stamp ## Build the ayb binary (rebuilds UI + demos if sources changed)
	go build $(LDFLAGS) -o ayb ./cmd/ayb

build-postgres: ## Build AYB-managed Postgres binaries for the current platform
	bash scripts/build-postgres.sh

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
	@bash scripts/run-with-ayb.sh 'cd ui && npm run test:browser -- --project=smoke'

test-browser-full: build ## Run Playwright full browser suite, ~15 min (builds + starts server)
	@bash -lc '$(BROWSER_EXPORT_AUTH_ENV); bash scripts/run-with-ayb.sh "cd ui && npm run test:browser -- --project=full"'

test-e2e: build ## Run all Playwright tests — smoke + full (builds + starts server)
	@bash -lc '$(BROWSER_EXPORT_AUTH_ENV); bash scripts/run-with-ayb.sh "cd ui && npm run test:browser"'

load-admin-status: ## Run direct k6 baseline scenario against AYB_BASE_URL (default http://127.0.0.1:8090)
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_resolve_admin_token; $(LOAD_ADMIN_STATUS_K6_COMMAND)'

load-admin-status-local: ## Start local AYB with run-with-ayb and run the baseline k6 scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_resolve_admin_token; load_export_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token; $(LOAD_ADMIN_STATUS_K6_COMMAND)"'

load-auth-request-path: ## Run direct k6 auth register/login/refresh scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_AUTH_REQUEST_PATH_K6_COMMAND)'

load-auth-request-path-local: ## Start local AYB with run-with-ayb and run the auth register/login/refresh scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_resolve_admin_token; load_export_env; load_export_auth_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token; $(LOAD_AUTH_REQUEST_PATH_K6_COMMAND)"'

load-data-path: ## Run direct k6 collection CRUD/batch data-path scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_DATA_PATH_K6_COMMAND)'

load-data-path-local: ## Start local AYB with run-with-ayb and run the collection CRUD/batch data-path scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_resolve_admin_token; load_export_env; load_export_auth_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token; $(LOAD_DATA_PATH_K6_COMMAND)"'

load-data-pool-pressure: ## Run direct k6 admin SQL pool-pressure scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_resolve_admin_token; $(LOAD_DATA_POOL_PRESSURE_K6_COMMAND)'

load-data-pool-pressure-local: ## Start local AYB with run-with-ayb and run the admin SQL pool-pressure scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_resolve_admin_token; load_export_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token; $(LOAD_DATA_POOL_PRESSURE_K6_COMMAND)"'

load-http-100: ## Run direct HTTP load scenario suite at 100 VUs/iterations per scenario
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-admin-status
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-auth-request-path
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-data-path
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-data-pool-pressure

load-http-500: ## Run direct HTTP load scenario suite at 500 VUs/iterations per scenario
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-admin-status
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-auth-request-path
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-data-path
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-data-pool-pressure

load-http-1000: ## Run direct HTTP load scenario suite at 1000 VUs/iterations per scenario
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-admin-status
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-auth-request-path
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-data-path
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-data-pool-pressure

load-realtime-ws: ## Run direct k6 realtime websocket subscribe scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_REALTIME_WS_K6_COMMAND)'

load-realtime-ws-local: ## Start local AYB with run-with-ayb and run the realtime websocket subscribe scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_resolve_admin_token; load_export_env; load_export_auth_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token; $(LOAD_REALTIME_WS_K6_COMMAND)"'

load-realtime-ws-1000: ## Run direct realtime websocket scenario at 1000 VUs/iterations
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-realtime-ws

load-realtime-ws-5000: ## Run direct realtime websocket scenario at 5000 VUs/iterations
	@K6_VUS=5000 K6_ITERATIONS=5000 $(MAKE) load-realtime-ws

load-realtime-ws-10000: ## Run direct realtime websocket scenario at 10000 VUs/iterations
	@K6_VUS=10000 K6_ITERATIONS=10000 $(MAKE) load-realtime-ws

load-sustained-soak: ## Run direct k6 sustained mixed-workload soak scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_SUSTAINED_SOAK_K6_COMMAND)'

load-sustained-soak-local: ## Start local AYB with run-with-ayb and run the sustained mixed-workload soak scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_resolve_admin_token; load_export_env; load_export_auth_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token; $(LOAD_SUSTAINED_SOAK_K6_COMMAND)"'

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
	run_step "Playwright e2e"     "bash scripts/run-with-ayb.sh 'cd ui && npm run test:browser'"; \
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

check-sizes: ## Run Go file-size guardrail
	bash scripts/check-file-sizes.sh

check-ui-lint: ## Lint admin UI TypeScript source
	cd ui && npx eslint src/

check-browser-tests-lint: ## Lint browser test specs
	cd ui && npm run lint:browser-tests && npm run lint:browser-tests:mocked

check-func-sizes: ## Run Go function-size guardrail test
	go test ./internal/codehealth -run TestFunctionSizeAllowlist -count=1

check: fmt lint check-sizes check-ui-lint check-func-sizes ## Run local CI-equivalent quality checks

check-installer: ## Run installer validation suite
	sh tests/test_install.sh

check-sync-pipeline: ## Run sync-to-public rewrite validation suite
	sh tests/test_sync_pipeline.sh

check-sdk-build: ## Build the JavaScript SDK
	cd sdk && npm run build

release-candidate-check: check check-browser-tests-lint test-all ui check-sdk-build check-installer check-sync-pipeline test-smoke ## Run the trusted public release candidate gate

ui: ## Build the admin dashboard SPA
	cd ui && pnpm install && pnpm build

demos: ## Build demo apps (force rebuild, pre-built for go:embed)
	cd examples/kanban && npm ci && VITE_AYB_URL="" npx vite build
	cd examples/live-polls && npm ci && VITE_AYB_URL="" npx vite build
	@touch examples/kanban/dist/.stamp examples/live-polls/dist/.stamp

docker: ## Build Docker image locally
	docker build -t allyourbase/ayb:latest -t allyourbase/ayb:$(VERSION) .

docker-runtime-smoke: ## Run the published-image Docker runtime smoke using /tmp bind mounts
	bash scripts/docker-runtime-smoke.sh

clean: ## Remove build artifacts
	rm -f ayb
	rm -rf dist/
	rm -f ui/dist/.stamp examples/kanban/dist/.stamp examples/live-polls/dist/.stamp

release: ## Build release binaries via goreleaser (dry run)
	goreleaser release --snapshot --clean

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting
	@FILES_NEEDING_FMT="$$(find . -name '*.go' -type f ! -path './vendor/*' ! -path './_dev/*' -print0 | xargs -0 gofmt -l)"; \
	test -z "$$FILES_NEEDING_FMT" || (echo "Files need formatting:" && echo "$$FILES_NEEDING_FMT" && exit 1)

sync-openapi: ## Copy OpenAPI spec to docs-site public dir
	cp openapi/openapi.yaml docs-site/public/openapi.yaml
