# Stapler Squad Makefile
# Comprehensive development and analysis toolchain

# Variables
PROFILE_FLAGS ?=
PROFILE_PORT ?= 6060
SERVER_FLAGS ?= --remote-access

# File dependencies
GO_FILES := $(shell find . -maxdepth 3 -name "*.go" -not -path "./vendor/*" -not -path "./node_modules/*")
WEB_FILES := $(shell find web-app/src -type f 2>/dev/null)
PROTO_FILES := $(shell find proto -name "*.proto" 2>/dev/null)
PROTO_STAMP := .proto-gen.stamp
PROTO_OUT_DIRS := gen/proto/go web/src/gen

# Tool detection and automatic installation
MISSING_TOOLS :=
ifeq ($(shell which go 2>/dev/null),)
	MISSING_TOOLS += go
endif
ifeq ($(shell which buf 2>/dev/null),)
	MISSING_TOOLS += buf
endif
ifeq ($(shell which npm 2>/dev/null),)
	MISSING_TOOLS += nodejs
endif

.PHONY: ensure-tools
ensure-tools: ## Automatically install missing system tools (go, buf, node) via asdf or Homebrew
ifneq ($(wildcard .tool-versions),)
	@if which asdf >/dev/null 2>&1; then \
		echo "🔍 asdf detected, ensuring versions from .tool-versions are installed..."; \
		asdf install; \
	fi
endif
ifneq ($(MISSING_TOOLS),)
	@if which brew >/dev/null 2>&1; then \
		echo "🔍 Missing tools detected: $(MISSING_TOOLS)"; \
		echo "🚀 Installing via Homebrew..."; \
		brew install $(MISSING_TOOLS); \
	else \
		echo "❌ Error: Missing tools: $(MISSING_TOOLS). Please install them or Homebrew/asdf."; \
		exit 1; \
	fi
endif

.PHONY: help build test benchmark install-tools lint analyze nil-safety security format check-deps clean all proto-gen proto-lint proto-build web-build web-dev restart-web restart-web-profile demo-video demo-post-process demo-gif

# Default target
help: ## Show this help message
	@echo "Stapler Squad Development Makefile"
	@echo "================================="
	@grep -E '^[a-zA-Z0-9._-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Build targets
build: stapler-squad ## Build the Go application

stapler-squad: ensure-tools proto-gen server/web/dist $(GO_FILES) ## Build the Go binary
	@echo "Building Go application..."
	go build -o stapler-squad .
	@echo "✅ stapler-squad built successfully"

# Build Next.js app to web-app/out
web-app/out: ensure-tools web-app/package.json $(WEB_FILES) web-app/next.config.ts
	@echo "Building Next.js web UI (development mode for better error messages)..."
	@cd web-app && ([ -d node_modules ] || npm install) && NEXT_BUILD_MODE=development npm run build
	@touch web-app/out # Update timestamp to mark completion

# Copy web-app/out to server/web/dist (used by Go embed)
server/web/dist: web-app/out
	@echo "Copying built files to server/web/dist..."
	@rm -rf server/web/dist
	@cp -r web-app/out server/web/dist
	@touch server/web/dist # Update timestamp
	@echo "✅ Web UI built and copied successfully"

web-build: server/web/dist ## Build the Next.js web UI (convenience target)

build-all: build ## Build both web UI and Go application
	@echo "✅ Full build complete (web + server)"

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

# Protocol Buffer code generation
proto-gen: ensure-tools ## Generate Go and TypeScript code from proto files
	@echo "Checking if proto files need regeneration..."
	@if [ ! -f $(PROTO_STAMP) ] || [ "$$(find proto -name '*.proto' -newer $(PROTO_STAMP) -print -quit)" ]; then \
		echo "Generating protocol buffer code..."; \
		buf generate proto; \
		echo "✅ Code generation complete"; \
		echo "  Go code:         gen/proto/go/"; \
		echo "  TypeScript code: web/src/gen/"; \
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
test: ensure-tools ## Run all tests
	go test ./...

test-verbose: ensure-tools ## Run tests with verbose output
	go test -v ./...

test-coverage: ensure-tools ## Run tests with coverage report
	go test -cover ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Performance benchmarks
benchmark: ensure-tools ## Run all benchmarks
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
	go install github.com/jtbonhomme/go-nilcheck@latest
	go install golang.org/x/tools/cmd/deadcode@latest
	@echo "All tools installed successfully!"

# Code quality and analysis
lint: ensure-tools ## Run golangci-lint with comprehensive checks
	golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet

format: ensure-tools ## Format code with gofmt
	go fmt ./...

vet: ensure-tools ## Run go vet with all analyzers
	go vet ./...
	go vet -nilness ./...

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
	rm -f stapler-squad coverage.out coverage.html benchmark_results.txt
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

ci: build test vet lint ## Continuous integration workflow

# Quick development workflows
quick-check: build test-coverage lint ## Quick development validation
	@echo "✅ Quick validation complete"

pre-commit: format vet test lint ## Pre-commit validation
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