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

See `docs/how-to/macos-codesigning.md` for first-time setup and cert backup instructions.

STAPLER_SQUAD_USE_CONTROL_MODE=false ./stapler-squad   # Disable tmux control mode (legacy polling)
./stapler-squad --tmux-keep-server                     # Keep tmux server alive after sessions close
```

**WARNING:** `make install-service` restarts the running service, which kills the tmux server and every live tmux session with it — including any session you're currently working in — unless the deployed unit passes `--tmux-keep-server`. See `docs/explanation/tmux-keep-server-on-restart.md`.

### Profiling

```bash
./stapler-squad --profile --trace
```

See `docs/how-to/profile-lockups.md` for full pprof/goroutine dump instructions.
OpenTelemetry (Datadog/OTLP) setup: `docs/how-to/enable-opentelemetry.md`

### Bundling tmux

Single-binary deployment with embedded tmux: `docs/how-to/bundle-tmux.md`

### Testing

```bash
make build && make test     # Build (generates protos) then test
make quick-check            # Build + test + lint (fast validation)
make ci                     # Full CI pipeline (definitive pre-push check)

go test ./server/services -timeout=20m   # Specific packages (requires make build first)
go test ./ui -run TestFoo   # Specific test
make test-coverage

# Frontend tests (not part of make ci)
cd web-app && npx jest --no-coverage
cd web-app && npx jest --testPathPatterns="<pattern>" --no-coverage
```

Benchmark reference (all benchmarks MUST be run with `&`): `docs/reference/benchmarks.md`

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

Nil safety and static analysis tool reference: `docs/how-to/run-nil-safety-analysis.md`

### Duplication and hotspot checks — required before pushing

`make ready` (superset of `make ci`) runs two **blocking** duplication gates —
both fail the build, not just report:

- **Go — `dupl`** (`.golangci.yml`'s settings block, `.github/workflows/lint.yml`):
  new-code-only via `--new-from-rev=origin/main`, mirroring the existing
  gocyclo/gocognit/funlen/revive complexity gate — only duplication introduced
  by your own diff fails; the repo's unmeasured pre-existing pool is never in
  scope. Run locally with `make ready-complexity-gate` (needs `dupl` from
  `make install-tools`). Fix what it finds in your diff before pushing —
  usually an Extract/Move Function refactor (see `quality:reflect-and-fix`'s
  Level 0 consolidation gate), not a suppression.
- **web-app — `jscpd`** (`web-app/.jscpd.json`; `make ready-duplication-gate-web`
  or `pnpm run lint:duplicates` in `web-app/`): jscpd has no git-diff scoping
  like `--new-from-rev`, so this gates on an absolute `threshold` (0.1%) instead
  of new-code-only — a ratchet against today's cleaned-up baseline (~0.09%,
  ~215 duplicated lines), not zero-tolerance. `minLines`/`minTokens` are tuned
  to 20/200: verified empirically (2026-08-24 repo-wide sweep + fix) that at
  that size every finding was real, actionable duplication — component forks,
  copy-pasted hooks/effects, shared style/test-fixture blocks — not
  boilerplate noise, which dominates at jscpd's noisy 5-line/50-token
  defaults. The ~0.09% baseline that remains is `jest.mock(...)` registration
  lines that can't be extracted into a shared function (babel-jest hoists them
  per test file), so it's irreducible, not unfixed debt. Long-term, true
  diff-aware cross-file duplicate detection belongs in `kibitzer`
  (github.com/tstapler/kibitzer) — see `tstapler/kibitzer#28`.

For "which files are actually risky to touch" (complexity × git-churn, not
literal duplication), see the `code-hotspot-analysis` skill and
`tstapler/kibitzer#15` (not yet implemented in kibitzer).

### Go Skills — Always Invoke for Go Work

When writing, reviewing, or refactoring Go code, invoke the relevant skill(s):

| Task | Skill |
|---|---|
| General idioms, error handling, interfaces, naming, project structure | `/go-development` |
| Concurrency primitive selection (mutex vs atomic vs channel vs lock-free) | `/go-concurrency` |
| pprof profiling — CPU, memory, goroutine, mutex profiles | `/go-profiling` |
| Fix a specific pprof hotspot (atomic shadow, RWMutex, TTL cache, etc.) | `/go:optimize` |

Invoke proactively — do not wait to be asked. If a task involves any `.go` file, load the appropriate skill before starting.

Subtle patterns (double-checked locking, etc.): `docs/explanation/concurrency-patterns.md`

## Application Data

State and logs live in `~/.stapler-squad/`:
- `logs/staplersquad.log` — main log (JSON-lines, one `slog` record per line); check here for session creation issues. `logs/service.log` is a different file — raw systemd stdout/stderr (startup banners, panics before logging init) — see `docs/how-to/debug-with-logs.md` for the full file breakdown, log-level controls, and volume-reduction guidance. With `STAPLER_SQUAD_INSTANCE=<name>` set, an instance logs to `instances/<name>/logs/staplersquad.log` instead (unset or `shared` uses the path above, unchanged)
- `worktrees/` — git worktrees for isolated sessions
- `config.json`, `sessions.json`

**Key log patterns:** `Starting tmux session`, `timed out waiting for tmux session`, `DoesSessionExist()` polling

State isolation (workspace-based by default): `docs/reference/state-isolation.md`
External session monitoring (ssq-mux for IDE terminals): `docs/how-to/monitor-external-terminal-sessions.md`

## Architecture Overview

Go web server on `localhost:8543` + React SPA. Manages AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees.

| Layer | Path | Purpose |
|---|---|---|
| Web Server | `server/` | HTTP + ConnectRPC handlers, middleware |
| Session Mgmt | `session/` | Lifecycle, storage, tmux, git worktrees, scrollback |
| Config | `config/` | JSON config, state persistence |
| Web UI | `web-app/` | React SPA, real-time terminal via ConnectRPC |

Sessions support tag-based multi-dimensional organization with 8 grouping strategies (Category, Tag, Branch, Path, Program, Status, Session Type, None). Full reference: `docs/reference/tag-organization.md`

## Git Remotes

| Remote | Repo | Role |
|---|---|---|
| `origin` | `tstapler/stapler-squad` | Canonical (upstream-fanatics deprecated, development stopped there) |

`tstapler-ssh` is a duplicate of `origin` (same repo, explicit SSH URL); `mainrepo` points at this same local checkout, used for worktree cross-referencing.

## Pull Request Requirements

Use [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Effect |
|---|---|
| `fix:` | Patch bump |
| `feat:` | Minor bump |
| `feat!:` / `BREAKING CHANGE:` footer | Major bump |
| `chore:`, `docs:`, `refactor:`, `test:` | No bump (hidden from changelog) |

Releases are not automatic — release-please opens a "Release PR"; merge when ready to ship.

**`gh pr merge` always needs `--repo owner/repo`** — this repo's worktrees make `gh` misresolve `main` otherwise. See `docs/how-to/merge-prs-with-gh-cli.md`.

**PRs in this repo default to ready for review, not draft.** This overrides the global "Draft PRs by default" instruction specifically for `tstapler/stapler-squad` — open with `gh pr create` (no `--draft`) unless the user asks for a draft.

## Adding New Features

### New Web UI Features
1. Create React components in `web-app/src/components/`
2. Add ConnectRPC endpoints in `server/services/`
3. Update protobuf definitions in `proto/session/v1/` if needed → `make proto-gen`
4. Test with `make install-service`

### New Omnibar Capabilities
Two registries must stay in sync — see `docs/reference/feature-testing-registry.md`:
- **OmnibarAction union** (`types.ts` + `dispatch.ts` + `dispatch.test.ts`) for user-triggerable actions
- **DetectorRegistry** (`detector.ts` + `detector.test.ts`) for auto-detected input patterns
- New session creation modes also require 7 touchpoints — see `docs/reference/session-creation-registry.md`

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

Workflow: edit schema → run correct generate → `go build ./...` to confirm it compiles. **Do not commit the generated output** — `.gitignore` deliberately excludes `session/ent/*.go` and `session/ent/*/` (everything except `schema/` and `generate.go`, which are hand-written), the same policy as the `gen/`-prefixed proto output above. Every Make target that needs it (`build`, `test`, `lint`) already depends on `ent-gen`, which regenerates from a stamp file — commit only the `session/ent/schema/` change itself. Force-adding generated ent code (`git add -f`) has caused real breakage before (missing/incomplete package left main broken until someone ran `make ent-gen` and noticed) — don't do it even to unblock a build.

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

Backlog items and other automation depend on the systemd-managed instance at `:8543` staying up — **never use `make install-service` to try out an in-progress change** (it restarts that live service, killing its tmux server and every session/backlog work in flight; see the WARNING above and `docs/explanation/tmux-keep-server-on-restart.md`). To click around a change by hand instead, run a second, fully separate instance:

```bash
mkdir -p ~/.stapler-squad/manual-builds/manual-1
go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server &
# ...test in a browser at http://localhost:62871...
# for --remote-access, also pass: --remote-port 62872   (its default, 8444, collides with the live instance)
kill %1   # stop it when done
```

- Build to `~/.stapler-squad/manual-builds/manual-<N>/stapler-squad` — never `./stapler-squad` (the live launchd/systemd unit's `ExecStart` binary; overwriting it in place is confusing even though a running process keeps its old inode open) and never a bare `/tmp/ssq-manual-test` path (no per-instance separation, so a second concurrent manual build silently overwrites the first instance's running binary, and `/tmp` can be cleared by the OS between reboots, unlike `~/.stapler-squad/`). Number the directory to match the port-block instance (`manual-1` ↔ `62871`/`62872`, `manual-2` ↔ `62873`/`62874`) so the binary path and the port it's bound to stay obviously paired.
- Use ports from the **manual dev port block** below — `PORT` must differ from `:8543` (and `--remote-port` from `:8444`) or the bind will fail.
- `STAPLER_SQUAD_INSTANCE=<name>` gives it its own state dir under `~/.stapler-squad/instances/<name>/` (see `docs/reference/state-isolation.md`) — it will not see or affect the live deployed instance's sessions, backlog items, or config. This is separate from the build directory above: `instances/<name>/` holds runtime state (sessions, config, worktrees), `manual-builds/manual-<N>/` holds the binary.
- `--tmux-keep-server` still applies here: without it, stopping this manual instance kills its tmux server too (fine for a throwaway instance, but keep the flag if you want to leave sessions running between restarts of it).

#### Manual dev port block

Per `local-dev-port-management`'s Sequential Batch Strategy: a fixed block reserved for this project's manual/interactive dev instances, so ad hoc `PORT=8999`-style choices stop colliding with each other and with the live instance's fixed ports (`:8543` main, `:8444` remote-access — those two are user-facing and documented elsewhere, so they stay put). Base `62871` = `61000 + CRC32("stapler-squad") % 4525`, run `make ports` to recompute/display.

| Port | Use |
|---|---|
| 62871 | Manual instance #1 — `PORT` |
| 62872 | Manual instance #1 — `--remote-port` |
| 62873 | Manual instance #2 — `PORT` (when a second concurrent instance is needed, e.g. comparing before/after) |
| 62874 | Manual instance #2 — `--remote-port` |
| 62875–62880 | Spare |

`tests/e2e/` doesn't use this block — it allocates a free ephemeral port per run via `findFreePort()` (`tests/e2e/helpers/test-server.ts`), which needs no fixed reservation.

---

## Documentation Placement

General project documentation goes under `docs/` in a Diataxis-style hierarchy
(`docs/how-to/`, `docs/reference/`, `docs/explanation/` — see `docs/README.md`
for the full layout), never under `.claude/docs/`. That directory no longer
exists — it was migrated wholesale in 2026-08.

AI-authorship code-review checklists and guardrails (the kind of thing that
used to live in `.claude/rules/*.md`) become project skills under
`.claude/skills/<slug>/SKILL.md` instead, never a new `.claude/rules/` file —
that directory no longer exists either. The reason is context cost, not
organization: a `.claude/rules/*.md` file's entire content loads into context
every time something references it (including this file's own references),
while a skill's full body only loads when actually invoked via the Skill
tool — the skill list itself shows just a one-line description the rest of
the time. See the `interface-pollution-checklist`, `primitive-obsession-checklist`,
`e2e-test-conventions`, `prefer-go-git-over-subshells`, and
`fix-flaky-tests-dont-defer` skills for the converted examples.

## Reference Documents Index

| Topic | File |
|---|---|
| Profiling / lock-up debugging | `docs/how-to/profile-lockups.md` |
| OpenTelemetry / Datadog setup | `docs/how-to/enable-opentelemetry.md` |
| Compile-time auto-instrumentation (opt-in `stapler-squad-otel` build) | `docs/how-to/enable-otel-auto-instrumentation.md` |
| Enabling pi coding-agent support (flag, extension install, health badge) | `docs/how-to/enable-pi-support.md` |
| macOS code signing / TCC | `docs/how-to/macos-codesigning.md` |
| PTY multiplexing (ssq-mux) | `docs/how-to/monitor-external-terminal-sessions.md` |
| State file isolation / multi-instance | `docs/reference/state-isolation.md` |
| Tag-based session organization | `docs/reference/tag-organization.md` |
| Benchmark reference | `docs/reference/benchmarks.md` |
| Nil safety & static analysis tools | `docs/how-to/run-nil-safety-analysis.md` |
| Go concurrency patterns | `docs/explanation/concurrency-patterns.md` |
| Bundling tmux (single-binary) | `docs/how-to/bundle-tmux.md` |
| Bundling tymuxd (single-binary, supervised) | `docs/reference/bundling-tymuxd.md` |
| CSS architecture (vanilla-extract) | `docs/reference/css-architecture.md` |
| Feature registry rules | `docs/reference/feature-registry.md` |
| Omnibar feature testing registry | `docs/reference/feature-testing-registry.md` |
| Session creation mode registry (7 touchpoints) | `docs/reference/session-creation-registry.md` |
| systemd user service (restart, logs, D-Bus issues) | `docs/how-to/manage-systemd-service.md` |
| Interface pollution checklist (leaky abstractions in LLM-generated Go) | `interface-pollution-checklist` skill |
| Primitive obsession checklist (same-typed parameter piles in LLM-generated Go) | `primitive-obsession-checklist` skill |
| E2E test conventions (annotation, locators, no waitForTimeout) | `e2e-test-conventions` skill |
| Commit SDD planning artifacts before ending a session | `docs/how-to/commit-sdd-planning-artifacts.md` |
| Prefer go-git over shelling out to git CLI | `prefer-go-git-over-subshells` skill |
| Service restart kills every live tmux session without `--tmux-keep-server` | `docs/explanation/tmux-keep-server-on-restart.md` |
| Package manager: always pnpm in web-app/, never npm/yarn | `docs/how-to/use-pnpm-in-web-app.md` |
| macOS restart can leave orphaned processes racing over tmux/session state | `docs/explanation/service-restart-orphan-process.md` |
| Fix flaky tests when found, don't just re-defer as "known pre-existing" | `fix-flaky-tests-dont-defer` skill |
| Slack Phase 2 interactive-approvals public reachability (scoping a tunnel to one path) | `docs/how-to/expose-slack-interactive-endpoint.md` |
| GitHub webhook (`/webhooks/github`, incl. PR-fix events) public reachability | `docs/how-to/expose-github-webhook-endpoint.md` |
| Log debugging: file locations, global/per-package log levels, reducing log volume, pattern-clustering tool | `docs/how-to/debug-with-logs.md` |
| `gh pr merge` needs `--repo owner/repo` | `docs/how-to/merge-prs-with-gh-cli.md` |
| Playwright Chromium install hangs during extraction | `docs/how-to/fix-playwright-chromium-install-stall.md` |
