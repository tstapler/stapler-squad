# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run

```bash
go build .                  # Build the application
./stapler-squad             # Run (web server on localhost:8543)

make install-service        # Build web UI + Go binary + install/restart system service (ALWAYS use this)
make uninstall-service      # Remove the service
make setup-codesign         # (macOS, one-time) Create self-signed cert for TCC grant persistence
make verify-codesign        # Check binary signing status
make tcc-reset              # Reset TCC grants (development/debugging only)

See `.claude/docs/codesigning.md` for first-time setup and cert backup instructions.

STAPLER_SQUAD_USE_CONTROL_MODE=false ./stapler-squad   # Disable tmux control mode (legacy polling)
./stapler-squad --tmux-keep-server                     # Keep tmux server alive after sessions close
```

**WARNING:** `make install-service` restarts the running service, which kills the tmux server and every live tmux session with it — including any session you're currently working in — unless the deployed unit passes `--tmux-keep-server`. See `.claude/rules/tmux-keep-server-on-restart.md`.

### Profiling

```bash
./stapler-squad --profile --trace
```

See `.claude/docs/profiling.md` for full pprof/goroutine dump instructions.
OpenTelemetry (Datadog/OTLP) setup: `.claude/docs/opentelemetry.md`

### Bundling tmux

Single-binary deployment with embedded tmux: `.claude/docs/bundling-tmux.md`

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

### Go Skills — Always Invoke for Go Work

When writing, reviewing, or refactoring Go code, invoke the relevant skill(s):

| Task | Skill |
|---|---|
| General idioms, error handling, interfaces, naming, project structure | `/go-development` |
| Concurrency primitive selection (mutex vs atomic vs channel vs lock-free) | `/go-concurrency` |
| pprof profiling — CPU, memory, goroutine, mutex profiles | `/go-profiling` |
| Fix a specific pprof hotspot (atomic shadow, RWMutex, TTL cache, etc.) | `/go:optimize` |
| Goroutine fan-out, singleflight, avoiding mutex contention | `/go:parallelism` |

Invoke proactively — do not wait to be asked. If a task involves any `.go` file, load the appropriate skill before starting.

Subtle patterns (double-checked locking, etc.): `.claude/docs/concurrency-patterns.md`

## Application Data

State and logs live in `~/.stapler-squad/`:
- `logs/stapler-squad.log` — main log; check here for session creation issues
- `worktrees/` — git worktrees for isolated sessions
- `config.json`, `sessions.json`

**Key log patterns:** `Starting tmux session`, `timed out waiting for tmux session`, `DoesSessionExist()` polling

State isolation (workspace-based by default): `.claude/docs/state-isolation.md`
External session monitoring (ssq-mux for IDE terminals): `.claude/docs/pty-multiplexing.md`

## Architecture Overview

Go web server on `localhost:8543` + React SPA. Manages AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees.

| Layer | Path | Purpose |
|---|---|---|
| Web Server | `server/` | HTTP + ConnectRPC handlers, middleware |
| Session Mgmt | `session/` | Lifecycle, storage, tmux, git worktrees, scrollback |
| Config | `config/` | JSON config, state persistence |
| Web UI | `web-app/` | React SPA, real-time terminal via ConnectRPC |

Sessions support tag-based multi-dimensional organization with 8 grouping strategies (Category, Tag, Branch, Path, Program, Status, Session Type, None). Full reference: `.claude/docs/tag-organization.md`

## Git Remotes

| Remote | Repo | Role |
|---|---|---|
| `origin` | `tstapler/stapler-squad` | Personal fork |
| `upstream-fanatics` | `TylerStaplerAtFanatics/stapler-squad` | Work upstream (canonical) |

`tstapler-ssh` is a duplicate of `origin` (same repo, explicit SSH URL); `mainrepo` points at this same local checkout, used for worktree cross-referencing.

When running `/sync-remotes`: `FORK_REMOTE=origin`, `UPSTREAM_REMOTE=upstream-fanatics`.

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
3. Update protobuf definitions in `proto/session/v1/` if needed → `make proto-gen`
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
1. Define RPC in `proto/session/v1/session.proto` → `make proto-gen`
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

Tests in `tests/e2e/` use Playwright + Allure. **Do not manually start a server first** — `tests/e2e/global-setup.ts` does this automatically for every run, fully isolated from any already-running instance:

- Spawns `stapler-squad --test-mode --test-dir <dir> --tmux-keep-server` with `PORT=<dynamically-assigned free port>` (`findFreePort()` in `tests/e2e/helpers/test-server.ts`) — never a fixed port, so it can never collide with the live dev instance on `:8543`.
- `--test-dir` (backed by `STAPLER_SQUAD_TEST_DIR`) points at a PID-scoped temp directory (`/tmp/stapler-squad-test-<pid>` by default, or `TEST_SERVER_DIR` to override) — completely separate from `~/.stapler-squad/`, so it shares no session/backlog state with the deployed service.
- Playwright's `baseURL` is set dynamically to that port via `TEST_SERVER_URL` (see `playwright.config.ts`); the hardcoded `:8544` fallback there only applies if you set `TEST_SERVER_URL` yourself to point at a server you started by hand.
- Global teardown (`global-teardown.ts`) kills that isolated server and cleans up its temp dir when the run ends.

```bash
cd tests/e2e && npm test                                 # runs the full suite (isolated instance auto-managed)
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

### Manual/interactive testing without touching the live deployed instance

Backlog items and other automation depend on the systemd-managed instance at `:8543` staying up — **never use `make install-service` to try out an in-progress change** (it restarts that live service, killing its tmux server and every session/backlog work in flight; see the WARNING above and `.claude/rules/tmux-keep-server-on-restart.md`). To click around a change by hand instead, run a second, fully separate instance:

```bash
go build -o /tmp/ssq-manual-test .
PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &
# ...test in a browser at http://localhost:8999...
kill %1   # stop it when done
```

- Build to a distinct output path (not `./stapler-squad`) — that path is the live systemd unit's `ExecStart` binary; overwriting it in place is confusing even though a running process keeps its old inode open.
- `PORT` must differ from `:8543` (and from any other manual/e2e instance you already have running) or the bind will fail.
- `STAPLER_SQUAD_INSTANCE=<name>` gives it its own state dir under `~/.stapler-squad/instances/<name>/` (see `.claude/docs/state-isolation.md`) — it will not see or affect the live deployed instance's sessions, backlog items, or config.
- `--tmux-keep-server` still applies here: without it, stopping this manual instance kills its tmux server too (fine for a throwaway instance, but keep the flag if you want to leave sessions running between restarts of it).

---

## Reference Documents Index

| Topic | File |
|---|---|
| Profiling / lock-up debugging | `.claude/docs/profiling.md` |
| OpenTelemetry / Datadog setup | `.claude/docs/opentelemetry.md` |
| macOS code signing / TCC | `.claude/docs/codesigning.md` |
| PTY multiplexing (ssq-mux) | `.claude/docs/pty-multiplexing.md` |
| State file isolation / multi-instance | `.claude/docs/state-isolation.md` |
| Tag-based session organization | `.claude/docs/tag-organization.md` |
| Benchmark reference | `.claude/docs/benchmarks.md` |
| Nil safety & static analysis tools | `.claude/docs/nil-safety.md` |
| Go concurrency patterns | `.claude/docs/concurrency-patterns.md` |
| Bundling tmux (single-binary) | `.claude/docs/bundling-tmux.md` |
| CSS architecture (vanilla-extract) | `.claude/rules/css-architecture.md` |
| Feature registry rules | `.claude/rules/feature-registry.md` |
| Omnibar feature testing registry | `.claude/rules/feature-testing-registry.md` |
| Session creation mode registry (7 touchpoints) | `.claude/rules/session-creation-registry.md` |
| systemd user service (restart, logs, D-Bus issues) | `.claude/rules/systemd-user-service.md` |
| ent ORM schema generation (`--feature sql/upsert`) | `.claude/rules/ent-schema-generation.md` |
| Go double-checked locking pattern | `.claude/rules/go-double-checked-locking.md` |
| Interface pollution checklist (leaky abstractions in LLM-generated Go) | `.claude/rules/interface-pollution-checklist.md` |
| E2E test conventions (annotation, locators, no waitForTimeout) | `.claude/rules/e2e-test-conventions.md` |
| Commit SDD planning artifacts before ending a session | `.claude/rules/sdd-planning-artifacts-commit.md` |
| Prefer go-git over shelling out to git CLI | `.claude/rules/prefer-go-git-over-subshells.md` |
| Service restart kills every live tmux session without `--tmux-keep-server` | `.claude/rules/tmux-keep-server-on-restart.md` |
| macOS restart can leave orphaned processes racing over tmux/session state | `.claude/rules/service-restart-orphan-process.md` |
| Fix flaky tests when found, don't just re-defer as "known pre-existing" | `.claude/rules/fix-flaky-tests-dont-defer.md` |
