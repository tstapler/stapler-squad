# Stapler Squad Makefile
# Comprehensive development and analysis toolchain

# Variables
UNAME_S := $(shell uname -s)
PROFILE_FLAGS ?=
PROFILE_PORT ?= 6060
SERVER_FLAGS ?= --remote-access --tmux-keep-server
ifeq ($(shell uname -s),Darwin)
  export CGO_CFLAGS := -Wno-ignored-qualifiers
else
  export CGO_CFLAGS := -Wno-discarded-qualifiers -Wno-ignored-qualifiers
endif
export CGO_ENABLED := 1

# Version reported by `stapler-squad version`, injected via ldflags to match
# GoReleaser's `-X main.version={{.Version}}`. Falls back to a dev marker
# when building outside a git checkout (e.g. from a source tarball). Stripped
# to a safe charset: git tag names may legally contain shell metacharacters
# (e.g. `` ` `` or `$()`), and this value is later embedded in a
# double-quoted shell argument, where those characters are NOT neutralized.
VERSION := $(shell (git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev) | tr -cd 'A-Za-z0-9.+_-')
LDFLAGS := -X main.version=$(VERSION)

# File dependencies
GO_FILES := $(shell find . -maxdepth 3 -name "*.go" -not -path "./vendor/*" -not -path "./node_modules/*")
WEB_FILES := $(shell find web-app/src -type f 2>/dev/null)
PROTO_FILES := $(shell find proto -name "*.proto" 2>/dev/null)
PROTO_STAMP := .proto-gen.stamp
PROTO_OUT_DIRS := gen/proto/go web-app/src/gen
ASDF_STAMP := .asdf-install.stamp
ENT_STAMP := .ent-gen.stamp

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
	@if which go >/dev/null 2>&1 && which buf >/dev/null 2>&1 && which pnpm >/dev/null 2>&1; then \
		touch $(ASDF_STAMP); \
	else \
		if which brew >/dev/null 2>&1; then \
			echo "🔍 Missing tools, installing via Homebrew..."; \
			brew install go buf nodejs; \
			brew install pnpm; \
		else \
			echo "❌ Error: go/buf/pnpm not found. Install asdf or Homebrew."; \
			exit 1; \
		fi; \
		touch $(ASDF_STAMP); \
	fi

.PHONY: help ports build test benchmark install-tools lint lint-custom actor-lint analyze nil-safety security format fmt-check check-deps clean all proto-gen proto-lint proto-build ent-gen web-build web-dev restart-web restart-web-profile qr demo-video demo-post-process demo-gif benchmark-baseline benchmark-compare benchmark-tier1 profile-goroutines profile-block profile-mutex profile-trace build-mux install-mux install-service install-hooks rollback backup-binary uninstall-service setup-codesign _codesign-binary verify-codesign tcc-reset preview dev-stack coverage-func coverage-gaps coverage-pkg coverage-refactor registry-generate-backend registry-generate-frontend registry-generate registry-diff e2e-report e2e-lighthouse build-tmux build-tmux-embed build-embedded build-embedded-tymux clean-tmux init-submodules fetch-tymuxd build-tymuxd-embed test-with-pinned-tmux test-trace test-profile vet-architecture vet-rpc-markers coverage-integration actor-field-guard ptmx-field-guard checklocks build-otel-auto build-otel-auto-embedded otel-auto-isolation-guard otel-auto-isolation-guard-selftest otel-auto-smoke otel-auto-smoke-suppression otel-auto-test

# Default target
help: ## Show this help message
	@echo "Stapler Squad Development Makefile"
	@echo "================================="
	@grep -E '^[a-zA-Z0-9._-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: ports
ports: ## Show reserved manual dev port block (see CLAUDE.md's "Manual dev port block")
	@base=$$(python3 -c "import zlib; print(61000 + zlib.crc32(b'stapler-squad') % 4525)" 2>/dev/null || echo 62871); \
	echo "Manual dev port block (base $$base = 61000 + CRC32(\"stapler-squad\") % 4525):"; \
	echo "  $$base = manual instance #1 - PORT"; \
	echo "  $$((base+1)) = manual instance #1 - --remote-port"; \
	echo "  $$((base+2)) = manual instance #2 - PORT"; \
	echo "  $$((base+3)) = manual instance #2 - --remote-port"; \
	echo "  $$((base+4))-$$((base+9)) = spare"; \
	echo ""; \
	echo "Fixed (documented, do not reassign): :8543 main service, :8444 remote-access default"

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
	@protos="$$(tools/scanner/list-backend-protos.sh)" || exit 1; \
	for proto in $$protos; do \
		./$(BACKEND_SCANNER_BIN) "$$proto" server/services/ $(BACKEND_FEATURES_DIR) || exit 1; \
	done
	@# Generation is additive; prune files whose RPC no longer exists so the
	@# committed set stays in sync with the proto (avoids registry-validation drift).
	@bash tools/scanner/prune-stale-backend.sh $(BACKEND_FEATURES_DIR)
	@echo "✅ Backend per-feature files written to $(BACKEND_FEATURES_DIR)/"

registry-generate-frontend: ## Generate frontend feature registry from React component markers
	@echo "Installing frontend scanner dependencies..."
	@cd tools/scanner/frontend && pnpm install --silent
	@echo "Scanning frontend features..."
	@tools/scanner/frontend/node_modules/.bin/ts-node \
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

docs-features: ## Generate per-feature Markdown docs from the typed catalog
	@echo "Generating feature docs..."
	@mkdir -p docs/api/features
	@cd tools/docs-gen && npm install --silent && npx ts-node --project tsconfig.json generate.ts

changelog-since: ## Print features introduced since a version: make changelog-since VERSION=1.4.0
	@cd tools/docs-gen && npx ts-node --project tsconfig.json changelog.ts $(VERSION)

e2e-report: ## Generate Allure HTML report from last test run
	@cd tests/e2e && npx allure generate allure-results --clean -o allure-report
	@echo "✅ Report generated at tests/e2e/allure-report/index.html"

e2e-lighthouse: ## Run Lighthouse CI performance audit
	@cd tests/e2e && npx lhci autorun --config=lighthouse.config.js

# Build targets
build: stapler-squad ## Build the Go application

stapler-squad: ensure-tools proto-gen ent-gen server/web/dist $(GO_FILES) ## Build the Go binary
	@echo "Building Go application..."
ifeq ($(UNAME_S),Darwin)
	CGO_LDFLAGS="-sectcreate __TEXT __info_plist $(CURDIR)/macos/Info.plist" \
		go build -ldflags "$(LDFLAGS)" -o stapler-squad .
	@# Verify Info.plist was actually embedded (catches silent CGO_ENABLED=0 failures)
	@otool -s __TEXT __info_plist "$(CURDIR)/stapler-squad" | grep -q "Contents of" || \
		(echo "ERROR: Info.plist was not embedded. Ensure CGO_ENABLED=1 and try again." && exit 1)
else
	go build -ldflags "$(LDFLAGS)" -o stapler-squad .
endif
	@echo "✅ stapler-squad built successfully"

# Install web-app pnpm dependencies when pnpm-lock.yaml changes
web-app/node_modules/.modules.yaml: web-app/package.json web-app/pnpm-lock.yaml
	@echo "Installing web-app pnpm dependencies..."
	@cd web-app && pnpm install --frozen-lockfile

# Build Next.js app to web-app/out
web-app/out: ensure-tools proto-gen web-app/node_modules/.modules.yaml $(WEB_FILES) web-app/next.config.ts
	@# Guard: re-install if node_modules was wiped by external tools without touching pnpm-lock.yaml
	@test -d web-app/node_modules/next || { \
		echo "⚠️  node_modules incomplete, re-installing..."; \
		cd web-app && pnpm install --frozen-lockfile; \
	}
	@echo "Building Next.js web UI (development mode for better error messages)..."
	@mkdir -p $(HOME)/.stapler-squad/nextjs-webpack-cache
	@cd web-app && NEXT_BUILD_MODE=development NEXTJS_SHARED_CACHE_DIR=$(HOME)/.stapler-squad/nextjs-webpack-cache ../scripts/retry-with-backoff.sh -n 3 -s 5 -- pnpm run build
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
	@-pkill -f "(^|/)stapler-squad([[:space:]]|$$)" 2>/dev/null || true
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
	@-pkill -f "(^|/)stapler-squad([[:space:]]|$$)" 2>/dev/null || true
	@sleep 1
	@echo "Starting server..."
	@./stapler-squad $(PROFILE_FLAGS) &
	@sleep 2
	@echo "✅ Server restarted at http://localhost:8543"
	@if [ -n "$(PROFILE_FLAGS)" ]; then \
		echo "📊 Profiling enabled at http://localhost:$(PROFILE_PORT)/debug/pprof/"; \
	fi

install: ensure-tools install-hooks ## Install stapler-squad locally
	go install .

install-hooks: ## Build and install ssq-hooks + ssq-hook-handler to ~/.local/bin (called by install and install-service)
	mkdir -p ~/.local/bin
	go build -o ~/.local/bin/ssq-hooks ./cmd/ssq-hooks/
	@# Stable path for the notification hook handler so the server can register
	@# it during onboarding (InstallHooks RPC). See internal/claudehooks.
	install -m 0755 scripts/ssq-hook-handler ~/.local/bin/ssq-hook-handler

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

BIN_TMUX        := bin/tmux
TMUX_BUILD_STAMP := .tmux-build.stamp

# Stamp-file approach: only rebuild when submodule source changes
$(BIN_TMUX): $(TMUX_BUILD_STAMP)
	@true

# scripts/build-tmux.sh self-heals an uninitialized/empty submodule (fresh
# worktrees don't auto-init submodules), so it must run unconditionally
# rather than gating on configure.ac already existing.
$(TMUX_BUILD_STAMP):
	@$(MAKE) build-tmux
	@touch $(TMUX_BUILD_STAMP)

init-submodules: ## Initialize git submodules (required once after clone)
	git submodule update --init --recursive

build-tmux: ## Build pinned tmux 3.4 binary from third_party/tmux submodule
	@./scripts/build-tmux.sh

build-tmux-embed: build-tmux ## Copy built tmux into the embed dir for go build -tags embed_tmux
	@mkdir -p session/tmux/embed
	@cp $(BIN_TMUX) session/tmux/embed/tmux
	@echo "✅ session/tmux/embed/tmux ready ($(shell $(BIN_TMUX) -V 2>/dev/null || echo unknown))"

# ── Pinned tymuxd binary (fetched, not compiled — see ADR-001) ─────────────
# Fetches a prebuilt tymuxd release binary from github.com/tstapler/tymux
# instead of requiring a cargo/rustc toolchain anywhere in this repo.

# Bump this and the github.com/tstapler/tymux/clients/go/gen/tymux/v1 require
# in go.mod together — nothing enforces this automatically; see ADR-001
# (project_plans/tymux-bundled-integration/decisions/ADR-001-prebuilt-tymuxd-binary-download.md).
TYMUX_VERSION ?= v1.0.0
BIN_TYMUXD        := session/tymux/embed/tymuxd
TYMUXD_FETCH_STAMP := .tymuxd-fetch.stamp

# Stamp-file approach: only re-fetch when TYMUX_VERSION changes. Unlike
# TMUX_BUILD_STAMP (gated on submodule source files changing), the fetch
# input here is a variable, not a file, so the stamp records the last-fetched
# version and FORCE re-evaluates that comparison on every invocation.
$(BIN_TYMUXD): $(TYMUXD_FETCH_STAMP)
	@true

$(TYMUXD_FETCH_STAMP): FORCE
	@if [ ! -f $(BIN_TYMUXD) ] || [ "$$(cat $@ 2>/dev/null)" != "$(TYMUX_VERSION)" ]; then \
		TYMUX_VERSION=$(TYMUX_VERSION) ./scripts/fetch-tymuxd.sh && \
		echo "$(TYMUX_VERSION)" > $@; \
	fi

.PHONY: FORCE
FORCE:

fetch-tymuxd: $(BIN_TYMUXD) ## Fetch pinned tymuxd release binary (no cargo/rustc required)

build-tymuxd-embed: fetch-tymuxd ## Confirm tymuxd is present in the embed dir for go build -tags embed_tymux
	@echo "✅ session/tymux/embed/tymuxd ready ($(shell du -h $(BIN_TYMUXD) 2>/dev/null | cut -f1 || echo unknown))"

build-embedded: build-tmux-embed ## Build stapler-squad with tmux bundled inside the binary
ifeq ($(UNAME_S),Darwin)
	CGO_LDFLAGS="-sectcreate __TEXT __info_plist $(CURDIR)/macos/Info.plist" \
		go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o stapler-squad .
	@otool -s __TEXT __info_plist "$(CURDIR)/stapler-squad" | grep -q "Contents of" || \
		(echo "ERROR: Info.plist was not embedded in embedded build." && exit 1)
else
	go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o stapler-squad .
endif
	@echo "✅ stapler-squad built with embedded tmux"

# build-embedded-tymux: kept separate from build-embedded (tags "embed_tmux
# embed_tymux" instead of changing build-embedded's tag set) so existing
# CI/release artifacts depending on -tags embed_tmux alone are unaffected.
# Darwin CGO_LDFLAGS/Info.plist branch mirrors build-embedded unchanged —
# tymuxd embedding doesn't touch TCC entitlement plumbing.
build-embedded-tymux: build-tmux-embed build-tymuxd-embed ## Build stapler-squad with tmux AND tymuxd bundled inside the binary
ifeq ($(UNAME_S),Darwin)
	CGO_LDFLAGS="-sectcreate __TEXT __info_plist $(CURDIR)/macos/Info.plist" \
		go build -tags "embed_tmux embed_tymux" -ldflags "$(LDFLAGS)" -o stapler-squad .
	@otool -s __TEXT __info_plist "$(CURDIR)/stapler-squad" | grep -q "Contents of" || \
		(echo "ERROR: Info.plist was not embedded in embedded build." && exit 1)
else
	go build -tags "embed_tmux embed_tymux" -ldflags "$(LDFLAGS)" -o stapler-squad .
endif
	@echo "✅ stapler-squad built with embedded tmux + tymuxd"

# build-otel-auto: opt-in, structurally isolated build path (go-auto-instrumentation
# project, project_plans/go-auto-instrumentation/). Never a prerequisite of build,
# ci, ready, quick-check, pre-commit, or install-service — see otel-auto-isolation-guard
# below, which fails ci if that ever changes. -tags embed_tmux is supported here
# (Spike A passed for -tags; see spike-verdicts.md) via build-otel-auto-embedded.
# No macOS CGO_LDFLAGS/Info.plist branch yet — deferred, see plan.md Unresolved
# Question 6.
build-otel-auto: ensure-tools proto-gen ent-gen server/web/dist ## Build stapler-squad-otel with otelc compile-time auto-instrumentation (opt-in — see project_plans/go-auto-instrumentation)
	@which otelc >/dev/null 2>&1 || (echo "❌ otelc not found on PATH. Install it from https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation (see project_plans/go-auto-instrumentation/implementation/spike-verdicts.md for the exact install command used in this repo)." && exit 1)
	./scripts/otel-auto-build.sh go build -ldflags "$(LDFLAGS)" -o stapler-squad-otel .
	@echo "✅ stapler-squad built with otelc auto-instrumentation → ./stapler-squad-otel"

build-otel-auto-embedded: ensure-tools proto-gen ent-gen server/web/dist build-tmux-embed ## Build stapler-squad-otel with tmux bundled + otelc auto-instrumentation (opt-in)
	@which otelc >/dev/null 2>&1 || (echo "❌ otelc not found on PATH. Install it from https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation (see project_plans/go-auto-instrumentation/implementation/spike-verdicts.md for the exact install command used in this repo)." && exit 1)
	./scripts/otel-auto-build.sh go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o stapler-squad-otel .
	@echo "✅ stapler-squad built with embedded tmux + otelc auto-instrumentation → ./stapler-squad-otel"

otel-auto-isolation-guard: ## Prove build-otel-auto is unreachable from ci/ready/quick-check/pre-commit/install-service (Story 2.1.3)
	@./scripts/otel-auto-isolation-guard.sh

otel-auto-isolation-guard-selftest: ## Prove the Isolation Guard's own detection logic actually fires (injects a deliberate leak into a temp Makefile copy)
	@./scripts/otel-auto-isolation-guard.sh --self-test

otel-auto-smoke: ## Verify stapler-squad-otel actually emits a db.system span (Collector Smoke Test; needs a local OTLP collector on :4317)
	@./scripts/otel-auto-smoke.sh

otel-auto-smoke-suppression: ## Verify stapler-squad-otel emits nothing when OTEL_ENABLED=false (Suppression Smoke Test; needs a local OTLP collector on :4317)
	@./scripts/otel-auto-smoke.sh --suppression

# otel-auto-test: the only repeatable way to run
# instrumentation/otelc/safeexec's hook_test.go, since that package is gated
# behind the otelcauto build tag AND requires the transient `otelc setup`
# scaffolding hook.go's go.opentelemetry.io/otelc/pkg/hook import depends on
# (see hook.go's package doc). Mirrors (rather than directly shells out to)
# otel-auto-build.sh's module-backup/GOFLAGS/cleanup lifecycle via
# scripts/otel-auto-test.sh — see that script's header comment for why it
# can't just call otel-auto-build.sh with the test packages directly (a
# genuine `otelc setup` chicken-and-egg failure when the rule-implementation
# package itself is a setup target). go.mod/go.sum end up byte-identical to
# HEAD afterward, same as build-otel-auto. Never a prerequisite of
# ci/ready/quick-check/pre-commit/install-service — see
# otel-auto-isolation-guard, which fails ci if that ever changes.
otel-auto-test: ensure-tools ## Run instrumentation/otelc/safeexec + telemetry tests under the otelcauto build tag (opt-in — see project_plans/go-auto-instrumentation)
	@./scripts/otel-auto-test.sh
	@echo "✅ otelc auto-instrumentation hook tests passed"

clean-tmux: ## Remove the built tmux binary and submodule build artifacts
	@./scripts/build-tmux.sh --clean
	@rm -f $(TMUX_BUILD_STAMP)
	@rm -f session/tmux/embed/tmux
	@echo "✅ tmux artifacts cleaned"

backup-binary: ## Snapshot the current binary to stapler-squad.prev before a new build (called by install-service)
	@if [ -f ./stapler-squad ]; then \
		cp -f ./stapler-squad ./stapler-squad.prev; \
		echo "==> Saved current binary to ./stapler-squad.prev"; \
	fi

install-service: backup-binary build install-hooks ## Install stapler-squad as a system service (systemd on Linux, LaunchAgent on macOS)
ifeq ($(UNAME_S),Darwin)
	@$(MAKE) _codesign-binary
endif
	@STAPLER_SQUAD_BIN="$(CURDIR)/stapler-squad" ./scripts/install-service.sh $(if $(NO_PROFILE),--no-profile) $(if $(PROFILE_PORT),--profile-port $(PROFILE_PORT))

sync-worktrees: ## Merge main into every worktree (skips dirty ones, reports conflicts for manual resolution)
	@./scripts/sync-worktrees.sh

rollback: ## Restore the previous build (stapler-squad.prev) and restart the service
	@if [ ! -f ./stapler-squad.prev ]; then \
		echo "✗ No previous build found (./stapler-squad.prev does not exist)"; \
		exit 1; \
	fi
	@echo "==> Restoring previous build..."
	@cp -f ./stapler-squad.prev ./stapler-squad
	@echo "✓ Binary restored from stapler-squad.prev"
ifeq ($(UNAME_S),Darwin)
	@$(MAKE) _codesign-binary
endif
	@STAPLER_SQUAD_BIN="$(CURDIR)/stapler-squad" ./scripts/install-service.sh $(if $(NO_PROFILE),--no-profile) $(if $(PROFILE_PORT),--profile-port $(PROFILE_PORT))

_codesign-binary: ## Sign the binary with StaplerSquadDev cert (called by install-service on macOS)
	@if ! ./scripts/check-codesign.sh; then \
		echo "  StaplerSquadDev signing cert not found."; \
		echo "   Run 'make setup-codesign' once to create it, then retry."; \
		exit 1; \
	fi
	@echo "Signing binary..."
	codesign --force \
		--sign "StaplerSquadDev" \
		--entitlements "$(CURDIR)/entitlements.plist" \
		"$(CURDIR)/stapler-squad"
	@echo "Binary signed"

uninstall-service: ## Remove the system service and disable auto-start on login
	@./scripts/install-service.sh --uninstall

setup-codesign: ## (macOS only) Create self-signed codesign certificate for stable TCC identity
	@# Requires OpenSSL (not LibreSSL). Set OPENSSL_BIN to override, e.g.:
	@#   OPENSSL_BIN=$(brew --prefix openssl)/bin/openssl make setup-codesign
	@./scripts/setup-codesign.sh

verify-codesign: ## Verify binary code signing status and TCC identity
	@echo "=== Code Signature ==="
	@codesign -dv --verbose=4 "$(CURDIR)/stapler-squad" 2>&1
	@echo ""
	@echo "=== Designated Requirement (must be cert-anchored, not cdhash-anchored) ==="
	@codesign -d --requirements - "$(CURDIR)/stapler-squad" 2>&1
	@# PASS: DR contains "anchor trusted" or "anchor H\"<cert-sha1>\""
	@# FAIL: DR contains only "cdhash H\"<binary-hash>\"" — means ad-hoc or no cert
	@echo ""
	@echo "=== Entitlements ==="
	@codesign -d --entitlements - "$(CURDIR)/stapler-squad" 2>&1
	@echo ""
	@echo "=== Embedded Info.plist ==="
	@# NOTE: otool -s output is offset+hex+ASCII interleaved; strip offsets and ASCII
	@# before feeding to xxd. The awk extracts the 2nd-5th columns (hex groups only).
	@otool -s __TEXT __info_plist "$(CURDIR)/stapler-squad" | \
		tail -n +2 | \
		awk '{for(i=2;i<=NF&&length($$i)==8;i++){s=$$i; printf "%s%s%s%s",substr(s,7,2),substr(s,5,2),substr(s,3,2),substr(s,1,2)}; print ""}' | \
		tr -d '\n' | xxd -r -p | plutil -p - 2>&1 || \
		echo "(no embedded plist — Info.plist not embedded; check CGO_ENABLED=1)"

tcc-reset: ## Reset TCC grants for stapler-squad (causes one-time re-prompt; use during development only)
	@echo "Resetting TCC grants for com.stapler-squad..."
	@# Must use sudo; without it the system TCC DB (FDA) reset is silently skipped.
	@# Do NOT add || true: sudo failure should surface as an error, not silently succeed.
	sudo tccutil reset All com.stapler-squad
	@echo "Done. Grants will be re-prompted on next launch."

# Isolated preview instance — does NOT touch the running global service.
# Auto-picks a free port and uses a branch-scoped STAPLER_SQUAD_INSTANCE so
# state never bleeds into your real sessions.
#
# Usage:
#   make preview          # build + start; press Ctrl-C to stop
#   make preview PORT=8599
#
# The instance name is derived from the current git branch, so switching
# branches and running `make preview` again starts a completely separate
# workspace.  All data lives under ~/.stapler-squad/<instance>/.
preview: build ## Build and run an isolated preview instance (auto-picks port, branch-scoped instance)
	$(eval PREVIEW_PORT := $(or $(PORT),$(shell python3 -c "import socket,random; s=socket.socket(); s.bind(('',0)); p=s.getsockname()[1]; s.close(); print(p)")))
	$(eval PREVIEW_INSTANCE := preview-$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null | tr '/' '-'))
	@echo "▶  Starting isolated preview: STAPLER_SQUAD_INSTANCE=$(PREVIEW_INSTANCE)"
	@echo "▶  URL: http://localhost:$(PREVIEW_PORT)"
	@echo "▶  Press Ctrl-C to stop."
	STAPLER_SQUAD_INSTANCE=$(PREVIEW_INSTANCE) \
	STAPLER_SQUAD_USE_CONTROL_MODE=false \
	  ./stapler-squad --listen localhost:$(PREVIEW_PORT) --tmux-keep-server

# Isolated backend + next-dev DevStack for manual testing (Epic 3.2,
# scripts/dev-stack/launch.ts). Distinct from `make preview` (which runs the
# built Go binary alone against the static-exported web UI) — this spins up
# BOTH a backend and a real `next dev` on separately allocated ports, wired
# together via STAPLER_SQUAD_EXTRA_ORIGINS/NEXT_PUBLIC_API_URL, and tears
# both down (plus any orphaned grandchildren) on Ctrl-C.
#
# Usage:
#   make dev-stack NAME=my-feature-test
DEV_STACK_TS_NODE := web-app/node_modules/.bin/ts-node
DEV_STACK_TSC_OPTS := {"module":"commonjs","moduleResolution":"node","esModuleInterop":true}

dev-stack: build ## Start an isolated backend+next-dev DevStack: make dev-stack NAME=my-feature-test
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make dev-stack NAME=<instance-name>"; \
		exit 1; \
	fi
	$(DEV_STACK_TS_NODE) --compiler-options '$(DEV_STACK_TSC_OPTS)' scripts/dev-stack/launch.ts $(NAME)

# Protocol Buffer code generation
proto-gen: ensure-tools web-app/node_modules/.modules.yaml ## Generate Go and TypeScript code from proto files
	@echo "Checking if proto files need regeneration..."
	@if [ ! -f $(PROTO_STAMP) ] \
	   || [ "$$(find proto -name '*.proto' -newer $(PROTO_STAMP) -print -quit)" ] \
	   || [ web-app/node_modules/.bin/protoc-gen-es -nt $(PROTO_STAMP) ] \
	   || [ ! -f gen/proto/go/session/v1/session.pb.go ] \
	   || [ ! -f web-app/src/gen/session/v1/session_pb.ts ]; then \
		echo "Generating protocol buffer code..."; \
		buf generate proto; \
		echo "✅ Code generation complete"; \
		echo "  Go code:         gen/proto/go/"; \
		echo "  TypeScript code: web-app/src/gen/"; \
		touch $(PROTO_STAMP); \
	else \
		echo "✅ Proto files unchanged, skipping generation"; \
	fi

ent-gen: ensure-tools ## Generate ent ORM code from session/ent/schema
	@if [ ! -f $(ENT_STAMP) ] \
	   || [ "$$(find session/ent/schema -name '*.go' -newer $(ENT_STAMP) -print -quit)" ]; then \
		echo "Generating ent ORM code..."; \
		go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema; \
		touch $(ENT_STAMP); \
		echo "✅ ent ORM code generated"; \
	else \
		echo "✅ ent schema unchanged, skipping generation"; \
	fi

proto-lint: ensure-tools ## Lint protocol buffer files
	buf lint proto

proto-build: ensure-tools ## Build/validate protocol buffer files
	buf build proto

proto-clean: ## Clean generated protocol buffer code
	rm -rf gen/proto/go
	rm -rf web/src/gen

# Testing targets
test: ensure-tools proto-gen $(BIN_TMUX) ## Run all tests (skips slow integration tests; use test-integration for full suite)
	# session, session/mux, and session/tmux fork real tmux subprocesses at high
	# t.Parallel() fan-out. Running them under the suite's default per-package
	# parallelism let those tmux-heavy tests compete for scheduler time against
	# fixed wall-clock budgets (destroyChainTimeout, mux list-session timeouts),
	# causing intermittent failures under `make test` that never reproduced in
	# isolation -- same root cause as test-integration's existing -p 1 scoping
	# below and CI's -race -p 1 coverage step (.github/workflows/build.yml). -p 1
	# serializes just these three packages against each other; everything else
	# still runs in parallel via the second invocation.
	# STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 matches CI's existing override
	# so local runs get the same tmux-create timeout headroom (production
	# default is 10s -- see session/tmux/tmux.go's sessionCreateTimeoutDefault).
	# testutil also forks real tmux subprocesses (TestRealTmuxSessionLifecycle) and
	# hit the same contention under full-suite load -- included in the -p 1 group.
	STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -short -timeout=20m -p 1 ./session ./session/mux ./session/tmux ./testutil
	STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -short -timeout=20m $$(go list ./... | grep -vE '^github\.com/tstapler/stapler-squad/(session|session/mux|session/tmux|testutil)$$')

test-verbose: ensure-tools proto-gen ## Run tests with verbose output
	go test -short -v ./...

test-coverage: ensure-tools proto-gen $(BIN_TMUX) ## Run tests with coverage report (HTML)
	# Same tmux-contention root cause as the test target above -- see its comment.
	# Split into two invocations and merge the resulting coverage profiles since
	# go test only writes one -coverprofile per invocation.
	STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -short -timeout=20m -p 1 -cover -coverprofile=coverage.tmux.out ./session ./session/mux ./session/tmux ./testutil
	STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -short -timeout=20m -cover -coverprofile=coverage.rest.out $$(go list ./... | grep -vE '^github\.com/tstapler/stapler-squad/(session|session/mux|session/tmux|testutil)$$')
	head -n 1 coverage.tmux.out > coverage.out
	tail -q -n +2 coverage.tmux.out coverage.rest.out >> coverage.out
	rm -f coverage.tmux.out coverage.rest.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

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

test-race: ensure-tools proto-gen $(BIN_TMUX) ## Run tests with race detector enabled (skips slow integration tests)
	# Same tmux-contention root cause as the test target above -- see its comment.
	STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -race -short -timeout=20m -p 1 ./session ./session/mux ./session/tmux ./testutil
	STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -race -short -timeout=20m $$(go list ./... | grep -vE '^github\.com/tstapler/stapler-squad/(session|session/mux|session/tmux|testutil)$$')

test-integration: ensure-tools proto-gen ## Run integration tests (requires real tmux)
	# ./session and ./session/tmux are the only integration-tagged packages that
	# fork real tmux servers (server/mcp and session/headless don't touch tmux).
	# Running the full suite's default per-package parallelism let those two
	# packages' tmux-heavy tests fork/poll real tmux servers concurrently and
	# compete for scheduler time -- root cause of intermittent failures like
	# TestTmuxServerRegistry_ConcurrentSubscriptions/PaneExitDetectedDespiteElevatedBackoff
	# missing their timing budgets under `make ci` while always passing in
	# isolation (see registryPollTimeout's comment in
	# session/tmux/server_registry_integration_test.go). -p 1 serializes just
	# these two packages against each other; everything else still runs in
	# parallel via the second invocation.
	go test -race -tags integration -timeout 20m -p 1 ./session ./session/tmux
	go test -race -tags integration -timeout 20m $$(go list ./... | grep -vE '^github\.com/tstapler/stapler-squad/(session|session/tmux)$$')

test-triage-harness: proto-gen ## Run all backlog triage harness phases (no UI/browser needed)
	go test -v -tags=harness -run TestTriageHarness ./server/services/

test-triage-gate: proto-gen ## Phase 1: verify TriggerTriage is blocked when repoPath is empty
	go test -v -tags=harness -run TestTriageHarness/Gate ./server/services/

test-triage-trigger: proto-gen ## Phase 2: trigger triage and poll until item reaches ready status
	go test -v -tags=harness -run TestTriageHarness/TriggerAndPoll ./server/services/

test-triage-parser: proto-gen ## Phase 3: verify parser tolerates LLM preamble before JSON block
	go test -v -tags=harness -run TestTriageHarness/ParserRobust ./server/services/

test-triage-flow: proto-gen ## Phase 4: full flow — create, gate, set repoPath, trigger, verify
	go test -v -tags=harness -run TestTriageHarness/FullFlow ./server/services/

test-triage-real: proto-gen ## Run triage with a REAL Claude session (requires claude in PATH, ~30s)
	go test -v -tags=harness -run TestTriageHarness_RealClaude ./server/services/ -timeout 5m

coverage-integration: ensure-tools proto-gen ## Run integration tests, emit integration.out
	go test -race -tags integration -coverprofile=integration.out -covermode=atomic ./...
	@echo "✅ Integration coverage written to integration.out"

test-ux-polish: ## Run tests registered in docs/registry/features/ (no server/tmux required)
	@RUN=$$(python3 -c "import json,glob; ids=[t for p in glob.glob('docs/registry/features/backend/**/*.json',recursive=True) for t in json.load(open(p)).get('testIds',[])]; print('|'.join(sorted(set(ids))))"); \
	echo "Running: $$RUN"; \
	go test ./server/services/ -run "$$RUN" -v -timeout 120s
	go test ./session/prompts/... -v -timeout 30s

test-with-pinned-tmux: ensure-tools proto-gen $(BIN_TMUX) ## Run tests using the pinned tmux binary (reproducible)
	TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -race ./...

test-trace: ensure-tools proto-gen $(BIN_TMUX) ## Run session tests with execution trace (open trace with: go tool trace /tmp/ss-test-trace.out)
	@echo "Running session tests with execution trace..."
	TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -v -trace /tmp/ss-test-trace.out -timeout 120s \
		github.com/tstapler/stapler-squad/session/... 2>&1 | tee /tmp/ss-test-trace.log
	@echo "Trace saved to /tmp/ss-test-trace.out — view with: go tool trace /tmp/ss-test-trace.out"

test-profile: ensure-tools proto-gen $(BIN_TMUX) ## Run session tests with CPU+block profiling
	@echo "Running session tests with profiling..."
	TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -v -cpuprofile /tmp/ss-test-cpu.prof \
		-blockprofile /tmp/ss-test-block.prof -timeout 120s \
		github.com/tstapler/stapler-squad/session/... 2>&1 | tee /tmp/ss-test-profile.log
	@echo "CPU profile: go tool pprof /tmp/ss-test-cpu.prof"
	@echo "Block profile: go tool pprof /tmp/ss-test-block.prof"

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
	go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@latest
	go install github.com/mibk/dupl@latest
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

lint: ensure-tools proto-gen ent-gen server/web/dist lint-custom lint-shell ## Run golangci-lint with comprehensive checks
	@GOBIN=$$(go env GOBIN); \
	if [ -z "$$GOBIN" ]; then GOBIN=$$(go env GOPATH)/bin; fi; \
	if ! which golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint v2..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
	fi; \
	CGO_ENABLED=0 golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet

LINTER_BIN := $(CURDIR)/bin/linter

lint-custom: $(LINTER_BIN) ## Run project-specific custom linters (entfullscan, hotpolllog, nocommandpattern, norawexec, norawgitopen, silenttransition, tmuxsocketscope) in a single pass
	@echo "Running custom lint..."
	@$(LINTER_BIN) $(shell go list ./... | grep -v "^github.com/tstapler/stapler-squad$$")
	@echo "custom lint: ok"

$(LINTER_BIN):
	@mkdir -p $(CURDIR)/bin
	@go -C tools/lint build -o $(LINTER_BIN) ./cmd/linter

# Excludes third_party/ (vendored tmux source — not ours to lint) and
# node_modules/. Includes scripts/ssq-hook-handler, which has no .sh
# extension but is a real bash script (installed as a hook handler).
SHELL_SCRIPTS := $(shell find . -not -path "./third_party/*" -not -path "*/node_modules/*" -not -path "./.git/*" -type f \( -name "*.sh" -o -name "ssq-hook-handler" \))

lint-shell: ## Run shellcheck over all first-party shell scripts
	@which shellcheck >/dev/null 2>&1 || (echo "shellcheck not installed; run 'brew install shellcheck' (macOS) or see https://github.com/koalaman/shellcheck#installing" && exit 1)
	@echo "Running shellcheck on $(words $(SHELL_SCRIPTS)) first-party shell script(s)..."
	@shellcheck -x $(SHELL_SCRIPTS)
	@echo "shellcheck: ok"

actor-lint: ## Detect actor self-deadlock patterns using ast-grep (sg)
	@which sg >/dev/null 2>&1 || (echo "sg (ast-grep) not installed; run: cargo install ast-grep" && exit 1)
	sg scan --rule session/.sg-rules/actor-lint.yml session/

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

lint-css-tokens: ## Fail if any component .css.ts file uses hardcoded hex colors instead of vars.color.*
	@echo "Checking for hardcoded colors in component .css.ts files..."
	@violations=$$(find web-app/src -name '*.css.ts' \
	  ! -name 'theme.css.ts' \
	  ! -name 'theme-contract.css.ts' \
	  ! -name 'ThemePicker.css.ts' \
	  ! -path '*/debug/escape-codes/page.css.ts' \
	  | while read f; do \
	    if grep '#[0-9a-fA-F]\{3,8\}' "$$f" 2>/dev/null | grep -v '^[[:space:]]*\*' | grep -qv '//.*#[0-9a-fA-F]\{3,8\}'; then echo "$$f"; fi; \
	  done); \
	if [ -n "$$violations" ]; then \
	  echo "❌ Hardcoded hex colors found in component .css.ts files (use vars.color.* instead):"; \
	  for f in $$violations; do \
	    grep -n '#[0-9a-fA-F]\{3,8\}' "$$f" | grep -v '^[0-9]*:[[:space:]]*\*' | grep -v '//.*#[0-9a-fA-F]\{3,8\}' | head -3 | sed "s|^|  $$f line |"; \
	  done; \
	  exit 1; \
	fi
	@echo "✅ No hardcoded colors in component .css.ts files"

format: ensure-tools ## Format code with gofmt
	go fmt ./...

fmt-check: ## Verify all Go files are gofmt-formatted (non-destructive; exits 1 if any are not)
	@UNFORMATTED=$$(gofmt -l . | grep -v vendor | grep -v "^\.claude/"); \
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

# Mutex discipline enforcement via gVisor checklocks
# Uses go vet -vettool to enforce explicit +checklocks: field annotations.
# -inferred=false: only report violations of explicit annotations, not suggestions.
# -atomic=false: skip atomic-inside-lock checks (handled by race detector in tests).
# Skips packages where deadlock.Mutex wrapping causes false-positive "return with
# unexpected locks held" reports (./session top-level, ./server/...).
# Install: go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@latest
checklocks: ## Enforce +checklocks: mutex-discipline annotations (explicit violations only)
	@if ! which checklocks >/dev/null 2>&1; then \
		echo "Installing checklocks..."; \
		go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@latest; \
	fi
	checklocks -inferred=false -atomic=false ./session/git/... ./session/detection/... ./session/artifacts/... ./session/cdp/... ./session/scrollback/... ./session/mux/... ./executor/... ./log/... ./config/... ./pkg/...

# Comprehensive analysis
analyze: install-tools vet lint staticcheck nil-safety security deadcode checklocks ## Run all static analysis tools

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

ci: build $(BIN_TMUX) test test-race vet lint lint-css-tokens test-integration fmt-check registry-generate actor-field-guard ptmx-field-guard otel-auto-isolation-guard ## Full CI pipeline: proto→web→build→tests→lint→fmt→registry

# ready: everything `make ci` runs, plus the CI-only checks that have no local
# equivalent yet — .github/workflows/lint.yml's complexity gate (gocyclo/
# gocognit/funlen/revive, new-code-only) and web-app's ESLint/CSS lint/scanner/
# ci-gates suites. Skips checks that need live PR/GH-Action context with no
# local equivalent (the external go-test-coverage action, the E2E-coverage PR
# comment) — those only run in CI. `--new-from-rev=origin/main` requires a
# reachable origin/main; `git fetch origin main` first if it's stale.
ready: ci ready-complexity-gate ready-duplication-gate-web ## Local approximation of every required PR check (make ci + complexity/duplication gates + web-app lint/scanner suites)
	cd web-app && npx next lint
	cd web-app && pnpm run lint:css && pnpm run lint:css-vars
	cd tools/scanner && go test ./...
	cd tools/ci-gates && pnpm install --silent && pnpm test
	@echo "✅ ready: local approximation of PR checks complete"

ready-complexity-gate: ensure-tools ## New-code-only gocyclo/gocognit/funlen/revive/dupl gate, mirroring lint.yml's PR-only complexity/duplication check
	@GOBIN=$$(go env GOBIN); \
	if [ -z "$$GOBIN" ]; then GOBIN=$$(go env GOPATH)/bin; fi; \
	if ! which golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint v2..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
	fi; \
	golangci-lint run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 \
		--enable=gocyclo,gocognit,funlen,revive,dupl --new-from-rev=origin/main

# Blocking (2026-08-24): web-app/.jscpd.json sets minLines/minTokens to a precision-tuned
# 20/200 (empirically verified: every finding at that size was real, actionable duplication —
# not test-boilerplate/token noise) and a 0.1% threshold. jscpd exits non-zero via its own
# --threshold flag when duplication exceeds that. The threshold is a RATCHET, not zero-tolerance:
# jscpd has no git-diff scoping like golangci-lint's --new-from-rev (kibitzer is the intended
# long-term fix for that gap, tstapler/kibitzer#28), so blocking on literal zero would fail
# every PR the moment two Jest files legitimately need the same set of jest.mock(...)
# registrations (those calls can't be extracted into a shared function — babel-jest hoists them
# per-file — so a small amount of that specific duplication is irreducible, ~0.09% today).
# 0.1% leaves a small buffer above that irreducible baseline while still failing on any
# meaningfully-sized new duplication.
ready-duplication-gate-web: ## Blocking web-app duplication gate (jscpd, see web-app/.jscpd.json)
	cd web-app && pnpm run lint:duplicates

# Quick development workflows
quick-check: build $(BIN_TMUX) test-coverage test-race lint lint-css-tokens registry-diff ## Quick development validation
	@echo "✅ Quick validation complete"

pre-commit: format vet test test-race lint vet-architecture ## Pre-commit validation
	@echo "✅ Pre-commit checks passed"

actor-field-guard: ## IAC Epic 5 guard: fail if direct Instance field writes exist outside session/instance*.go and actor.go
	@echo "actor-field-guard: scanning for direct Instance field writes..."
	@if grep -rEn '\b(inst|instance|liveInst)\.[A-Z][a-zA-Z0-9]+ = [^=]' \
	    server/services/session_service.go \
	    session/pr_status_poller.go \
	    session/review_queue_poller.go \
	    session/autonomous_driver.go \
	    daemon/daemon.go \
	    2>/dev/null | grep -vE ':[0-9]+:[[:space:]]*//' ; then \
	    echo "❌ actor-field-guard: direct Instance field writes found — route through actor setters (see IAC Epic 5)"; \
	    exit 1; \
	fi
	@echo "✅ actor-field-guard: no direct Instance field writes"

ptmx-field-guard: ## tmux-ptmx-race-fix guard: fail if ptmx/attachCmd/attachCmdWaitOnce are touched outside the ptmxMu helpers
	@echo "ptmx-field-guard: scanning session/tmux/*.go for direct PTY-triple field access..."
	@if grep -nE '\b[A-Za-z_][A-Za-z0-9_]*\.(ptmx|attachCmd|attachCmdWaitOnce)\b' session/tmux/*.go \
	    | grep -v '^session/tmux/shell_handle.go:' \
	    `# shell_handle.go declares its own unrelated ShellTmuxHandle.ptmx/attachCmd fields` \
	    `# (receiver "h", guarded by spawnMu, not ptmxMu) -- excluded by file, not by line marker,` \
	    `# because none of that file's lines ever legitimately touch the PTY triple this guards` \
	    | grep -vE ':[0-9]+:[[:space:]]*//' \
	    | grep -v 'allow-direct-ptmx-access' ; then \
	    echo "❌ ptmx-field-guard: direct PTY-triple field access found outside lockedPTMX/ptySnapshot/tryInstallPTYTriple/clearPTYTriple — route through the ptmxMu helpers (session/tmux/tmux.go)"; \
	    exit 1; \
	fi
	@echo "✅ ptmx-field-guard: no direct PTY-triple field access outside the guarded helpers"

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
	@cd tests/e2e && pnpm install --silent
	RECORD_DEMO=1 go test ./tests/demo/... -run TestRecordDemo -v -timeout 180s

demo-video: assets/demo.gif ## Record demo video, add browser chrome, and export GIF (assets/demo.webm + assets/demo.gif)

# Environment validation
validate-env: ensure-tools ## Validate development environment setup
	@echo "Validating development environment..."
	@go version
	@pnpm --version
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
