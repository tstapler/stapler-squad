# Claude Squad Makefile
# Comprehensive development and analysis toolchain

# Variables
PROFILE_FLAGS ?=
PROFILE_PORT ?= 6060

.PHONY: help build test benchmark install-tools lint analyze nil-safety security format check-deps clean all proto-gen proto-lint proto-build web-build web-dev restart-web restart-web-profile

# Default target
help: ## Show this help message
	@echo "Claude Squad Development Makefile"
	@echo "================================="
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Build targets
build: server/web/dist ## Build the Go application (depends on web dist being ready)
	go build -o claude-squad .

# Build Next.js app to web-app/out (using development mode for unminified React)
web-app/out: $(shell find web-app/src -type f) web-app/package.json web-app/next.config.ts
	@echo "Building Next.js web UI (development mode for better error messages)..."
	@cd web-app && NEXT_BUILD_MODE=development npm run build
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
	@echo "Stopping existing claude-squad processes..."
	@-pkill -f "claude-squad.*--web" 2>/dev/null || true
	@sleep 1
	@echo "Starting server..."
	@./claude-squad --web $(PROFILE_FLAGS) &
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
	@echo "📝 Trace file will be saved to /tmp/claude-squad-trace-*.out on exit"
	@echo "   Analyze with: go tool trace /tmp/claude-squad-trace-*.out"

web-dev: build-all ## Build web UI and server, then restart (detects file changes automatically)
	@echo "Stopping existing claude-squad processes..."
	@-pkill -f "claude-squad.*--web" 2>/dev/null || true
	@sleep 1
	@echo "Starting server..."
	@./claude-squad --web $(PROFILE_FLAGS) &
	@sleep 2
	@echo "✅ Server restarted at http://localhost:8543"
	@if [ -n "$(PROFILE_FLAGS)" ]; then \
		echo "📊 Profiling enabled at http://localhost:$(PROFILE_PORT)/debug/pprof/"; \
	fi

install: ## Install claude-squad locally
	go install .

# Protocol Buffer code generation
proto-gen: ## Generate Go and TypeScript code from proto files
	@echo "Generating protocol buffer code..."
	buf generate proto
	@echo "✅ Code generation complete"
	@echo "  Go code:         gen/proto/go/"
	@echo "  TypeScript code: web/src/gen/"

proto-lint: ## Lint protocol buffer files
	buf lint proto

proto-build: ## Build/validate protocol buffer files
	buf build proto

proto-clean: ## Clean generated protocol buffer code
	rm -rf gen/proto/go
	rm -rf web/src/gen

# Testing targets
test: ## Run all tests
	go test ./...

test-verbose: ## Run tests with verbose output
	go test -v ./...

test-coverage: ## Run tests with coverage report
	go test -cover ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Performance benchmarks
benchmark: ## Run all benchmarks (runs in background due to long duration)
	@echo "Running comprehensive benchmarks (this will take several minutes)..."
	@echo "Use 'make benchmark-quick' for faster subset"
	go test -bench=. -benchmem -timeout=10m ./app > benchmark_results.txt 2>&1 &
	@echo "Benchmarks running in background. Results will be saved to benchmark_results.txt"

benchmark-quick: ## Run quick benchmarks for basic performance validation
	go test -bench=BenchmarkInstanceChanged -benchmem -timeout=30s ./app
	go test -bench=BenchmarkListNavigation/List_Nav_10 -benchmem -timeout=30s ./app

benchmark-navigation: ## Run navigation performance benchmarks
	go test -bench=BenchmarkListNavigation -benchmem -timeout=2m ./app

benchmark-tabs: ## Run tab switching performance benchmarks
	go test -bench=BenchmarkTabSwitching -benchmem -timeout=2m ./app

benchmark-attach: ## Run attach/detach performance benchmarks
	go test -bench=BenchmarkAttachDetach -benchmem -timeout=5m ./app

# Development tools installation
install-tools: ## Install all development and analysis tools
	@echo "Installing Go development tools..."
	go install go.uber.org/nilaway/cmd/nilaway@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/jtbonhomme/go-nilcheck@latest
	go install golang.org/x/tools/cmd/deadcode@latest
	@echo "All tools installed successfully!"

# Code quality and analysis
lint: ## Run golangci-lint with comprehensive checks
	golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet

format: ## Format code with gofmt
	go fmt ./...

vet: ## Run go vet with all analyzers
	go vet ./...
	go vet -nilness ./...

# Nil safety analysis
nil-safety: ## Run comprehensive nil safety analysis
	@echo "🔍 Running nil safety analysis..."
	@echo "================================"
	@echo "1. NilAway (Advanced nil flow analysis):"
	@-nilaway -include-pkgs="claude-squad" ./... 2>&1 | head -20
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

nilaway: ## Run NilAway nil safety analyzer
	nilaway -include-pkgs="claude-squad" ./...

staticcheck: ## Run staticcheck comprehensive analysis
	staticcheck ./...

# Security analysis
security: ## Run security analysis with gosec
	@echo "🔒 Running security analysis..."
	gosec ./...

# Dead code detection
deadcode: ## Find unreachable/dead code
	@echo "💀 Finding dead code..."
	deadcode -test ./...

# Comprehensive analysis
analyze: install-tools vet lint staticcheck nil-safety security deadcode ## Run all static analysis tools

# Dependency management
check-deps: ## Check for outdated dependencies
	go list -u -m all

tidy: ## Tidy and verify go modules
	go mod tidy
	go mod verify

# Cleanup
clean: ## Clean build artifacts and temporary files
	go clean
	rm -f claude-squad coverage.out coverage.html benchmark_results.txt
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
profile-cpu: ## Run benchmarks with CPU profiling
	go test -bench=BenchmarkInstanceChanged -benchmem -cpuprofile=cpu.prof ./app
	@echo "Run 'go tool pprof cpu.prof' to analyze CPU profile"

profile-memory: ## Run benchmarks with memory profiling  
	go test -bench=BenchmarkInstanceChanged -benchmem -memprofile=mem.prof ./app
	@echo "Run 'go tool pprof mem.prof' to analyze memory profile"

# Documentation
docs: ## Generate and open test coverage documentation
	make test-coverage
	@which open >/dev/null 2>&1 && open coverage.html || echo "Open coverage.html in your browser"

# Environment validation
validate-env: ## Validate development environment setup
	@echo "Validating development environment..."
	@go version
	@which nilaway >/dev/null 2>&1 && echo "✅ nilaway installed" || echo "❌ nilaway missing (run 'make install-tools')"
	@which staticcheck >/dev/null 2>&1 && echo "✅ staticcheck installed" || echo "❌ staticcheck missing (run 'make install-tools')"
	@which golangci-lint >/dev/null 2>&1 && echo "✅ golangci-lint installed" || echo "❌ golangci-lint missing (run 'make install-tools')"
	@which gosec >/dev/null 2>&1 && echo "✅ gosec installed" || echo "❌ gosec missing (run 'make install-tools')"
	@which deadcode >/dev/null 2>&1 && echo "✅ deadcode installed" || echo "❌ deadcode missing (run 'make install-tools')"
	@echo "Environment validation complete"