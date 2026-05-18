# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run

```bash
go build .                  # Build the application
./stapler-squad             # Run (web server on localhost:8543)

make install-service        # Build web UI + Go binary + install/restart system service (ALWAYS use this)
make uninstall-service      # Remove the service

STAPLER_SQUAD_USE_CONTROL_MODE=false ./stapler-squad   # Disable tmux control mode (legacy polling)
./stapler-squad --tmux-keep-server                     # Keep tmux server alive after sessions close

# Auto-rebuild on file changes
fswatch -o web-app/src | xargs -n1 -I{} make install-service
```

### Profiling

```bash
./stapler-squad --profile --trace
```

See `.claude/docs/profiling.md` for full pprof/goroutine dump instructions.
OpenTelemetry (Datadog/OTLP) setup: `.claude/docs/opentelemetry.md`

### Bundling tmux (single-binary deployment)

```bash
git submodule update --init third_party/tmux
make build-tmux             # Compile pinned tmux 3.4 (~30s)
make build-embedded         # Build stapler-squad with tmux embedded
make test-with-pinned-tmux  # Tests against pinned tmux (reproducible)
```

### Testing

```bash
make build && make test     # Build (generates protos) then test
make quick-check            # Build + test + lint (fast validation)
make ci                     # Full CI pipeline (definitive pre-push check)

go test ./server/services   # Specific packages (requires make build first)
go test ./ui -run TestFoo   # Specific test
make test-coverage

# Frontend tests (not part of make ci)
cd web-app && npx jest --no-coverage
cd web-app && npx jest --testPathPatterns="<pattern>" --no-coverage
```

Benchmark reference (all benchmarks MUST be run with `&`): `.claude/docs/benchmarks.md`

### Code Quality

```bash
make lint          # Linting — REQUIRED; make build fails if this fails
make quick-check   # Build + test + lint
make pre-commit    # Full pre-commit validation
make analyze       # All static analysis tools
make nil-safety    # Nil safety (NilAway + go vet -nilness)
make security      # gosec security scan
make install-tools # Install all dev tools
gofmt -w .         # Format before committing
```

Nil safety and static analysis tool reference: `.claude/docs/nil-safety.md`

### Go Concurrency Patterns

**Double-checked locking — always return the locally-computed value:**
In the pattern `read-lock → cache miss → compute → write-lock → conditional store`, always return the locally-computed value, not the cache slot. Re-reading the slot after a lost write race returns another goroutine's observation, which may contradict the current goroutine's computation.

```go
// WRONG: returns g.cache (another goroutine may have stored a different value)
g.mu.Lock()
if cacheExpired { g.cache = computed }
g.mu.Unlock()
return g.cache, nil

// CORRECT: always return locally-computed value
g.mu.Lock()
if cacheExpired { g.cache = computed }
g.mu.Unlock()
return computed, nil
```

## Application Data

State and logs live in `~/.stapler-squad/`:
- `logs/stapler-squad.log` — main log; check here for session creation issues
- `worktrees/` — git worktrees for isolated sessions
- `config.json`, `sessions.json`

**Key log patterns:** `Starting tmux session`, `timed out waiting for tmux session`, `DoesSessionExist()` polling

**State isolation** (workspace-based by default — per git directory):
```bash
STAPLER_SQUAD_INSTANCE=work ./stapler-squad       # Named instance
STAPLER_SQUAD_INSTANCE=shared ./stapler-squad     # Legacy global shared state
STAPLER_SQUAD_WORKSPACE_MODE=false ./stapler-squad
```
Full isolation reference: `.claude/docs/state-isolation.md`

**External session monitoring** (claude-mux PTY multiplexer for IntelliJ/VS Code terminals):
install via `./scripts/install-mux.sh` then `alias claude='claude-mux claude'`.
Full guide: `.claude/docs/pty-multiplexing.md`

## Architecture Overview

Go web server on `localhost:8543` + React SPA. Manages AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees.

| Layer | Path | Purpose |
|---|---|---|
| Web Server | `server/` | HTTP + ConnectRPC handlers, middleware |
| Session Mgmt | `session/` | Lifecycle, storage, tmux, git worktrees, scrollback |
| Config | `config/` | JSON config, state persistence |
| Web UI | `web-app/` | React SPA, real-time terminal via ConnectRPC |

Sessions support tag-based multi-dimensional organization with 8 grouping strategies (Category, Tag, Branch, Path, Program, Status, Session Type, None). Full reference: `.claude/docs/tag-organization.md`

## Pull Request Requirements

Use [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Effect |
|---|---|
| `fix:` | Patch bump |
| `feat:` | Minor bump |
| `feat!:` / `BREAKING CHANGE:` footer | Major bump |
| `chore:`, `docs:`, `refactor:`, `test:` | No bump (hidden from changelog) |

Releases are not automatic — release-please opens a "Release PR"; merge when ready to ship.

## Adding New Features

### New Web UI Features
1. Create React components in `web-app/src/components/`
2. Add ConnectRPC endpoints in `server/services/`
3. Update protobuf definitions in `proto/session/v1/` if needed → `make generate-proto`
4. Test with `make install-service`

### New Omnibar Capabilities
Two registries must stay in sync — see `.claude/rules/feature-testing-registry.md`:
- **OmnibarAction union** (`types.ts` + `dispatch.ts` + `dispatch.test.ts`) for user-triggerable actions
- **DetectorRegistry** (`detector.ts` + `detector.test.ts`) for auto-detected input patterns
- New session creation modes also require 7 touchpoints — see `.claude/rules/session-creation-registry.md`

### New Session Filters
1. Add filter params to ConnectRPC service definitions
2. Implement logic in `session/storage.go` or service layer
3. Update web UI filter components

### New API Endpoints
1. Define RPC in `proto/session/v1/session.proto` → `make generate-proto`
2. Implement handler in `server/services/`, register in `server/server.go`

### Modifying the ent ORM Schema

**CRITICAL:** Use the command from `session/ent/generate.go` — the `--feature sql/upsert` flag is required:

```bash
# CORRECT
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema

# WRONG — breaks UpsertRule and similar methods
go run entgo.io/ent/cmd/ent generate ./session/ent/schema
```

Workflow: edit schema → run correct generate → `go build ./...` → commit all `session/ent/` changes together.

## Feature Registry

Per-feature JSON files in `docs/registry/features/` map RPCs and components to feature IDs. One file per feature prevents merge conflicts.

```bash
make registry-generate   # Scan source → update per-feature files
make registry-diff       # Dry run: show what would change
make registry-aggregate  # Assemble monolithic JSON (local use only)
```

Run `make registry-generate` and commit changed files whenever you: add/rename a proto RPC, add a React page/component, or add/move a `// +api:` or `// +feature:` marker.

Markers: `// +api: session:create` in Go handlers; `// +feature: session-list` in first 10 lines of React files.

## E2E Tests

Tests in `tests/e2e/` use Playwright + Allure.

```bash
# Start test server first
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
cd tests/e2e && npm test
cd tests/e2e && npx playwright test session-lifecycle.spec.ts
make e2e-report
make e2e-lighthouse
```

**Conventions (enforced in CI):**
1. Every spec file starts with `// @feature session:create, ...`
2. No `waitForTimeout` — use `expect(locator).toHaveValue(...)` or `waitForSelector`
3. Locators use `data-testid` or ARIA roles only (no CSS class selectors)
4. New page helpers go in `tests/e2e/pages/`

**UX analysis CI** runs on PRs touching `web-app/src/`: Axe Core (blocks on WCAG AA violations), Lighthouse CI (warns if score < 70).

---

## Reference Documents Index

| Topic | File |
|---|---|
| Profiling / lock-up debugging | `.claude/docs/profiling.md` |
| OpenTelemetry / Datadog setup | `.claude/docs/opentelemetry.md` |
| PTY multiplexing (claude-mux) | `.claude/docs/pty-multiplexing.md` |
| State file isolation / multi-instance | `.claude/docs/state-isolation.md` |
| Tag-based session organization | `.claude/docs/tag-organization.md` |
| Benchmark reference | `.claude/docs/benchmarks.md` |
| Nil safety & static analysis tools | `.claude/docs/nil-safety.md` |
| CSS architecture (vanilla-extract) | `.claude/rules/css-architecture.md` |
| Feature registry rules | `.claude/rules/feature-registry.md` |
| Omnibar feature testing registry | `.claude/rules/feature-testing-registry.md` |
| Session creation mode registry (7 touchpoints) | `.claude/rules/session-creation-registry.md` |
