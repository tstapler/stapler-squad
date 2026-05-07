# Stapler Squad Makefile
# Comprehensive development and analysis toolchain

# Variables
PROFILE_FLAGS ?=
PROFILE_PORT ?= 6060
SERVER_FLAGS ?= --remote-access --tmux-keep-server
ifeq ($(shell uname -s),Darwin)
  export CGO_CFLAGS := -Wno-ignored-qualifiers
else
  export CGO_CFLAGS := -Wno-discarded-qualifiers -Wno-ignored-qualifiers
endif
export CGO_ENABLED := 1

# File dependencies
GO_FILES := $(shell find . -maxdepth 3 -name "*.go" -not -path "./vendor/*" -not -path "./node_modules/*")
WEB_FILES := $(shell find web-app/src -type f 2>/dev/null)
PROTO_FILES := $(shell find proto -name "*.proto" 2>/dev/null)
PROTO_STAMP := .proto-gen.stamp
PROTO_OUT_DIRS := gen/proto/go web-app/src/gen
ASDF_STAMP := .asdf-install.stamp

.PHONY: ensure-tools
# ensure-tools runs asdf install only when .tool-versions changes
ensure-tools: $(ASDF_STAMP) ## Automatically install missing system tools (go, buf, node) via asdf or Homebrew

$(ASDF_STAMP): .tool-versions
ifneq ($(wildcard .tool-versions),)
	@if which asdf >/dev/null 2>&1; then \
		echo "🔍 asdf detected, ensuring versions from .tool-versions are installed..."; \
		asdf plugin add nodejs || true; \
		asdf install; \
	fi
endif
	@if which go >/dev/null 2>&1 && which buf >/dev/null 2>&1 && which npm >/dev/null 2>&1; then \
		touch $(ASDF_STAMP); \
	else \
		if which brew >/dev/null 2>&1; then \
			echo "🔍 Missing tools, installing via Homebrew..."; \
			brew install go buf nodejs; \
		else \
			echo "❌ Error: go/buf/npm not found. Install asdf or Homebrew."; \
			exit 1; \
		fi; \
		touch $(ASDF_STAMP); \
	fi

.PHONY: help build test benchmark install-tools lint lint-custom analyze nil-safety security format fmt-check check-deps clean all proto-gen proto-lint proto-build web-build web-dev restart-web restart-web-profile qr demo-video demo-post-process demo-gif benchmark-baseline benchmark-compare benchmark-tier1 profile-goroutines profile-block profile-mutex profile-trace build-mux install-mux install-service uninstall-service coverage-func coverage-gaps coverage-pkg coverage-refactor registry-generate-backend registry-generate-frontend registry-generate registry-diff e2e-report e2e-lighthouse build-tmux build-tmux-embed build-embedded clean-tmux init-submodules test-with-pinned-tmux vet-architecture vet-rpc-markers coverage-integration

# Default target
help: ## Show this help message
	@echo "Stapler Squad Development Makefile"
	@echo "================================="
	@grep -E '^[a-zA-Z0-9._-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Registry targets
# Per-feature files live under docs/registry/features/ — one file per RPC/component.
# They are the committed source of truth; the monolithic aggregate files are generated.
BACKEND_FEATURES_DIR := docs/registry/features/backend
FRONTEND_FEATURES_DIR := docs/registry/features/frontend
REGISTRY_OUTPUT_DIR ?= docs/registry
BACKEND_SCANNER_BIN := tools/scanner/backend/cmd/scanner
FRONTEND_SCANNER := tools/scanner/frontend/src/main.ts

registry-generate-backend: ## Scan proto+markers → write per-feature files under docs/registry/features/backend/
	@echo "Building backend scanner..."
	@cd tools/scanner && go build -o backend/cmd/scanner ./backend/cmd/
	@echo "Scanning backend features..."
	@./$(BACKEND_SCANNER_BIN) proto/session/v1/session.proto server/services/ $(BACKEND_FEATURES_DIR)
	@./$(BACKEND_SCANNER_BIN) proto/session/v1/unfinished.proto server/services/ $(BACKEND_FEATURES_DIR)
	@echo "✅ Backend per-feature files written to $(BACKEND_FEATURES_DIR)/"

registry-generate-frontend: ## Generate frontend feature registry from React component markers
	@echo "Installing frontend scanner dependencies..."
	@cd tools/scanner/frontend && npm install --silent
	@echo "Scanning frontend features..."
	@node tools/scanner/frontend/node_modules/.bin/ts-node \
		tools/scanner/frontend/src/main.ts \
		web-app/src \
		$(REGISTRY_OUTPUT_DIR)/frontend-features.json \
		$(REGISTRY_OUTPUT_DIR)/backend-features.json \
		$(REGISTRY_OUTPUT_DIR)/coverage-gaps.json
	@echo "✅ Frontend registry written to $(REGISTRY_OUTPUT_DIR)/frontend-features.json"

registry-aggregate: ## Assemble per-feature files → monolithic JSON (for tooling that needs the flat format)
	@python3 tools/scanner/aggregate.py $(BACKEND_FEATURES_DIR) $(REGISTRY_OUTPUT_DIR)/backend-features.json
	@python3 tools/scanner/aggregate.py $(FRONTEND_FEATURES_DIR) $(REGISTRY_OUTPUT_DIR)/frontend-features.json
	@echo "✅ Monolithic registries aggregated from per-feature files"

registry-generate: registry-generate-backend registry-generate-frontend registry-aggregate ## Generate per-feature files and aggregate monolithic registries

registry-diff: ## Show what would change in registry without writing files (dry run)
	@echo "Comparing current code against committed registries..."
	@./tools/scanner/validate-registry.sh

e2e-report: ## Generate Allure HTML report from last test run
	@cd tests/e2e && npx allure generate allure-results --clean -o allure-report
	@echo "✅ Report generated at tests/e2e/allure-report/index.html"

e2e-lighthouse: ## Run Lighthouse CI performance audit
	@cd tests/e2e && npx lhci autorun --config=lighthouse.config.js

# Build targets
build: stapler-squad ## Build the Go application

stapler-squad: ensure-tools proto-gen server/web/dist lint $(GO_FILES) ## Build the Go binary
	@echo "Building Go application..."
	go build -o stapler-squad .
	@echo "✅ stapler-squad built successfully"

# Install web-app npm dependencies when package-lock.json changes
web-app/node_modules/.package-lock.json: web-app/package.json web-app/package-lock.json
	@echo "Installing web-app npm dependencies..."
	@cd web-app && npm install
	@touch web-app/node_modules/.package-lock.json

# Build Next.js app to web-app/out
web-app/out: ensure-tools web-app/node_modules/.package-lock.json $(WEB_FILES) web-app/next.config.ts
	@echo "Building Next.js web UI (development mode for better error messages)..."
	@cd web-app && NEXT_BUILD_MODE=development npm run build
	@touch web-app/out # Update timestamp to mark completion

# Copy web-app/out to server/web/dist (used by Go embed)
server/web/dist: web-app/out
	@echo "Copying built files to server/web/dist..."
	@rm -rf server/web/dist
	@mkdir -p server/web/dist
	@cp -r web-app/out/* server/web/dist/
	@touch server/web/dist # Update timestamp
	@echo "✅ Web UI built and copied successfully"

web-build: server/web/dist ## Build the Next.js web UI (convenience target)

build-all: build ## Build both web UI and Go application
	@echo "✅ Full build complete (web + server)"

qr: ensure-tools proto-gen ## Print remote access QR codes for phone setup
	@[ -f ./stapler-squad ] || $(MAKE) build
	@./stapler-squad print-qr-codes

restart-web: build-all ## Rebuild and restart the web server
	@echo "Stopping existing stapler-squad processes..."
	@-pkill -f "^\./stapler-squad" 2>/dev/null || true
	@sleep 1
	@echo "Starting server..."
	@./stapler-squad $(SERVER_FLAGS) $(PROFILE_FLAGS) &
	@sleep 2
	@echo "✅ Server restarted at http://localhost:8543"
	@if [ -n "$(PROFILE_FLAGS)" ]; then \
		echo "📊 Profiling enabled at http://localhost:$(PROFILE_PORT)/debug/pprof/"; \
	fi

restart-web-profile: ## Rebuild and restart web server with profiling enabled
	@$(MAKE) restart-web PROFILE_FLAGS="--profile --trace" PROFILE_PORT=$(PROFILE_PORT)
	@echo ""
	@echo "📊 Profiling Endpoints:"
	@echo "  Goroutines: http://localhost:$(PROFILE_PORT)/debug/pprof/goroutine?debug=1"
	@echo "  Block:      http://localhost:$(PROFILE_PORT)/debug/pprof/block?debug=1"
	@echo "  Mutex:      http://localhost:$(PROFILE_PORT)/debug/pprof/mutex?debug=1"
	@echo ""
	@echo "📝 Trace file will be saved to /tmp/stapler-squad-trace-*.out on exit"
	@echo "   Analyze with: go tool trace /tmp/stapler-squad-trace-*.out"

web-dev: build-all ## Build web UI and server, then restart (detects file changes automatically)
	@echo "Stopping existing stapler-squad processes..."
	@-pkill -f "^\./stapler-squad" 2>/dev/null || true
	@sleep 1
	@echo "Starting server..."
	@./stapler-squad $(PROFILE_FLAGS) &
	@sleep 2
	@echo "✅ Server restarted at http://localhost:8543"
	@if [ -n "$(PROFILE_FLAGS)" ]; then \
		echo "📊 Profiling enabled at http://localhost:$(PROFILE_PORT)/debug/pprof/"; \
	fi

install: ensure-tools ## Install stapler-squad locally
	go install .
	mkdir -p ~/.local/bin
	go build -o ~/.local/bin/ssq-hooks ./cmd/ssq-hooks/

build-mux: ensure-tools ## Build the claude-mux PTY multiplexer binary
	@echo "Building claude-mux..."
	go build -o claude-mux ./cmd/claude-mux
	@echo "✅ claude-mux built to ./claude-mux"

install-mux: ensure-tools ## Build and install claude-mux to ~/.local/bin
	@./scripts/install-mux.sh

# ── Pinned tmux binary (for test isolation) ────────────────────────────────
# Builds tmux 3.4 from the third_party/tmux git submodule.
# Tests use TMUX_BIN=bin/tmux to run against the pinned binary instead of the
# system tmux, ensuring reproducible results across developer machines and CI.
#
# Bazel caches the C build artifacts — subsequent runs are instant.
# Without Bazel, falls back to make (full recompile each clean build).

BIN_TMUX        := bin/tmux
TMUX_BUILD_STAMP := .tmux-build.stamp

# Stamp-file approach: only rebuild when submodule source changes
$(BIN_TMUX): $(TMUX_BUILD_STAMP)
	@true

$(TMUX_BUILD_STAMP): third_party/tmux/configure.ac
	@$(MAKE) build-tmux
	@touch $(TMUX_BUILD_STAMP)

init-submodules: ## Initialize git submodules (required once after clone)
	git submodule update --init --recursive

build-tmux: ## Build pinned tmux 3.4 binary from third_party/tmux submodule
	@echo "Building pinned tmux binary..."
	@if command -v bazel >/dev/null 2>&1 && [ -f third_party/tmux/configure.ac ]; then \
		echo "Using Bazel (artifacts cached)..."; \
		bazel build //third_party/tmux:tmux && \
		mkdir -p bin && \
		cp "$$(bazel info bazel-bin)/third_party/tmux/tmux" $(BIN_TMUX) && \
		chmod +x $(BIN_TMUX) && \
		echo "✅ tmux built via Bazel at $(BIN_TMUX)"; \
	else \
		./scripts/build-tmux.sh; \
	fi

build-tmux-embed: build-tmux ## Copy built tmux into the embed dir for go build -tags embed_tmux
	@mkdir -p session/tmux/embed
	@cp $(BIN_TMUX) session/tmux/embed/tmux
	@echo "✅ session/tmux/embed/tmux ready ($(shell $(BIN_TMUX) -V 2>/dev/null || echo unknown))"

build-embedded: build-tmux-embed ## Build stapler-squad with tmux bundled inside the binary
	go build -tags embed_tmux -o stapler-squad .
	@echo "✅ stapler-squad built with embedded tmux"

clean-tmux: ## Remove the built tmux binary and submodule build artifacts
	@./scripts/build-tmux.sh --clean
	@rm -f $(TMUX_BUILD_STAMP)
	@rm -f session/tmux/embed/tmux
	@echo "✅ tmux artifacts cleaned"

install-service: build ## Install stapler-squad as a system service (systemd on Linux, LaunchAgent on macOS)
	@STAPLER_SQUAD_BIN="$(CURDIR)/stapler-squad" ./scripts/install-service.sh $(if $(NO_PROFILE),--no-profile) $(if $(PROFILE_PORT),--profile-port $(PROFILE_PORT))

uninstall-service: ## Remove the system service and disable auto-start on login
	@./scripts/install-service.sh --uninstall

# Protocol Buffer code generation
proto-gen: ensure-tools web-app/node_modules/.package-lock.json ## Generate Go and TypeScript code from proto files
	@echo "Checking if proto files need regeneration..."
	@if [ ! -f $(PROTO_STAMP) ] \
	   || [ "$$(find proto -name '*.proto' -newer $(PROTO_STAMP) -print -quit)" ] \
	   || [ web-app/node_modules/.bin/protoc-gen-es -nt $(PROTO_STAMP) ]; then \
		echo "Generating protocol buffer code..."; \
		buf generate proto; \
		echo "✅ Code generation complete"; \
		echo "  Go code:         gen/proto/go/"; \
		echo "  TypeScript code: web-app/src/gen/"; \
		touch $(PROTO_STAMP); \
	else \
		echo "✅ Proto files unchanged, skipping generation"; \
	fi

proto-lint: ensure-tools ## Lint protocol buffer files
	buf lint proto

proto-build: ensure-tools ## Build/validate protocol buffer files
	buf build proto

proto-clean: ## Clean generated protocol buffer code
	rm -rf gen/proto/go
	rm -rf web/src/gen

# Testing targets
test: ensure-tools proto-gen ## Run all tests (skips slow integration tests; use test-integration for full suite)
	go test -short ./...

test-verbose: ensure-tools proto-gen ## Run tests with verbose output
	go test -short -v ./...

test-coverage: ensure-tools proto-gen ## Run tests with coverage report (HTML)
	go test -short -cover ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@which open >/dev/null 2>&1 && open coverage.html || true

coverage-func: ensure-tools proto-gen ## Show function-level coverage sorted by % (all non-100% functions)
	@go test -short -coverprofile=coverage.out -covermode=atomic ./... 2>/dev/null
	@echo ""
	@echo "=== Function Coverage (sorted, lowest first) ==="
	@go tool cover -func=coverage.out | grep -v "^total" | sort -t'%' -k1 -n | head -60
	@echo ""
	@go tool cover -func=coverage.out | grep "^total"

coverage-gaps: ensure-tools proto-gen ## Show only functions with 0% coverage (completely untested)
	@go test -short -coverprofile=coverage.out -covermode=atomic ./... 2>/dev/null
	@echo ""
	@echo "=== Untested Functions (0.0% coverage) ==="
	@go tool cover -func=coverage.out | grep " 0.0%" | grep -v "_test.go"
	@echo ""
	@echo "=== Total ==="
	@go tool cover -func=coverage.out | grep "^total"

coverage-pkg: ensure-tools proto-gen ## Show per-package coverage summary sorted by % (lowest first)
	@go test -short -coverprofile=coverage.out -covermode=atomic ./... 2>&1 | \
		grep -E "^(ok|FAIL|\?)" | \
		awk '{for(i=1;i<=NF;i++) if($$i ~ /coverage:/) {pct=$$i+0; print pct"% "$$1" "$$2}}' | \
		sort -n
	@echo ""
	@go test -short -coverprofile=coverage.out -covermode=atomic ./... 2>/dev/null; \
		go tool cover -func=coverage.out | grep "^total"

coverage-refactor: ensure-tools proto-gen ## Show coverage for the 4 files targeted by the backend refactor
	@go test -short -coverprofile=coverage.out -covermode=atomic ./... 2>/dev/null
	@echo ""
	@echo "=== session/instance.go ==="
	@go tool cover -func=coverage.out | grep "session/instance.go" | sort -t'%' -k1 -n
	@echo ""
	@echo "=== server/services/session_service.go ==="
	@go tool cover -func=coverage.out | grep "session_service.go" | sort -t'%' -k1 -n
	@echo ""
	@echo "=== session/storage.go ==="
	@go tool cover -func=coverage.out | grep "session/storage.go" | sort -t'%' -k1 -n
	@echo ""
	@echo "=== session/review_queue_poller.go ==="
	@go tool cover -func=coverage.out | grep "review_queue_poller.go" | sort -t'%' -k1 -n
	@echo ""
	@echo "=== server/adapters/review_queue_adapter.go ==="
	@go tool cover -func=coverage.out | grep "review_queue_adapter.go" | sort -t'%' -k1 -n
	@echo ""
	@go tool cover -func=coverage.out | grep "^total"

test-race: ensure-tools proto-gen ## Run tests with race detector enabled (skips slow integration tests)
	go test -race -short ./...

test-integration: ensure-tools proto-gen ## Run integration tests (requires real tmux)
	go test -race -tags integration ./...

coverage-integration: ensure-tools proto-gen ## Build instrumented binary, run integration tests, emit integration.out
	@mkdir -p /tmp/covdata
	go build -cover -o stapler-squad-cov .
	@echo "Starting instrumented binary..."
	GOCOVERDIR=/tmp/covdata STAPLER_SQUAD_INSTANCE=cov-$$PPID ./stapler-squad-cov &
	@sleep 2
	@echo "Running integration tests against instrumented binary..."
	go test -race -tags integration ./... || true
	@echo "Stopping instrumented binary..."
	@pkill -f stapler-squad-cov || true
	@sleep 1
	go tool covdata textfmt -i=/tmp/covdata -o integration.out
	@echo "✅ Integration coverage written to integration.out"
	@rm -f stapler-squad-cov
	@rm -rf /tmp/covdata

test-ux-polish: ## Run tests registered in docs/registry/features/ (no server/tmux required)
	@RUN=$$(python3 -c "import json,glob; ids=[t for p in glob.glob('docs/registry/features/backend/**/*.json',recursive=True) for t in json.load(open(p)).get('testIds',[])]; print('|'.join(sorted(set(ids))))"); \
	echo "Running: $$RUN"; \
	go test ./server/services/ -run "$$RUN" -v -timeout 120s
	go test ./session/prompts/... -v -timeout 30s

test-with-pinned-tmux: ensure-tools proto-gen $(BIN_TMUX) ## Run tests using the pinned tmux binary (reproducible)
	TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -race ./...

# Performance benchmarks
benchmark: ensure-tools proto-gen ## Run all benchmarks
	@echo "Running comprehensive benchmarks..."
	go test -bench=. -benchmem -timeout=10m ./... > benchmark_results.txt 2>&1 &
	@echo "Benchmarks running in background. Results will be saved to benchmark_results.txt"

# Development tools installation
install-tools: ensure-tools ## Install all development and analysis tools
	@echo "Installing Go development tools..."
	go install go.uber.org/nilaway/cmd/nilaway@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/jtbonhomme/go-nilcheck/cmd/nilcheck@latest
	go install golang.org/x/tools/cmd/deadcode@latest
	go install golang.org/x/perf/cmd/benchstat@latest
	@echo "All tools installed successfully!"

# Code quality and analysis
.PHONY: vet-architecture
vet-architecture: ## Run all architectural lint checks (depguard + import cycle check)
	golangci-lint run --enable depguard ./...
	go build ./...

.PHONY: vet-rpc-markers
vet-rpc-markers: registry-generate-backend ## Check that all RPC handlers have a // +api: marker (advisory)
	@missing=0; \
	for f in $$(find $(BACKEND_FEATURES_DIR) -name "*.json"); do \
		if jq -e '.markerFound == false' "$$f" > /dev/null 2>&1; then \
			id=$$(jq -r '.id' "$$f"); method=$$(jq -r '.method' "$$f"); \
			echo "MISSING MARKER  $$id  ($$method)"; \
			echo "  Add:  // +api: $$id  to the handler in server/services/"; \
			missing=$$((missing + 1)); \
		fi; \
	done; \
	if [ "$$missing" -gt 0 ]; then \
		echo ""; \
		echo "$$missing RPC handler(s) missing // +api: marker."; \
		exit 1; \
	else \
		echo "✅ All RPC handlers have +api: markers."; \
	fi

lint: ensure-tools proto-gen server/web/dist lint-custom ## Run golangci-lint with comprehensive checks
	@GOBIN=$$(go env GOBIN); \
	if [ -z "$$GOBIN" ]; then GOBIN=$$(go env GOPATH)/bin; fi; \
	if ! which golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint v2..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
	fi; \
	golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet

HOTPOLLLOG_BIN       := $(CURDIR)/bin/hotpolllog-lint
NOCOMMANDPATTERN_BIN := $(CURDIR)/bin/nocommandpattern-lint

lint-custom: $(HOTPOLLLOG_BIN) $(NOCOMMANDPATTERN_BIN) ## Run project-specific custom linters (hotpolllog, nocommandpattern)
	@echo "Running custom lint: hotpolllog..."
	@$(HOTPOLLLOG_BIN) ./...
	@echo "hotpolllog: ok"
	@echo "Running custom lint: nocommandpattern..."
	@$(NOCOMMANDPATTERN_BIN) ./pkg/classifier/...
	@echo "nocommandpattern: ok"

$(HOTPOLLLOG_BIN):
	@mkdir -p $(CURDIR)/bin
	@cd tools/lint/hotpolllog && go build -o $(HOTPOLLLOG_BIN) ./cmd/hotpolllog

$(NOCOMMANDPATTERN_BIN):
	@mkdir -p $(CURDIR)/bin
	@cd tools/lint/nocommandpattern && go build -o $(NOCOMMANDPATTERN_BIN) ./cmd/nocommandpattern

lint-no-sleep-tests: ## ADR-003 audit: count time.Sleep calls in test files outside testutil/ (target: 0)
	@violations=$$(grep -rn 'time\.Sleep(' --include='*_test.go' . \
	  | grep -v 'vendor\|web-app\|third_party\|bin/\|testutil/' \
	  | grep -v ':[[:space:]]*//' \
	  | wc -l | tr -d ' '); \
	echo "⏱  time.Sleep in test files (excluding testutil/): $$violations (target: 0, per ADR-003)"; \
	if [ "$$violations" -gt 0 ]; then \
	  grep -rn 'time\.Sleep(' --include='*_test.go' . | grep -v 'vendor\|web-app\|third_party\|bin/\|testutil/' | grep -v ':[[:space:]]*//' ; \
	  exit 1; \
	fi

format: ensure-tools ## Format code with gofmt
	go fmt ./...

fmt-check: ## Verify all Go files are gofmt-formatted (non-destructive; exits 1 if any are not)
	@UNFORMATTED=$$(gofmt -l . | grep -v vendor); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files are not gofmt formatted:"; \
		echo "$$UNFORMATTED"; \
		echo "Fix with: gofmt -w ."; \
		exit 1; \
	fi
	@echo "✅ All Go files are properly formatted"

vet: ensure-tools proto-gen ## Run go vet with all analyzers
	go vet ./...

# Nil safety analysis
nil-safety: ensure-tools ## Run comprehensive nil safety analysis
	@echo "🔍 Running nil safety analysis..."
	@echo "================================"
	@echo "1. NilAway (Advanced nil flow analysis):"
	@-nilaway -include-pkgs="github.com/tstapler/stapler-squad" ./... 2>&1 | head -20
	@echo ""
	@echo "2. Built-in nilness analyzer:"
	@-go vet -nilness ./... 2>&1 | head -10
	@echo ""
	@echo "3. go-nilcheck (Function pointer validation):"
	@-go-nilcheck ./... 2>&1 | head -10
	@echo ""
	@echo "For detailed analysis, run individual tools:"
	@echo "  make nilaway"
	@echo "  make staticcheck"

nilaway: ensure-tools ## Run NilAway nil safety analyzer
	nilaway -include-pkgs="github.com/tstapler/stapler-squad" ./...

staticcheck: ensure-tools ## Run staticcheck comprehensive analysis
	staticcheck ./...

# Security analysis
security: ensure-tools ## Run security analysis with gosec
	@echo "🔒 Running security analysis..."
	gosec ./...

# Dead code detection
deadcode: ensure-tools ## Find unreachable/dead code
	@echo "💀 Finding dead code..."
	deadcode -test ./...

# Comprehensive analysis
analyze: install-tools vet lint staticcheck nil-safety security deadcode ## Run all static analysis tools

# Dependency management
check-deps: ensure-tools ## Check for outdated dependencies
	go list -u -m all

tidy: ensure-tools ## Tidy and verify go modules
	go mod tidy
	go mod verify

# Cleanup
clean: ## Clean build artifacts and temporary files
	go clean
	rm -f stapler-squad claude-mux coverage.out coverage.html benchmark_results.txt
	rm -rf analysis_results/

clean-tools: ## Remove all installed development tools (use with caution)
	@echo "This will remove development tools from GOPATH/bin"
	@echo "Cancel with Ctrl+C if you want to keep them"
	@sleep 3
	rm -f $(GOPATH)/bin/nilaway $(GOPATH)/bin/staticcheck $(GOPATH)/bin/golangci-lint $(GOPATH)/bin/gosec $(GOPATH)/bin/go-nilcheck $(GOPATH)/bin/deadcode

# Comprehensive workflows
all: clean build test lint analyze ## Clean, build, test, and analyze everything

dev-setup: install-tools ## Set up development environment
	@echo "Development environment setup complete!"
	@echo "Run 'make help' to see available commands"

ci: build test test-race vet lint test-integration fmt-check registry-generate ## Full CI pipeline: proto→web→build→tests→lint→fmt→registry

# Quick development workflows
quick-check: build test-coverage test-race lint ## Quick development validation
	@echo "✅ Quick validation complete"

pre-commit: format vet test test-race lint vet-architecture ## Pre-commit validation
	@echo "✅ Pre-commit checks passed"

# Debugging and profiling
profile-cpu: ensure-tools ## Run benchmarks with CPU profiling
	go test -bench=. -benchmem -cpuprofile=cpu.prof ./...
	@echo "Run 'go tool pprof cpu.prof' to analyze CPU profile"

profile-memory: ensure-tools ## Run benchmarks with memory profiling
	go test -bench=. -benchmem -memprofile=mem.prof ./...
	@echo "Run 'go tool pprof mem.prof' to analyze memory profile"

# Documentation
docs: ## Generate and open test coverage documentation
	make test-coverage
	@which open >/dev/null 2>&1 && open coverage.html || echo "Open coverage.html in your browser"

# File-target: re-post-process whenever the raw WebM is newer than the GIF.
# Both outputs (webm with chrome frame, gif) are produced by the script.
# `demo-post-process` is a .PHONY convenience alias; `assets/demo.gif` is the
# real file-level dependency target used by demo-video.
assets/demo.gif: assets/demo.webm scripts/demo-post-process.sh
	@./scripts/demo-post-process.sh assets/demo.webm

demo-post-process: assets/demo.gif ## Add browser chrome frame to assets/demo.webm and export assets/demo.gif
demo-gif: assets/demo.gif ## Alias for demo-post-process

# assets/demo.webm is produced by the Go test harness (Playwright recording).
# Declaring it as a file target lets make skip the recording when the webm is
# already newer than the stapler-squad binary and no source files changed.
assets/demo.webm: stapler-squad tests/e2e/demo.spec.ts tests/demo/helpers.go
	@cd tests/e2e && npm install --silent
	RECORD_DEMO=1 go test ./tests/demo/... -run TestRecordDemo -v -timeout 180s

demo-video: assets/demo.gif ## Record demo video, add browser chrome, and export GIF (assets/demo.webm + assets/demo.gif)

# Environment validation
validate-env: ensure-tools ## Validate development environment setup
	@echo "Validating development environment..."
	@go version
	@npm --version
	@buf --version
	@which nilaway >/dev/null 2>&1 && echo "✅ nilaway installed" || echo "❌ nilaway missing (run 'make install-tools')"
	@which staticcheck >/dev/null 2>&1 && echo "✅ staticcheck installed" || echo "❌ staticcheck missing (run 'make install-tools')"
	@which golangci-lint >/dev/null 2>&1 && echo "✅ golangci-lint installed" || echo "❌ golangci-lint missing (run 'make install-tools')"
	@which gosec >/dev/null 2>&1 && echo "✅ gosec installed" || echo "❌ gosec missing (run 'make install-tools')"
	@which deadcode >/dev/null 2>&1 && echo "✅ deadcode installed" || echo "❌ deadcode missing (run 'make install-tools')"
	@echo "Environment validation complete"

# Benchmark comparison (local A/B testing with benchstat)
benchmark-baseline: ensure-tools proto-gen ## Save current benchmark results as baseline for comparison
	@echo "Running benchmarks and saving as baseline..."
	go test -bench=. -benchmem -count=8 -timeout=30m ./... > bench-old.txt 2>&1
	@echo "✅ Baseline saved to bench-old.txt"

benchmark-compare: ensure-tools proto-gen ## Run benchmarks and compare against saved baseline
	@if [ ! -f bench-old.txt ]; then \
		echo "❌ No baseline found. Run 'make benchmark-baseline' first."; \
		exit 1; \
	fi
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -count=8 -timeout=30m ./... > bench-new.txt 2>&1
	@echo "Comparing results (old vs new):"
	benchstat bench-old.txt bench-new.txt
	@echo ""
	@echo "Tip: Run 'make benchmark-baseline' to update the baseline to the current results."

benchmark-tier1: ensure-tools proto-gen ## Run Tier 1 critical-path benchmarks (fast, ~5 min)
	@echo "Running Tier 1 benchmarks..."
	go test \
		-bench='BenchmarkEventBus|BenchmarkDeltaGeneration|BenchmarkCircularBuffer|BenchmarkSessionService_List|BenchmarkSessionService_Get|BenchmarkSessionService_Stream' \
		-benchmem \
		-count=8 \
		-timeout=10m \
		./...

# Profiling helpers — capture profiles from a running server (make restart-web-profile first)
PROFILE_SERVER ?= http://localhost:6060

profile-goroutines: ## Capture goroutine dump from running server (requires --profile flag)
	@echo "Capturing goroutine dump..."
	curl -s "$(PROFILE_SERVER)/debug/pprof/goroutine?debug=2" > goroutines.txt
	@echo "✅ Saved to goroutines.txt"
	@echo "   Review: cat goroutines.txt | grep -A3 'goroutine [0-9]'"

profile-block: ## Capture block profile from running server (requires --profile flag)
	@echo "Capturing block profile..."
	curl -s "$(PROFILE_SERVER)/debug/pprof/block?debug=1" > block.prof
	@echo "✅ Saved to block.prof"
	@echo "   Analyze: go tool pprof -http=:8081 block.prof"

profile-mutex: ## Capture mutex profile from running server (requires --profile flag)
	@echo "Capturing mutex profile..."
	curl -s "$(PROFILE_SERVER)/debug/pprof/mutex?debug=1" > mutex.prof
	@echo "✅ Saved to mutex.prof"
	@echo "   Analyze: go tool pprof -http=:8081 mutex.prof"

profile-trace: ## Capture 30-second execution trace from running server (requires --profile flag)
	@echo "Capturing 30-second execution trace..."
	curl -s "$(PROFILE_SERVER)/debug/pprof/trace?seconds=30" > trace.out
	@echo "✅ Saved to trace.out"
	@echo "   Analyze: go tool trace trace.out"
