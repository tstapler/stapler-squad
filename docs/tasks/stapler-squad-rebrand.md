# Stapler Squad Rebrand Plan

Hard fork rebrand of `stapler-squad` to `stapler-squad` / `Stapler Squad`.

## Critical Distinction

References to `claude` as the **Anthropic AI tool** (e.g., `ProgramClaude = "claude"`, `claude-mux`, `claudesession`, `claudemetadata`, `which claude`) MUST NOT be renamed. Only rename references to the **project name** `stapler-squad` / `Stapler Squad`.

---

## Phase 1: Core Identity Files

These files define the project identity and must be changed first since everything else depends on them.

### Task 1.1: Go Module Path

**File:** `go.mod`
- Change `module stapler-squad` to `module github.com/tstapler/stapler-squad`
- This is the most impactful change: every Go file that imports internal packages uses this module name.
- All import paths change from `"stapler-squad/..."` to `"github.com/tstapler/stapler-squad/..."`

### Task 1.2: buf.gen.yaml (Protobuf Code Generation Config)

**File:** `buf.gen.yaml`
- Change `stapler-squad/gen/proto/go` to `stapler-squad/gen/proto/go` in the `go_package_prefix` override.

### Task 1.3: GoReleaser Config

**File:** `.goreleaser.yaml`
- Change `binary: stapler-squad` to `binary: stapler-squad`.

### Task 1.4: Makefile

**File:** `Makefile`
- Line 1 comment: `Stapler Squad Makefile` -> `Stapler Squad Makefile`
- Line 13: `"Stapler Squad Development Makefile"` -> `"Stapler Squad Development Makefile"`
- Line 19: `go build -o stapler-squad .` -> `go build -o stapler-squad .`
- Lines 41-42: `pkill -f "^\./stapler-squad"` -> `pkill -f "^\./stapler-squad"`
- Lines 44-45: `./stapler-squad` -> `./stapler-squad`
- Line 60: `stapler-squad-trace-*.out` -> `stapler-squad-trace-*.out`
- Line 61: `stapler-squad-trace-*.out` -> `stapler-squad-trace-*.out`
- Lines 65-68: Same `./stapler-squad` -> `./stapler-squad` pattern
- Line 141: `nilaway -include-pkgs="stapler-squad"` -> `nilaway -include-pkgs="stapler-squad"`
- Line 154: Same nilaway include-pkgs change
- Line 183: `rm -f stapler-squad` -> `rm -f stapler-squad`

### Task 1.5: .gitignore

**File:** `.gitignore`
- Line 4: `stapler-squad` -> `stapler-squad`
- Line 8: `stapler-squad-test` -> `stapler-squad-test`

### Task 1.6: GitHub Actions Workflows

**File:** `.github/workflows/build.yml`
- `BINARY_NAME=stapler-squad` -> `BINARY_NAME=stapler-squad`
- `name: stapler-squad-*` -> `name: stapler-squad-*`

**File:** `.github/workflows/cla.yml`
- Update all `smtg-ai/stapler-squad` GitHub URLs to the new fork URL (e.g., `tylerstapler/stapler-squad` or whatever the target org/repo is).
- `remote-repository-name: 'stapler-squad-clas'` -> update to new repo name.

---

## Phase 2: Go Source Code (Import Paths)

Every `.go` file that has `import "stapler-squad/..."` must be updated to `import "stapler-squad/..."`. This is a mechanical find-and-replace across **197 Go files**.

### Task 2.1: Bulk Import Path Replacement

**Scope:** All `.go` files in the repository.
**Operation:** Replace all occurrences of `"stapler-squad/` with `"github.com/tstapler/stapler-squad/` in import statements.

This affects files across every package:
- `main.go` (8 imports)
- `config/` (2 files)
- `cmd/` (12 files)
- `server/` (25+ files)
- `session/` (60+ files)
- `daemon/` (1 file)
- `log/` (1 file)
- `telemetry/` (2 files)
- `profiling/` (1 file)
- `terminal/` (2 files)
- `testutil/` (5 files)
- `github/` (2 files)
- `session/ent/` (36 generated files -- see Task 2.3)

### Task 2.2: Go Source String Literals

Beyond import paths, several Go files contain `"stapler-squad"` in string literals (NOT import paths). These must also be renamed.

| File | String | New Value |
|------|--------|-----------|
| `telemetry/telemetry.go:22` | `ServiceName = "stapler-squad"` | `ServiceName = "stapler-squad"` |
| `server/server.go:204` | `"stapler-squad-web"` | `"stapler-squad-web"` |
| `server/server.go:215` | `"stapler-squad-http"` | `"stapler-squad-http"` |
| `server/server.go:287` | `"stapler-squad-remote"` | `"stapler-squad-remote"` |
| `profiling/profiling.go:57` | `"/tmp/stapler-squad-trace-%d.out"` | `"/tmp/stapler-squad-trace-%d.out"` |
| `log/log.go:276` | `".stapler-squad"` | `".stapler-squad"` |
| `log/log.go:291` | Comment: `~/.stapler-squad/logs/` | `~/.stapler-squad/logs/` |
| `log/log.go:576` | `"stapler-squad-test"` | `"stapler-squad-test"` |
| `config/config.go:127` | `".stapler-squad"` | `".stapler-squad"` |
| `config/config.go:191` | Comment: `~/.stapler-squad/logs` | `~/.stapler-squad/logs` |
| `session/repo_path.go:16,29,131` | `".stapler-squad"` paths | `".stapler-squad"` paths |
| `session/scrollback/manager.go:16` | Comment: `~/.stapler-squad/sessions` | `~/.stapler-squad/sessions` |
| `session/ent_repository.go:36` | `"~/.stapler-squad/sessions.db"` | `"~/.stapler-squad/sessions.db"` |
| `session/claude_controller.go:678` | `"/.stapler-squad"` | `"/.stapler-squad"` |
| `github/clone.go:12,31` | `"~/.stapler-squad/repos"` | `"~/.stapler-squad/repos"` |
| `server/dependencies.go:433` | `".stapler-squad", "sessions"` | `".stapler-squad", "sessions"` |
| `server/services/session_service.go:410` | Comment: `~/.stapler-squad/repos/` | `~/.stapler-squad/repos/` |
| `cmd/migrate_global.go:22` | `".stapler-squad"` | `".stapler-squad"` |
| `session/instance.go:2120` | Comment: `~/.stapler-squad/worktrees/` | `~/.stapler-squad/worktrees/` |
| `main.go:58` | `"STAPLER_SQUAD_TEST_DIR"` env var | See Phase 6 |
| `main.go:310` | `"smtg-ai/stapler-squad"` URL | Update to fork URL |
| `main.go:494` | Comment: `/tmp/stapler-squad-trace-` | `/tmp/stapler-squad-trace-` |

### Task 2.3: Generated Ent Code

The `session/ent/` directory contains **36 files** generated by the `ent` ORM framework. Every file has `import "stapler-squad/..."` paths.

**Strategy:** After updating `go.mod` (Task 1.1), regenerate ent code:
```bash
go generate ./session/ent/...
```

If regeneration is not feasible (e.g., ent version mismatch), do a bulk find-and-replace of `"stapler-squad/` with `"stapler-squad/` in all `session/ent/*.go` files. The `session/ent/schema/` files do NOT contain project name references.

### Task 2.4: Generated Protobuf Go Code

The `gen/proto/go/` directory contains **4 generated files** with `stapler-squad` references in:
- Import paths (e.g., `"stapler-squad/gen/proto/go/session/v1"`)
- Embedded proto descriptor bytes (binary strings containing `stapler-squad`)

**Strategy:** After updating `buf.gen.yaml` (Task 1.2) and proto files (Task 5.1), regenerate:
```bash
make proto-gen
```

### Task 2.5: Tmux Prefix Constant

**File:** `session/tmux/tmux.go:97`
- `const TmuxPrefix = "claudesquad_"` -> `const TmuxPrefix = "staplersquad_"`

**WARNING -- BREAKING CHANGE:** This changes how tmux sessions are named on disk. Existing running sessions with the `claudesquad_` prefix will be invisible to the renamed application. This requires a migration strategy (see Known Issues section).

Related function names to rename:
- `ToClaudeSquadTmuxName` -> `ToStaplerSquadTmuxName`
- `toClaudeSquadTmuxName` -> `toStaplerSquadTmuxName`
- `toClaudeSquadTmuxNameWithPrefix` -> `toStaplerSquadTmuxNameWithPrefix`
- `ListClaudeSquadSessions` -> `ListStaplerSquadSessions`
- `ListClaudeSquadSessionsWithInfo` -> `ListStaplerSquadSessionsWithInfo`

Files affected by function renames:
- `session/tmux/tmux.go` (definitions + usages)
- `session/mux/multiplexer.go` (calls `ListClaudeSquadSessions`)
- `session/mux/picker.go` (calls `ListClaudeSquadSessionsWithInfo`)
- `session/pty_discovery.go` (calls `ToClaudeSquadTmuxName`)

### Task 2.6: Environment Variables

**Files:** `config/config.go`, `main.go`

Rename all environment variables:
- `STAPLER_SQUAD_TEST_DIR` -> `STAPLER_SQUAD_TEST_DIR`
- `STAPLER_SQUAD_INSTANCE` -> `STAPLER_SQUAD_INSTANCE`
- `STAPLER_SQUAD_WORKSPACE_MODE` -> `STAPLER_SQUAD_WORKSPACE_MODE`

Also update `STAPLER_SQUAD_USE_CONTROL_MODE` references if they exist (check `server/services/connectrpc_websocket.go`).

### Task 2.7: Test String Assertions

**File:** `config/config_test.go`
- Update all `.stapler-squad` path assertions to `.stapler-squad` (lines 284-455, approximately 10 occurrences).

**File:** `log/log_test.go`
- Update `.stapler-squad` path assertions (line 57-58).

**Files:** Various `*_test.go` files
- Update any test helpers or assertions that reference `stapler-squad` by name.

---

## Phase 3: Frontend (React/TypeScript)

### Task 3.1: web-app Page Titles and Metadata

| File | Change |
|------|--------|
| `web-app/src/app/layout.tsx:11` | `"Stapler Squad Sessions"` -> `"Stapler Squad Sessions"` |
| `web-app/src/app/login/page.tsx:69` | `"Stapler Squad"` -> `"Stapler Squad"` |
| `web-app/src/app/login/layout.tsx:4-5` | `"Sign In - Stapler Squad"` etc. -> `"Sign In - Stapler Squad"` |
| `web-app/src/app/config/layout.tsx:4-5` | `"Configuration - Stapler Squad"` -> `"Configuration - Stapler Squad"` |
| `web-app/src/app/review-queue/layout.tsx:4-5` | `"Review Queue - Stapler Squad"` -> `"Review Queue - Stapler Squad"` |
| `web-app/src/app/history/layout.tsx:4-5` | `"History - Stapler Squad"` -> `"History - Stapler Squad"` |

### Task 3.2: web-app Header and Navigation

| File | Change |
|------|--------|
| `web-app/src/components/layout/Header.tsx:43` | `"Stapler Squad"` -> `"Stapler Squad"` |
| `web-app/src/components/ui/Navigation.tsx:20-21` | `"Stapler Squad home"` / `"Stapler Squad"` -> `"Stapler Squad home"` / `"Stapler Squad"` |

### Task 3.3: web-app localStorage Keys

**File:** `web-app/src/components/sessions/SessionList.tsx` (lines 28-35)

All localStorage keys use `stapler-squad-*` prefix. Rename to `stapler-squad-*`:
- `stapler-squad-search-query` -> `stapler-squad-search-query`
- `stapler-squad-selected-status` -> `stapler-squad-selected-status`
- `stapler-squad-selected-category` -> `stapler-squad-selected-category`
- `stapler-squad-selected-tag` -> `stapler-squad-selected-tag`
- `stapler-squad-hide-paused` -> `stapler-squad-hide-paused`
- `stapler-squad-grouping-strategy` -> `stapler-squad-grouping-strategy`
- `stapler-squad-sort-field` -> `stapler-squad-sort-field`
- `stapler-squad-sort-dir` -> `stapler-squad-sort-dir`

**Other localStorage keys:**
- `web-app/src/lib/hooks/useSearchHistory.ts:5` -- `'stapler-squad-logs-search-history'`
- `web-app/src/lib/config/terminalConfig.ts:133` -- `"stapler-squad-terminal-config"`
- `web-app/src/lib/utils/notificationStorage.ts:22` -- `"stapler-squad-notifications"`
- `web-app/src/components/history/HistorySearchInput.tsx:44` -- `"stapler-squad-history-search"`

### Task 3.4: web-app URL Parser Tests

**File:** `web-app/src/lib/github/urlParser.test.ts`

Lines 146-157 contain test cases using `stapler-squad` as test data for GitHub URL parsing. These are test fixture data -- rename them:
- `'stapler-squad repo'` test case name
- `'https://github.com/anthropics/stapler-squad'` test URL
- `wantRepo: 'stapler-squad'` expected result
- PR test case similarly

### Task 3.5: web-app Terminal README

**File:** `web-app/src/lib/terminal/README.md:3`
- `"stapler-squad web UI"` -> `"stapler-squad web UI"`

### Task 3.6: Generated TypeScript (Protobuf)

**Files:** `web-app/src/gen/session/v1/types_pb.ts`, `web-app/src/gen/session/v1/session_connect.ts`

These contain `stapler-squad` references in generated comments. They will be updated automatically when protos are regenerated (Task 5.1 + `make proto-gen`).

### Task 3.7: web/ (Old Frontend -- REMOVE)

**Decision:** The `web/` directory is a legacy frontend superseded by `web-app/`. **Delete it entirely.**

```bash
git rm -r web/
```

No updates needed — just remove.

### Task 3.8: E2E Tests

**File:** `tests/e2e/package.json`
- `"name": "stapler-squad-e2e-tests"` -> `"stapler-squad-e2e-tests"`
- `"description": "...stapler-squad..."` -> `"...stapler-squad..."`

**File:** `tests/e2e/package-lock.json` -- regenerate after package.json change.

**File:** `tests/e2e/helpers/test-server.ts`
- `/tmp/stapler-squad-test-${pid}` -> `/tmp/stapler-squad-test-${pid}`
- `'../../../stapler-squad'` build path -> `'../../../stapler-squad'`
- `'go build -o stapler-squad .'` -> `'go build -o stapler-squad .'`

**File:** `tests/e2e/smoke.spec.ts`
- `toHaveTitle(/Stapler Squad/)` -> `toHaveTitle(/Stapler Squad/)`

**File:** `tests/test-results.json` -- this is a snapshot artifact. Either delete it or update the paths. Probably best to delete and regenerate.

---

## Phase 4: Shell Scripts

### Task 4.1: install.sh

**File:** `install.sh`
- `smtg-ai/stapler-squad` API URL -> new fork URL
- `stapler-squad${extension}` binary name -> `stapler-squad${extension}`
- `stapler-squad_${VERSION}` archive name -> `stapler-squad_${VERSION}`

### Task 4.2: clean.sh and clean_hard.sh

**File:** `clean.sh`
- `rm -rf ~/.stapler-squad` -> `rm -rf ~/.stapler-squad`

**File:** `clean_hard.sh`
- `rm -rf ~/.stapler-squad` -> `rm -rf ~/.stapler-squad`

### Task 4.3: scripts/install-mux.sh

**File:** `scripts/install-mux.sh`
- Comment references to `stapler-squad` as project name. Update to `stapler-squad`.
- DO NOT rename `claude-mux` binary -- that refers to the Claude AI tool.

### Task 4.4: scripts/ssq-hook-handler, ssq-hooks-install, ssq-notify

**Decision:** Rename `cs-` prefix to `ssq-` (avoids `ss-` which has Nazi SS connotation).

Rename files:
- `scripts/ssq-hook-handler` -> `scripts/ssq-hook-handler`
- `scripts/ssq-hooks-install` -> `scripts/ssq-hooks-install`
- `scripts/ssq-notify` -> `scripts/ssq-notify`

Update comments in renamed files:
- `ssq-hook-handler:3` -- `"Stapler Squad notifications"` -> `"Stapler Squad notifications"`
- `ssq-notify:3` -- `"Send notifications from tmux sessions to Stapler Squad"` -> `"...Stapler Squad"`
- `ssq-hooks-install:3` -- `"Install Claude Code hooks for Stapler Squad notifications"` -> `"...Stapler Squad notifications"`

Also update any references to these script names in `docs/claude-hooks-integration.md` and CLAUDE.md.

---

## Phase 5: Protobuf Definitions

### Task 5.1: Proto Source Files

**File:** `proto/session/v1/session.proto`
- `~/.stapler-squad/logs/` in comment -> `~/.stapler-squad/logs/`

**File:** `proto/session/v1/types.proto`
- `"managed by stapler-squad"` -> `"managed by stapler-squad"` (4 occurrences in comments)

After changes, regenerate all code:
```bash
make proto-gen
```

This will update:
- `gen/proto/go/session/v1/*.go`
- `web-app/src/gen/session/v1/*.ts`

---

## Phase 6: Configuration and Data Directory

### Task 6.1: Data Directory Path

The application stores data in `~/.stapler-squad/`. This path is hardcoded in:
- `config/config.go:127`
- `log/log.go:276`
- `session/repo_path.go:29`
- `session/claude_controller.go:678`
- `session/ent_repository.go:36`
- `github/clone.go:12`
- `server/dependencies.go:433`
- `cmd/migrate_global.go:22`

All should change to `~/.stapler-squad/`.

**WARNING -- BREAKING CHANGE:** Existing users will have data in `~/.stapler-squad/`. Requires migration (see Known Issues).

### Task 6.2: Environment Variables

Environment variables with `STAPLER_SQUAD_` prefix (used in `config/config.go` and `main.go`):
- `STAPLER_SQUAD_TEST_DIR` -> `STAPLER_SQUAD_TEST_DIR`
- `STAPLER_SQUAD_INSTANCE` -> `STAPLER_SQUAD_INSTANCE`
- `STAPLER_SQUAD_WORKSPACE_MODE` -> `STAPLER_SQUAD_WORKSPACE_MODE`
- `STAPLER_SQUAD_USE_CONTROL_MODE` -> `STAPLER_SQUAD_USE_CONTROL_MODE`

### Task 6.3: Tmux Session Prefix

**File:** `session/tmux/tmux.go:97`
- `claudesquad_` -> `staplersquad_`

This is a runtime-visible identifier. See Known Issues for migration concerns.

---

## Phase 7: Documentation

### Task 7.1: CLAUDE.md (Project Instructions)

**File:** `CLAUDE.md`
- Approximately 38 occurrences of `stapler-squad` and 10 of `Stapler Squad`.
- Rename project references throughout. Preserve references to `claude` the AI tool.
- Update paths: `~/.stapler-squad/` -> `~/.stapler-squad/`
- Update env vars: `STAPLER_SQUAD_*` -> `STAPLER_SQUAD_*`
- Update binary name: `./stapler-squad` -> `./stapler-squad`
- Update trace paths: `/tmp/stapler-squad-trace-` -> `/tmp/stapler-squad-trace-`

### Task 7.2: README.md

**File:** `README.md`
- Update all project name references (21 occurrences of `stapler-squad`).
- Update `smtg-ai/stapler-squad` GitHub URLs to new fork URL.
- Update installation instructions (binary name, brew formula if applicable).

### Task 7.3: Other Top-Level Docs

| File | Action |
|------|--------|
| `CONTRIBUTING.md` | Update `smtg-ai/stapler-squad` URLs |
| `CLA.md` | Update `smtg-ai/stapler-squad` URLs |
| `TODO.md` | Update project name references |
| `TODO.md.bak` | Update or delete |
| `PERFORMANCE_OPTIMIZATIONS.md` | Update project name reference |
| `SQLITE_MIGRATION_STRATEGY.md` | Update `Stapler Squad` references |

### Task 7.4: docs/ Directory

Large number of documentation files reference `stapler-squad` or `Stapler Squad`. Update all `.md` files in:
- `docs/tasks/` (20+ files)
- `docs/archive/` (15+ files)
- `docs/bugs/` (6+ files)
- `docs/architecture/` (3+ files)
- `docs/upstream/` (6 files -- these reference the upstream project and may need special handling)
- `docs/tui-test-project/` (4+ files)
- Root docs files (15+ files)

**Special case -- `docs/upstream/`:** These files analyze the upstream `stapler-squad` project and its forks. References to the upstream project should remain as `stapler-squad` since they refer to the original project. Only references to "this project" or "our fork" should be updated.

### Task 7.5: cmd/README.md and log/README.md

**File:** `cmd/README.md` -- Update project name references.
**File:** `log/README.md` -- Update `~/.stapler-squad/` paths and project name.

### Task 7.6: tuitest/README.md

**File:** `tuitest/README.md`
- Update `stapler-squad` / `ClaudeSquad` references.

---

## Phase 8: Submodule and Auxiliary Go Modules

### Task 8.1: tuitest/go.mod

**File:** `tuitest/go.mod`
- `module github.com/stapler-squad/tuitest` -> decide on new module path (e.g., `github.com/tylerstapler/stapler-squad/tuitest` or keep as-is if it is an upstream dependency).

### Task 8.2: tuitest/Makefile

**File:** `tuitest/Makefile`
- Update any `stapler-squad` / `ClaudeSquad` references.

---

## Phase 9: Upstream References (smtg-ai GitHub URLs)

All references to `github.com/smtg-ai/stapler-squad` need to be updated to the fork's new URL. Affected files:

| File | Context |
|------|---------|
| `main.go:310` | Release URL in version check |
| `install.sh:62,299-300` | API and download URLs |
| `web/src/app/page.tsx` | Multiple URLs |
| `web/src/app/layout.tsx:30` | Metadata URL |
| `.github/workflows/cla.yml:26,30,36` | CLA references |
| `README.md` | Multiple URLs |
| `CONTRIBUTING.md` | Contributor URLs |
| `CLA.md` | CLA text |
| `docs/upstream/` | Upstream analysis docs (may keep as historical reference) |

**New GitHub URL:** `github.com/tstapler/stapler-squad`

---

## Phase 10: Verification and Regeneration

### Task 10.1: Regenerate All Generated Code

After all source changes:
```bash
# Regenerate protobuf code
make proto-gen

# Regenerate ent ORM code
go generate ./session/ent/...

# Rebuild everything
make build
```

### Task 10.2: Run Full Test Suite

```bash
go test ./...
```

### Task 10.3: Verify No Remaining References

```bash
# Should return zero results (excluding docs/upstream/ if keeping historical references)
grep -r "stapler-squad" --include="*.go" --include="*.ts" --include="*.tsx" --include="*.proto" --include="*.yaml" --include="*.yml" --include="*.json" --include="*.sh" .

# Check for remaining ClaudeSquad function names (excluding claude-the-tool references)
grep -r "ClaudeSquad" --include="*.go" .

# Check for remaining environment variables
grep -r "CLAUDE_SQUAD" --include="*.go" .
```

### Task 10.4: Update package-lock.json Files

```bash
cd web-app && npm install
cd tests/e2e && npm install
cd web && npm install  # If keeping old frontend
```

### Task 10.5: Build and Smoke Test

```bash
make build
./stapler-squad --help
make restart-web
# Verify http://localhost:8543 shows "Stapler Squad" in title
```

---

## Execution Order

The tasks MUST be executed in this dependency order:

1. **Phase 1** (Core Identity) -- Everything else depends on these.
2. **Phase 2** (Go Imports + Strings) -- Bulk mechanical replacement.
3. **Phase 5** (Proto Sources) -- Before regeneration.
4. **Phase 6** (Config/Data paths) -- Overlaps with Phase 2 but called out separately for clarity.
5. **Phase 10.1** (Regenerate) -- `make proto-gen` + `go generate` after source changes.
6. **Phase 3** (Frontend) -- Independent of Go changes.
7. **Phase 4** (Shell Scripts) -- Independent.
8. **Phase 7** (Documentation) -- Independent, can be done last.
9. **Phase 8** (Submodule) -- Small scope.
10. **Phase 9** (URLs) -- Requires decision on new GitHub URL.
11. **Phase 10.2-10.5** (Verification) -- Final validation.

---

## File Count Summary

| Category | Files Affected | Complexity |
|----------|---------------|------------|
| Go source (imports only) | ~197 | Mechanical (sed/find-replace) |
| Go source (string literals) | ~25 | Manual review needed |
| Go generated (ent) | ~36 | Regenerate after go.mod change |
| Go generated (proto) | 4 | Regenerate after proto changes |
| TypeScript/React (web-app) | ~12 | Manual |
| TypeScript generated (proto) | 2 | Regenerate |
| TypeScript/React (web/ old) | ~5 | Manual or skip |
| E2E tests | ~5 | Manual |
| Shell scripts | ~6 | Manual |
| Proto definitions | 2 | Manual |
| Config files (yaml, json, mod) | ~8 | Manual |
| Documentation (md) | ~80+ | Bulk replace with manual review |
| **Total** | **~380+ files** | |

---

## Known Issues

### Issue 1: Data Directory Migration (SEVERITY: High)

**Description:** Changing `~/.stapler-squad/` to `~/.stapler-squad/` means existing session data, configuration, logs, worktrees, and SQLite databases will be orphaned in the old directory.

**Resolution:** Implement automatic migration on first startup.

**Implementation:**
- On startup, if `~/.stapler-squad` does not exist but `~/.stapler-squad` does, automatically migrate (move/copy) the directory.
- Log a message: `"Migrating data directory from ~/.stapler-squad to ~/.stapler-squad"`
- Best location: `config/config.go` during config initialization, or `main.go` early startup.

**Files Likely Affected:** `config/config.go`, `main.go`

### Issue 2: Tmux Session Prefix Change (SEVERITY: High)

**Description:** Changing `TmuxPrefix` from `claudesquad_` to `staplersquad_` means any currently running tmux sessions will become invisible to the application. The session discovery logic searches by prefix.

**Resolution:** Implement dual-prefix discovery — find sessions with either `claudesquad_` or `staplersquad_` prefix.

**Implementation:**
- Update session listing/discovery to search for both prefixes.
- Log a notice when `claudesquad_` sessions are found: `"Found legacy session with old prefix, consider restarting it"`
- New sessions are always created with `staplersquad_` prefix.

**Files Likely Affected:** `session/tmux/tmux.go`, `session/mux/multiplexer.go`, `session/mux/picker.go`

### Issue 3: Environment Variable Backward Compatibility (SEVERITY: Medium)

**Description:** Renaming `STAPLER_SQUAD_*` environment variables breaks existing user configurations, CI scripts, and shell profiles.

**Mitigation:**
- Check both old and new environment variable names, with new name taking priority.
- Log a deprecation warning when old variable names are detected.

**Files Likely Affected:** `config/config.go`, `main.go`

**Prevention Strategy:**
- Implement fallback logic: `os.Getenv("STAPLER_SQUAD_X")` || `os.Getenv("STAPLER_SQUAD_X")`
- Add deprecation warnings to logs.

### Issue 4: localStorage Key Migration (SEVERITY: Low)

**Description:** Renaming localStorage keys in the web UI means users lose their saved preferences (search queries, filter settings, sort preferences).

**Mitigation:**
- Add a one-time migration in the frontend that copies old keys to new keys on page load.
- Or keep old keys and only write new keys going forward.

**Files Likely Affected:** `web-app/src/components/sessions/SessionList.tsx`, various hooks/utils.

**Prevention Strategy:**
- Add a `migrateLocalStorage()` function called once on app init.

### Issue 5: Generated Code Drift (SEVERITY: Medium)

**Description:** If `ent` or `buf` tool versions differ from what generated the current code, regeneration may produce unexpected diffs beyond the rename.

**Mitigation:**
- Pin tool versions before regeneration.
- Review generated code diffs carefully -- only accept the rename changes.
- If regeneration causes problems, fall back to mechanical find-and-replace on generated files.

**Prevention Strategy:**
- Record current tool versions before starting.
- Use `git diff` to verify only rename-related changes in generated code.

### Issue 6: Upstream Merge Conflicts (SEVERITY: Medium)

**Description:** After the rebrand, pulling changes from the upstream `smtg-ai/stapler-squad` repository will create massive merge conflicts since every file has been modified.

**Mitigation:**
- Use `git merge -X theirs` or cherry-pick individual commits from upstream.
- Maintain a script that re-applies the rename after merging upstream changes.
- Consider using `sed`-based rename scripts that can be re-run after each upstream merge.

**Prevention Strategy:**
- Create a `scripts/rebrand.sh` that automates the rename. After merging upstream, re-run the script.
- Document the upstream merge process.

### Issue 7: Binary Name in User PATH (SEVERITY: Low)

**Description:** Users who have `stapler-squad` in their PATH (via `go install`, brew, or manual install) will need to update their references.

**Mitigation:**
- Document in release notes.
- Optionally provide a `stapler-squad` -> `stapler-squad` symlink during transition.

---

## Decisions Made

All decisions confirmed by project owner. No further input needed before execution.

| # | Decision | Resolution |
|---|----------|------------|
| 1 | New GitHub URL | `github.com/tstapler/stapler-squad` |
| 2 | Go module path | `github.com/tstapler/stapler-squad` (fully qualified) |
| 3 | Old `web/` frontend | **Remove entirely** — `web-app/` is the canonical frontend |
| 4 | Script prefix (`cs-`) | Rename to **`ssq-`** (not `ss-` — avoids SS/Nazi connotation) |
| 5 | Data directory migration | **Implement automatic migration** — detect `~/.stapler-squad/`, migrate to `~/.stapler-squad/` on first startup |
| 6 | Tmux prefix migration | **Dual-prefix support** — discover both `claudesquad_` and `staplersquad_` sessions during transition |
| 7 | Upstream sync strategy | **Clean fork, no upstream sync** — complete separation from `smtg-ai/stapler-squad` |

### Critical Naming Rule

> **`claude` (the Anthropic AI tool) is NEVER renamed.**
> Only `stapler-squad` / `Stapler Squad` (the project name) is renamed.
>
> Examples of what to preserve unchanged:
> - `ProgramClaude = "claude"` — the AI tool name
> - `claude-mux` — the multiplexer binary that wraps Claude
> - `claudesession`, `claudemetadata` — tmux session/metadata names for Claude the tool
> - `which claude` — checking for the Claude binary
> - Any user-facing references to "Claude" as the AI model
