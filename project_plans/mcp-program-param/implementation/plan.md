# Implementation Plan: mcp-program-param

**Feature**: Let `create_session`/`create_session_for_pr`'s MCP `program` parameter accept any registered program (built-in or custom), not just `claude`/`aider`, while keeping the agent able to discover valid values.
**Date**: 2026-09-24
**Status**: Ready for implementation
**ADRs**: [ADR-001: Dynamic program enum via a shared helper, plus an additive `list_programs` tool](../decisions/ADR-001-dynamic-program-enum-plus-list-programs-tool.md)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| Program ID | The string identifier for a program a session can launch (e.g. `claude`, `aider`, or a custom `claude-250k-proxy`). Matches `config.ProgramConfig.ID` / `sessionv1.ProgramConfigProto.Id`. | Stays a plain `string` — no new Go type. The proto/backend already treat it as free text (`config.ResolveProgramConfig`); introducing a newtype at the MCP schema layer alone would create a type boundary that doesn't exist anywhere else in the flow, for no behavioral gain. |
| Built-in Program | One of the 7 programs returned by `services.BuiltInPrograms()`: `claude`, `pi`, `aider`, `opencode`, `gemini`, `agy`, `bash`. | Not user-editable; `IsBuiltInProgram` prevents a custom program from reusing one of these IDs. |
| Custom Program | A `config.ProgramConfig` a user adds via `UpsertProgramConfig` (`server/services/defaults_service.go`), persisted in `cfg.SessionDefaults.Programs`. | Surfaced in the "Program Configurations" UI (`ProgramsManager.tsx`). |
| `programSchemaOptions` | The new shared helper function (`server/mcp/tools_program_common.go`) both `registerLifecycleTools` and `registerGitHubTools` call to build the `program` property's `mcpgo.PropertyOption`s. | Single source of truth for the enum + description text; see ADR-001. |
| `defaultProgramID` | New `const` (`"claude"`) replacing the two independent `"claude"` literal fallbacks in `tools_lifecycle.go:151` and `tools_github.go:217`. | Consistency cleanup riding along with the schema fix — same underlying value, now named once. |
| Registration-time snapshot | The known limitation that `programSchemaOptions`' enum is computed once, when `NewCore` runs at server startup, and does not update if a custom program is added afterward without a restart. | Documented in the `program` property's description text and mitigated by `list_programs` (Phase 2). |
| `list_programs` | New, additive MCP tool (Phase 2) that calls `svc.ListProgramsConfig` live, on every invocation, for authoritative (non-stale) program discovery. | Not required by any AC as written — see ADR-001 for why it's included anyway. |
| `ProgramWarning` | New optional field on `CreateSessionResult`/`CreateSessionForPRResult`, set (non-fatally) when an incoming `program` isn't found in `programIDs(svc)`'s registration-time snapshot. | A warning, not a rejection — a custom program added after server start must still work. See ADR-001's addendum and Story 1.2.1/1.3.1. Surfaced in two places, not just one: the JSON field itself, and a sentence in `create_session`/`create_session_for_pr`'s own tool-level description (Task 1.2.1a/1.3.1a) telling the calling agent to check it — a field it was never told about is invisible next to `success: true` (pre-mortem.md failure mode #2). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `program` schema construction (both tools) | Config-driven enum populated at registration time, via one shared helper | Precedent: `enumNamesWithoutPrefix`/`notificationTypeNames` (`server/mcp/tools_notifications.go:27-49`) | Static hand-maintained `mcpgo.Enum("claude", "aider")` (status quo) | Already stale — 5 of 7 built-ins are unreachable today, independent of any custom program (`research/features.md`). |
| `program` schema construction (both tools) | (as above) | — | Free-form string, no enum at all (backlog Option 1) | Satisfies AC1–3 but weakens AC4: no schema-level hint, and `research/pitfalls.md` flags typo failures as silent (surface only in tmux output, not as a tool-call error). |
| Enum/description logic shared across `tools_lifecycle.go` and `tools_github.go` | Extracted pure function (`programSchemaOptions`) — a single source of truth, not a GoF/PoEAA pattern per se, just Extract Function applied deliberately | `research/architecture.md`'s explicit recommendation | Duplicate the `ListProgramsConfig`-calling + `mcpgo.Enum`-building logic independently in each file | Recreates the exact two-file-drift bug this backlog item exists to fix — the same shape as `tools_backlog.go:598-608`'s hand-synced `eventTypeFilter` counter-example. |
| Program discovery for agents (AC4) | Hybrid: dynamic enum (startup-time convenience list, honestly labeled as such) **plus** a live `list_programs` tool | Backlog description's Option 2 + `research/features.md`'s "clearest concrete gap for AC4" recommendation | Dynamic enum alone, with no staleness caveat and no live fallback | Leaves the one documented weakness of a startup-time snapshot (`research/pitfalls.md`) with no mitigation and no way for the agent to learn about it. |
| `list_programs` handler | Direct passthrough to `svc.ListProgramsConfig` (Transaction Script — no new domain logic) | `research/build-vs-buy.md`: "no OSS/SaaS alternative … nothing to buy" | A caching/memoizing layer in front of `ListProgramsConfig` | Unnecessary — `research/stack.md` confirms the call is cheap, synchronous, side-effect-free; caching would reintroduce the exact staleness problem this tool exists to solve. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/mcp/tools_lifecycle.go` + `server/mcp/tools_github.go` `program` enum duplication | Two hand-duplicated `mcpgo.Enum("claude", "aider")` literals, no shared helper file exists in `server/mcp/` today (`research/architecture.md` §1) | **Refactor-first**: introduce `server/mcp/tools_program_common.go`, eliminating the duplication outright (both call sites use the one shared helper — no legacy mess is left wrapped behind an adapter, so this isn't "Isolate via seam") | Matches `research/architecture.md`'s explicit recommendation; the smallest change that eliminates the duplication and prevents recreating the drift bug. Architecture research found no broader hotspot in `server/mcp/` warranting escalation to a full package rewrite — this is scoped to the one recurring shape (enum-building). |
| `tools_backlog.go:601-609`'s `eventType...` const block vs. its `mcpgo.Enum(...)` call at `tools_backlog.go:3038` | **Confirmed already broken, not just a theoretical drift risk** (architecture-review finding): the const block defines all 7 `eventType*` values, but the `mcpgo.Enum(...)` call for `wait_for_backlog_event`'s `event_type` parameter only lists 5 of them — `eventTypeItemUpdated` and `eventTypeSessionAttached` are silently unreachable through the declared schema, live in production today. Same bug class this feature fixes for `program`, in the file this plan already cites as precedent. | **Extend as-is (out of scope, not touched)** | Not in this feature's blast radius (AC6 scopes this item to the `program` parameter only); fixing it now would be unrelated scope creep. This is not left as a passive note: a separate backlog item should be filed for `tools_backlog.go:3038`'s missing `eventTypeItemUpdated`/`eventTypeSessionAttached` enum values — not filed as part of this plan, but the gap and its exact location are recorded here so it isn't lost. |

---

## Migration Plan

N/A — no proto, database, or on-disk config schema changes. `CreateSessionRequest.program` stays a free-form `string`; `config.ProgramConfig`/`cfg.SessionDefaults.Programs` are unchanged (AC6).

## Observability Plan

- **Metrics/Alerts**: N/A — this is a client-facing MCP schema change with no new runtime failure mode; backend validation and its existing error paths (`mapCreateSessionRPCError`) are unchanged.
- **Logs**: None added. `programIDs()` (Task 1.1.1a) calls `svc.ListProgramsConfig`, which today has no non-nil-error return path (`server/services/defaults_service.go:684-703` — `config.LoadConfig()` doesn't return an error; the two loops can't fail). Logging a call that structurally cannot fail would be dead code. If `ListProgramsConfig` ever gains a fallible path, add a `log.Warn` there at that time — not preemptively here.

## Risk Control

- **Feature flag**: N/A — this is a strictly additive/corrective schema change (widens what was wrongly rejected; doesn't remove or gate any existing capability). No staged rollout is meaningful for a JSON-schema hint that was never server-enforced (`research/architecture.md` §4).
- **Rollback procedure**: Standard `git revert` of the merge commit — no data or config migration to unwind.
- **Staged rollout**: N/A — same reasoning as feature flag above.

## Unresolved Questions

None. (Whitespace/trim handling for `program` values is a pre-existing gap at every layer, not this fix's job to close, per `research/features.md` — intentionally left alone, not "unresolved.")

## Dependency Visualization

```
Phase 1 (core fix — AC6 scope)
┌─────────────────────────────────────────────────────────────┐
│ Epic 1.1: Shared helper                                     │
│   Task 1.1.1a: tools_program_common.go (programSchemaOptions)│
│         │                                                    │
│   Task 1.1.1b: tools_program_common_test.go                 │
└─────────────────────┬─────────────────────────┬─────────────┘
                       │                         │
                       ▼                         ▼
   Epic 1.2: create_session wiring     Epic 1.3: create_session_for_pr wiring
     Task 1.2.1a: tools_lifecycle.go     Task 1.3.1a: tools_github.go
             │                                   │
             ▼                                   ▼
   Epic 1.4: Test coverage (AC5)
     Task 1.4.1a/b: create_session tests   Task 1.4.2a: create_session_for_pr tests
             │                                   │
             └─────────────────┬─────────────────┘
                                ▼
              Phase 1 complete — AC1–AC6 satisfied
                                │
                                ▼  (optional, additive — not required for any AC)
Phase 2 (additive — beyond AC6's named files, see ADR-001)
┌─────────────────────────────────────────────────────────────┐
│ Epic 2.1: list_programs tool                                │
│   Task 2.1.1a: tools_programs.go (handler)                  │
│   Task 2.1.1b: server.go (NewCore registration)              │
│   Task 2.1.1c: types.go (ProgramSummary/ListProgramsResult)  │
│   Task 2.1.1d: tools_programs_test.go                        │
└─────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Core Fix — Dynamic Program Enum (AC1–AC6, in AC6's named scope)

### Epic 1.1: Shared Program-Enum Helper

**Goal**: One function, called from both tool-registration sites, that builds the `program` property's schema options from live `ListProgramsConfig` data — eliminating the duplication `research/architecture.md` flags as the drift risk.

#### Story 1.1.1: Introduce `programSchemaOptions`
**As a** developer maintaining `create_session`/`create_session_for_pr`, **I want** one function that builds the `program` property's enum + description, **so that** the two tools can never drift out of sync the way the current two hand-copied literals already have.

**Acceptance Criteria**:
- The helper returns an enum containing all 7 built-ins plus any registered custom program, and is nil-safe when `svc` is nil.
  - *Given* a `*services.SessionService` `svc` whose `ListProgramsConfig` reflects the 7 built-ins and one custom `ProgramConfig{ID: "claude-250k-proxy"}`, *When* `programSchemaOptions(svc)` is called, *Then* the returned `[]mcpgo.PropertyOption`, applied to a property schema map, produces `schema["enum"]` containing `"claude"`, `"pi"`, `"aider"`, `"opencode"`, `"gemini"`, `"agy"`, `"bash"`, and `"claude-250k-proxy"`.
  - *Given* `svc == nil` (the `githubHandlers.svc`-may-be-nil case documented in `research/stack.md`), *When* `programSchemaOptions(nil)` is called, *Then* it returns options containing only a `Description(...)` (no `Enum(...)`), and applying them to a property schema map does not panic and does not set `schema["enum"]`.
**Files**: `server/mcp/tools_program_common.go` (new), `server/mcp/tools_program_common_test.go` (new), `server/mcp/tools_lifecycle_worktree_guard_test.go` (add config isolation to `newWorktreeGuardHandlers`)

##### Task 1.1.1a: Create the shared helper file (~5 min)
- Create `server/mcp/tools_program_common.go` with:
  - `const defaultProgramID = "claude"`
  - `const programDescription = "Program to run (default: claude). Accepts any registered program ID. Common built-ins: claude, aider, pi, opencode, gemini, agy, bash. Custom programs added via the Program Configurations UI are also accepted. The enum below reflects programs known at server startup and may not include a program added since — check the Program Configurations UI, or ask whether a list_programs tool is available in this deployment, for the live, authoritative list."` (**Pre-mortem P1 fix**: does not assert `list_programs` exists — see Phase 2 note.)
  - `func programIDs(svc *services.SessionService) []string` — returns `nil` if `svc == nil`; otherwise calls `svc.ListProgramsConfig(context.Background(), connect.NewRequest(&sessionv1.ListProgramsConfigRequest{}))`, collects `resp.Msg.Programs[i].Id`, `sort.Strings`s them, and returns. Ignore a non-nil `err` by returning `nil` (matches the "no enum is safe, not fatal" contract already implicit in the current hardcoded-enum behavior).
  - `func programSchemaOptions(svc *services.SessionService) []mcpgo.PropertyOption` — starts with `[]mcpgo.PropertyOption{mcpgo.Description(programDescription)}`; if `programIDs(svc)` is non-empty, appends `mcpgo.Enum(ids...)`.
  - Imports: `context`, `sort`, `connectrpc.com/connect`, `mcpgo "github.com/mark3labs/mcp-go/mcp"`, `sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"`, `"github.com/tstapler/stapler-squad/server/services"`.
- Files: `server/mcp/tools_program_common.go`

##### Task 1.1.1b: Unit-test the helper, and fix a pre-existing test-isolation gap it would otherwise inherit (~8 min)
- **Config isolation fix (resolves architecture-review BLOCKER: new tests writing to the real `~/.stapler-squad/config.json`)**: `newWorktreeGuardHandlers` (`server/mcp/tools_lifecycle_worktree_guard_test.go:44-57`) constructs `services.NewSessionService(storage, bus)` with no test-isolation wrapper; `UpsertProgramConfig`/`ListProgramsConfig` resolve through `config.LoadConfig()`/`SaveConfig()` to the real `$HOME/.stapler-squad` unless `STAPLER_SQUAD_TEST_DIR` is set. `server/services/defaults_service_test.go:24-26`'s `newIsolatedDefaultsService(t)` already solves exactly this with `envtest.NewIsolatedStateDir(t)` (`envtest/envtest.go:71-76`: `t.TempDir()` + `t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)`). As part of this task, add `envtest.NewIsolatedStateDir(t)` as the first line of `newWorktreeGuardHandlers`'s body, before `services.NewSessionService(...)` is constructed. This is safe for its two existing callers (`server/mcp/tools_lifecycle_worktree_guard_test.go:101,123`) — it only redirects config resolution to a fresh temp dir, no behavior change — and it is the single point of isolation every other task in this plan that reuses `newWorktreeGuardHandlers` (1.4.1a, 1.4.1b, 1.4.2a) inherits automatically. `t.Setenv` forbids `t.Parallel()` on any test that (transitively) calls this helper — none of this plan's new tests use `t.Parallel()`, consistent with the existing convention at `tools_github_test.go:228`.
- Create `server/mcp/tools_program_common_test.go` with:
  - `TestProgramSchemaOptions_should_IncludeAllBuiltinsAndCustom_When_SvcHasCustomProgram` — build a real `*services.SessionService` the same way `newWorktreeGuardHandlers` (`server/mcp/tools_lifecycle_worktree_guard_test.go:44`, now isolated per the fix above) does, call `svc.UpsertProgramConfig` with a custom `ProgramConfigProto{Id: "claude-250k-proxy", ...}`, then call `programSchemaOptions(svc)`, apply the returned options to a `map[string]any{}`, and assert `schema["enum"].([]string)` contains `"bash"`, `"opencode"`, and `"claude-250k-proxy"`.
  - `TestProgramSchemaOptions_should_OmitEnum_When_SvcIsNil` — call `programSchemaOptions(nil)`, apply to a schema map, assert `schema["enum"]` is absent (`_, ok := schema["enum"]; !ok`) and `schema["description"]` is non-empty.
- Files: `server/mcp/tools_program_common_test.go`, `server/mcp/tools_lifecycle_worktree_guard_test.go` (add the isolation call to `newWorktreeGuardHandlers`)

### Epic 1.2: Wire `create_session` to the Shared Helper

**Goal**: `create_session`'s `program` property stops using a hardcoded 2-value enum.

#### Story 1.2.1: Replace the hardcoded enum and literal default
**As an** agent calling `create_session`, **I want** `program` to accept any registered program ID, **so that** I can launch a session with a built-in like `bash` or a custom program without the request being rejected before it reaches the backend.

**Acceptance Criteria** (AC1, AC3, AC4):
- A registered custom program is accepted.
  - *Given* `lifecycleHandlers{store, svc}` where `svc.ListProgramsConfig` includes `ProgramConfig{ID: "claude-250k-proxy"}` (added via `svc.UpsertProgramConfig`), *When* `create_session` is called with `title="t1"`, `path=<tempdir>`, `program="claude-250k-proxy"`, *Then* the call reaches `svc.CreateSession` with `CreateSessionRequest.Program == "claude-250k-proxy"` and the tool result's `success == true`.
- `claude`/`aider` and the omitted-default case are unchanged.
  - *Given* the same handler, *When* `create_session` is called with `program` omitted entirely, *Then* `CreateSessionRequest.Program == "claude"` (via the new `defaultProgramID` constant, same value as before).
- The registered schema documents valid values.
  - *Given* `registerLifecycleTools(s, lh)` has run, *When* `s.GetTool("create_session").Tool.InputSchema.Properties["program"]` is inspected, *Then* it has a non-empty `"description"` and, when `lh.svc` is non-nil, an `"enum"` containing all 7 built-ins.
- An unrecognized `program` value is not rejected, but the agent is warned it wasn't in the known-at-startup list (soft check — resolves architecture-review CONCERN: typo/unrecognized-program failure mode was unmitigated).
  - *Given* `lifecycleHandlers{store, svc}` with `svc` non-nil, *When* `create_session` is called with `program="clade"` (a typo, not present in `programIDs(svc)`), *Then* the tool result still has `success == true` (a custom program registered after server start must still work — this is a warning, not a rejection) but `CreateSessionResult.ProgramWarning` is non-empty and names `"clade"` and points at `list_programs`/the known list.
  - *Given* the same handler, *When* `create_session` is called with `program="bash"` (recognized, just previously unreachable via the old hardcoded enum), *Then* `CreateSessionResult.ProgramWarning` is empty.
**Files**: `server/mcp/tools_lifecycle.go`

##### Task 1.2.1a: Replace the enum and default literal, and warn (non-fatally) on an unrecognized program (~10 min)
- In `registerLifecycleTools` (`server/mcp/tools_lifecycle.go:59`), replace:
  ```go
  mcpgo.WithString("program", mcpgo.Description("Program to run: claude or aider (default: claude)"), mcpgo.Enum("claude", "aider")),
  ```
  with:
  ```go
  mcpgo.WithString("program", programSchemaOptions(lh.svc)...),
  ```
- In `createSessionWithAwaitTimeout` (`server/mcp/tools_lifecycle.go:150-151`), replace the literal fallback:
  ```go
  if program == "" {
      program = "claude"
  }
  ```
  with:
  ```go
  if program == "" {
      program = defaultProgramID
  }
  ```
- Immediately after that fallback, add a soft (non-fatal) check against the registration-time snapshot — this is deliberately not a hard rejection, because a custom program registered via `UpsertProgramConfig` *after* this process started must still be allowed to launch (the same registration-time-snapshot caveat `programDescription` already documents):
  ```go
  var programWarning string
  if lh.svc != nil {
      if known := programIDs(lh.svc); len(known) > 0 && !slices.Contains(known, program) {
          programWarning = fmt.Sprintf(
              "program %q was not in the list of programs known at server start (%s) — the session will still be created, but if this is a typo it will fail silently as shell output in the session's tmux pane. Check the Program Configurations UI, or ask whether list_programs is available in this deployment for a live list.",
              program, strings.Join(known, ", "))
      }
  }
  ```
  (`slices` joins the existing import list; `programIDs` already exists from Task 1.1.1a. **Pre-mortem P1 fix**: the warning text deliberately does NOT assert `list_programs` exists as a callable tool — Phase 2 is optional/additive and may not ship in the same release as this task. Naming a tool that might not exist yet would leave an agent following broken guidance. If Phase 2 ships in the same release, this text may be tightened back to name `list_programs` directly — see Phase 2's note below.)
- Add `ProgramWarning string \`json:"program_warning,omitempty"\`` to `CreateSessionResult` (`server/mcp/tools_lifecycle.go:41-50`), with a one-line doc comment: `// ProgramWarning is set (non-fatally) when program didn't match programSchemaOptions' registration-time snapshot — see createSessionWithAwaitTimeout's soft check.`
- Set `ProgramWarning: programWarning` on both `okResult(CreateSessionResult{...})` call sites in this function (the `StillCreating` early return and the final Active-session return) — not on error returns, since a warning is only meaningful alongside `success == true`.
- Append one sentence to `create_session`'s tool-level `mcpgo.WithDescription(...)` call (`server/mcp/tools_lifecycle.go:55`) — the description of the tool itself, not the `program` property's `programDescription` from Task 1.1.1a — so the calling agent knows to check the new field: `"If the result includes program_warning, the session was still created but the program value didn't match any program known at server start — check for a typo before assuming the session is running normally."` (resolves UX-review gap: pre-mortem.md's failure mode #2 — a `program_warning` field the agent was never told to look for is invisible next to `success: true`.)
- Files: `server/mcp/tools_lifecycle.go`

### Epic 1.3: Wire `create_session_for_pr` to the Shared Helper

**Goal**: Parity fix for `create_session_for_pr`, using the exact same helper (not a second hand-rolled copy).

#### Story 1.3.1: Replace the hardcoded enum and literal default
**As an** agent calling `create_session_for_pr`, **I want** the same program flexibility as `create_session`, **so that** launching a session for a PR review isn't restricted to `claude`/`aider` while direct session creation is not.

**Acceptance Criteria** (AC2, AC3, AC4):
- A registered custom program is accepted for parity with `create_session`.
  - *Given* `githubHandlers{cache, store, svc}` where `svc.ListProgramsConfig` includes `ProgramConfig{ID: "claude-250k-proxy"}`, *When* `create_session_for_pr` is called with `owner="tstapler"`, `repo="stapler-squad"`, `branch="feature/x"`, `pr_number=42`, `program="claude-250k-proxy"` (and no existing session already linked to that PR, so the short-circuit in `research/pitfalls.md`'s note doesn't trigger), *Then* the call reaches `svc.CreateSession` with `CreateSessionRequest.Program == "claude-250k-proxy"` and `success == true`.
- `claude`/`aider` and the omitted-default case are unchanged.
  - *Given* the same handler, *When* `program` is omitted, *Then* `CreateSessionRequest.Program == "claude"` (via `defaultProgramID`).
- An unrecognized `program` value is not rejected, but the agent is warned — parity with `create_session` (Story 1.2.1's same-shaped AC; resolves architecture-review CONCERN: typo/unrecognized-program failure mode was unmitigated).
  - *Given* `githubHandlers{cache, store, svc}` with `svc` non-nil and no existing session linked to the PR, *When* `create_session_for_pr` is called with `program="clade"` (a typo, not present in `programIDs(svc)`), *Then* the tool result still has `success == true` but `CreateSessionForPRResult.ProgramWarning` is non-empty and names `"clade"` and points at `list_programs`/the known list.
**Files**: `server/mcp/tools_github.go`

##### Task 1.3.1a: Replace the enum and default literal, and warn (non-fatally) on an unrecognized program (~10 min)
- In `registerGitHubTools` (`server/mcp/tools_github.go:101-103`), replace:
  ```go
  mcpgo.WithString("program",
      mcpgo.Description("Program to run: claude or aider (default: claude)"),
      mcpgo.Enum("claude", "aider"),
  ),
  ```
  with:
  ```go
  mcpgo.WithString("program", programSchemaOptions(gh.svc)...),
  ```
- In `createSessionForPR` (`server/mcp/tools_github.go:216-217`), replace the literal fallback:
  ```go
  if program == "" {
      program = "claude"
  }
  ```
  with:
  ```go
  if program == "" {
      program = defaultProgramID
  }
  ```
- Immediately after, add the same soft (non-fatal) check as Task 1.2.1a — not a hard rejection, for the same registration-time-snapshot reason:
  ```go
  var programWarning string
  if gh.svc != nil {
      if known := programIDs(gh.svc); len(known) > 0 && !slices.Contains(known, program) {
          programWarning = fmt.Sprintf(
              "program %q was not in the list of programs known at server start (%s) — the session will still be created, but if this is a typo it will fail silently as shell output in the session's tmux pane. Check the Program Configurations UI, or ask whether list_programs is available in this deployment for a live list.",
              program, strings.Join(known, ", "))
      }
  }
  ```
- Add `ProgramWarning string \`json:"program_warning,omitempty"\`` to `CreateSessionForPRResult` (`server/mcp/tools_github.go:58-66`), same doc comment as `CreateSessionResult.ProgramWarning`.
- Set `ProgramWarning: programWarning` on the `okResult(CreateSessionForPRResult{...})` call site(s) in `createSessionForPRWithAwaitTimeout` that represent a newly-created session (not the pre-existing-session short-circuit at `tools_github.go:186`, since that path never reaches the `program` argument at all).
- Append one sentence to `create_session_for_pr`'s tool-level `mcpgo.WithDescription(...)` call (`server/mcp/tools_github.go:78`) — the description of the tool itself, not the `program` property's `programDescription` — so the calling agent knows to check the new field: `"If the result includes program_warning, the session was still created but the program value didn't match any program known at server start — check for a typo before assuming the session is running normally."` (parity with Task 1.2.1a; same pre-mortem.md failure mode #2 fix.)
- Files: `server/mcp/tools_github.go`

### Epic 1.4: Test Coverage for the New Behavior (AC5)

**Goal**: Net-new tests proving a non-`claude`/`aider` program value succeeds through the MCP tool path — the "zero existing coverage" gap `research/features.md` identifies — plus confirmation existing suites still pass, including an explicit `claude`/`aider` regression check (AC3).

**Rate-limiter isolation (resolves adversarial-review BLOCKER: new tests colliding with the package-global `createSessionLimiter`)**: `createSessionLimiter` (`server/mcp/rate_limiter.go:61`, `var createSessionLimiter = newTokenBucket(3.0/60.0, 3)`) is a single package-level token bucket, capacity 3, refilling 1 token/~20s, keyed on the constant string `"global"` (`tools_lifecycle.go:123`, `tools_github.go:158`) — every call to `create_session`/`create_session_for_pr` anywhere in the `mcp` package's test binary draws from the same bucket, and one pre-existing test already draws one token (`TestCreateSessionForPR_should_ReturnExistingSession_When_PRAlreadyHasOne`, `tools_github_test.go:227`). This plan's new tests below add 5 more direct calls (3 in Task 1.4.1b — bash, custom program, explicit aider — and 2 in Task 1.4.2a — custom program, explicit aider) — enough to exhaust the bucket and hit `ErrRateLimitExceeded` on a fast in-memory test run, well inside the 20s refill window. Fix (no production-code change required): every test function below that calls `createSession`/`createSessionWithAwaitTimeout`/`createSessionForPR`/`createSessionForPRWithAwaitTimeout` must, as its first line, reset the package var to a fresh bucket with the same production parameters — `createSessionLimiter = newTokenBucket(3.0/60.0, 3)` — which is legal because these test files are `package mcp` (white-box), not `package mcp_test`. None of these new tests may use `t.Parallel()` (this also follows from the config-isolation fix's `t.Setenv` in Task 1.1.1b, and matches the existing convention noted at `tools_github_test.go:228`), so the reassignment can't race a concurrently-running sibling test. Introduce this as a small shared helper, `resetCreateSessionLimiterForTest(t *testing.T)`, in `tools_lifecycle_program_test.go` (Task 1.4.1a), and call it from every rate-limited test in both this file and `tools_github_test.go`.

#### Story 1.4.1: `create_session` accepts a previously-unreachable built-in and a custom program, and `claude`/`aider` still work
**As a** maintainer, **I want** a regression test pinning that `create_session` accepts `bash` and a custom program, and that it still accepts `claude`/`aider` unchanged, **so that** this bug class (hardcoded enum silently rejecting valid programs) can't reappear unnoticed, and the fix itself can't regress the two programs that already worked (AC3).

**Acceptance Criteria** (AC3, AC5):
- *Given* `lifecycleHandlers` wired to a real `*services.SessionService` (per `newWorktreeGuardHandlers`'s pattern) with no custom programs registered, *When* `create_session` is called with `program="bash"` (previously rejected by the hardcoded `Enum("claude", "aider")`), *Then* the resulting `CreateSessionRequest.Program == "bash"` and the call does not fail with a program-related error.
- *Given* the same setup with a custom program `claude-250k-proxy` registered via `UpsertProgramConfig`, *When* `create_session` is called with `program="claude-250k-proxy"`, *Then* it succeeds identically.
- *Given* the same setup, *When* `create_session` is called with `program="aider"` explicitly, *Then* `CreateSessionRequest.Program == "aider"` and the call succeeds — closing AC3's previously test-free claim that `claude`/`aider` are unaffected by this change.
- *Given* the same setup, *When* `create_session` is called with `program` omitted entirely, *Then* `CreateSessionRequest.Program == "claude"` and `ProgramWarning` is empty (validation.md gap #2).
- *Given* the same setup, *When* `create_session` is called with `program="clade"` (unrecognized), *Then* `success == true` and `ProgramWarning` is non-empty and names `"clade"` (validation.md gap #1 — closes the previously-unverified Story-level AC on `ProgramWarning`).
**Files**: `server/mcp/tools_lifecycle_program_test.go` (new — no pre-existing `tools_lifecycle_test.go` to extend, per repo scan)

##### Task 1.4.1a: Schema-level test + shared rate-limiter test helper (~7 min)
- In new file `server/mcp/tools_lifecycle_program_test.go`: `TestRegisterLifecycleTools_should_IncludeAllBuiltinsInProgramEnum_When_SvcIsWired` — build a real `mcpserver.NewMCPServer(...)`, call `registerLifecycleTools(s, lh)` with a wired `svc`, then `s.GetTool("create_session")`, assert `InputSchema.Properties["program"].(map[string]any)["enum"].([]string)` contains `"bash"` and `"opencode"` (the two clearest previously-unreachable built-ins per `research/features.md`). This test only inspects the registered schema — it never calls the handler, so it does not draw a rate-limiter token and needs no reset.
- In the same file, add `func resetCreateSessionLimiterForTest(t *testing.T) { t.Helper(); createSessionLimiter = newTokenBucket(3.0/60.0, 3) }` (see this Epic's "Rate-limiter isolation" note above) for Task 1.4.1b and Task 1.4.2a to call.
- **Validation-plan addition** (`validation.md` Findings §, infra self-test gap): add `TestResetCreateSessionLimiterForTest_should_RestoreCapacity_When_BucketExhausted` — drain the package-level `createSessionLimiter` (e.g. call `.Allow()` until it returns `false`), call `resetCreateSessionLimiterForTest(t)`, then assert `.Allow()` returns `true` again 3 times before `false` — proving the reset helper the other 5 rate-limited tests below depend on actually restores capacity, rather than trusting it implicitly. Pure unit test, no service/config I/O, no rate-limiter draw against the "real" bucket path (drains and resets its own copy).
- Files: `server/mcp/tools_lifecycle_program_test.go`

##### Task 1.4.1b: Functional tests — built-in, custom program, explicit aider, omitted-default, and unrecognized-program warning (AC3, AC4), end-to-end (~14 min)
- In the same file: `TestCreateSession_should_AcceptPreviouslyUnreachableBuiltin_When_ProgramIsBash`, `TestCreateSession_should_AcceptCustomProgram_When_RegisteredViaUpsertProgramConfig`, and `TestCreateSession_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` (AC3) — following `newWorktreeGuardHandlers`'s handler-construction pattern (real `services.NewSessionService`, config-isolated per Task 1.1.1b's fix), calling `resetCreateSessionLimiterForTest(t)` as the first line, then `lh.createSession`/`lh.createSessionWithAwaitTimeout` directly with `makeToolReq(map[string]interface{}{"title": ..., "path": t.TempDir(), "program": "bash"})` (`"claude-250k-proxy"` in the second test, registered first via `svc.UpsertProgramConfig`; `"aider"` in the third), asserting `parseResult(t, res)["success"] == true` and, for the third test, that `CreateSessionRequest.Program`/the persisted instance's program == `"aider"`.
- **Validation-plan additions** (`validation.md` Findings §1/§2 — the plan's Story 1.2.1 AC named these scenarios but no task exercised them):
  - Assert `parseResult(t, res)["program_warning"]` is empty/absent on the `program="bash"` test above (proves a recognized-but-previously-unreachable program produces no warning).
  - `TestCreateSession_should_SetProgramWarning_When_ProgramNotInKnownList` — call `lh.createSession` with `program="clade"` (a typo, not in `programIDs(lh.svc)`), assert `parseResult(t, res)["success"] == true` (warning, not rejection) AND `parseResult(t, res)["program_warning"]` is a non-empty string containing `"clade"`.
  - `TestCreateSession_should_DefaultToClaudeProgram_When_ProgramOmitted` — call `lh.createSession` with no `"program"` key in the request map at all, assert the resulting `CreateSessionRequest.Program == "claude"` (via `defaultProgramID`) and `parseResult(t, res)["program_warning"]` is empty (the default must never itself warn).
  - `TestCreateSession_should_SucceedWithNoWarning_When_SvcIsNilAndProgramIsATypo` (UX-review gap, P3 — pre-mortem.md failure mode #5: `programSchemaOptions`' dynamic enum and the `ProgramWarning` check are both gated on `svc != nil`, so a `svc == nil` construction — the documented fallback-transport/test case per `research/stack.md` — silently no-ops both agent-legibility mitigations at once; Task 1.1.1b only tests `programSchemaOptions(nil)`'s schema shape in isolation, not this compound behavior end-to-end through the handler) — construct `lifecycleHandlers{store, svc: nil}`, call `createSession`/`createSessionWithAwaitTimeout` with a typo'd `program` (e.g. `"clade"`), assert the call still succeeds (`parseResult(t, res)["success"] == true`, no client-side rejection) and `parseResult(t, res)["program_warning"]` is empty — asserting this explicitly as the documented, accepted degradation (no known-list to check against when `svc` is nil), not leaving it unverified.
- Files: `server/mcp/tools_lifecycle_program_test.go`

#### Story 1.4.2: `create_session_for_pr` accepts a custom program, for parity
**As a** maintainer, **I want** the same regression coverage on `create_session_for_pr`, **so that** the two tools can't silently drift back apart.

**Acceptance Criteria** (AC3, AC5):
- *Given* `githubHandlers` wired the same way as `TestCreateSessionForPR_should_ReturnExistingSession_When_PRAlreadyHasOne` (existing test at `server/mcp/tools_github_test.go:227`) but with `svc` non-nil and a custom program registered, and no existing session linked to the PR, *When* `create_session_for_pr` is called with `program="claude-250k-proxy"`, *Then* it succeeds and `CreateSessionRequest.Program == "claude-250k-proxy"`.
- *Given* the same setup, *When* `create_session_for_pr` is called with `program="aider"` explicitly, *Then* `CreateSessionRequest.Program == "aider"` and the call succeeds (AC3 parity with Story 1.4.1's aider test).
- *Given* the same setup, *When* `create_session_for_pr` is called with `program` omitted, *Then* `CreateSessionRequest.Program == "claude"` and `ProgramWarning` is empty (validation.md gap #2, parity).
- *Given* the same setup, *When* `create_session_for_pr` is called with `program="clade"` (unrecognized), *Then* `success == true` and `ProgramWarning` is non-empty and names `"clade"` (validation.md gap #1, parity).
**Files**: `server/mcp/tools_github_test.go`

##### Task 1.4.2a: Add the parity tests, rate-limiter-reset and config-isolated (~14 min)
- In `server/mcp/tools_github_test.go`, add `TestCreateSessionForPR_should_AcceptCustomProgram_When_RegisteredViaUpsertProgramConfig` and `TestCreateSessionForPR_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` (AC3), mirroring the existing `TestCreateSessionForPR_should_ReturnExistingSession_When_PRAlreadyHasOne` fixture setup but constructing `githubHandlers{cache, store, svc: <real *services.SessionService with the custom program upserted, config-isolated per Task 1.1.1b's fix>}` instead of `svc: nil`, and a PR with no existing session (empty `cache.Annotate` sessions) so the short-circuit path isn't hit. Both new tests must call `resetCreateSessionLimiterForTest(t)` (defined in `tools_lifecycle_program_test.go`, Task 1.4.1a — same package, no new helper needed) as their first line, and must not use `t.Parallel()`.
- **Validation-plan additions** (`validation.md` Findings §1/§2, parity with Task 1.4.1b):
  - `TestCreateSessionForPR_should_SetProgramWarning_When_ProgramNotInKnownList` — same shape as Task 1.4.1b's `create_session` version, `program="clade"`, no existing session on the PR, assert `success == true` and `program_warning` non-empty and mentions `"clade"`.
  - `TestCreateSessionForPR_should_DefaultToClaudeProgram_When_ProgramOmitted` — `program` omitted, assert `CreateSessionRequest.Program == "claude"` and `program_warning` empty.
- Files: `server/mcp/tools_github_test.go`

---

## Phase 2: Additive — Live Program Discovery (`list_programs`)

Not required by any acceptance criterion as written (the dynamic enum from Phase 1 already satisfies AC4's literal text). Included per ADR-001 to close the one documented weakness of a startup-time snapshot — a program added via `UpsertProgramConfig` after the server started is invisible to the enum until restart. **This phase touches files beyond AC6's named scope** (`server/mcp/tools_lifecycle.go`, `server/mcp/tools_github.go`, and the Phase 1 shared helper) — flagged here explicitly rather than folded silently into Phase 1.

**Pre-mortem P1 note (recommended, not blocking)**: ship this phase in the same PR/release as Phase 1 if feasible. Phase 1's `programDescription`/`programWarning` text deliberately hedges ("ask whether a list_programs tool is available") rather than asserting `list_programs` exists, specifically so a Phase-1-only release doesn't point agents at a nonexistent tool. If Phase 2 does ship alongside Phase 1, tighten both strings back to name `list_programs` directly as a follow-up micro-task (not written out here to avoid over-specifying a contingent edit).

### Epic 2.1: `list_programs` MCP Tool

**Goal**: A read-only tool giving an agent a live, authoritative list of every registered program, independent of what any tool's schema enum captured at startup.

#### Story 2.1.1: Add `list_programs`
**As an** agent that isn't sure whether a program ID is currently registered, **I want** a tool that tells me live, **so that** I'm not limited to whatever was true when the server last started.

**Acceptance Criteria**:
- *Given* a running MCP server with `svc.ListProgramsConfig` reflecting the 7 built-ins plus a custom program added after server startup, *When* `list_programs` is called, *Then* the result includes that custom program (proving it isn't relying on the Phase 1 registration-time enum snapshot).
**Files**: `server/mcp/tools_programs.go` (new), `server/mcp/server.go`, `server/mcp/types.go`, `server/mcp/tools_programs_test.go` (new)

##### Task 2.1.1a: Add the tool + handler (~5 min)
- Create `server/mcp/tools_programs.go`:
  - `type programHandlers struct { svc *services.SessionService }`
  - `func registerProgramTools(s *mcpserver.MCPServer, ph *programHandlers)` — registers `list_programs` with `mcpgo.WithDescription("List every program (built-in and custom) currently registered, live — unlike the enum shown on create_session/create_session_for_pr's program parameter, this always reflects the current config, including programs added since the server started.")`, no arguments.
  - `func (ph *programHandlers) listPrograms(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)` — nil-guards `ph.svc`, calls `ph.svc.ListProgramsConfig`, maps each `ProgramConfigProto` to a `ProgramSummary{ID, Label, Description, IsBuiltin}`, returns via `okResult`.
- Files: `server/mcp/tools_programs.go`

##### Task 2.1.1b: Register in `NewCore` (~2 min)
- In `server/mcp/server.go`, inside the existing `if svc != nil { ... }` block (around the `registerWorkflowTools`/`registerRulesTools` calls), add `registerProgramTools(s, &programHandlers{svc: svc})`.
- Files: `server/mcp/server.go`

##### Task 2.1.1c: Add result types (~2 min)
- In `server/mcp/types.go`, add:
  ```go
  // ProgramSummary is one entry in list_programs' result.
  type ProgramSummary struct {
      ID          string `json:"id"`
      Label       string `json:"label"`
      Description string `json:"description,omitempty"`
      IsBuiltin   bool   `json:"is_builtin"`
  }

  // ListProgramsResult is returned by list_programs.
  type ListProgramsResult struct {
      MCPResult
      Programs []ProgramSummary `json:"programs"`
  }
  ```
- Files: `server/mcp/types.go`

##### Task 2.1.1d: Tests (~5 min)
- Create `server/mcp/tools_programs_test.go`: `TestListPrograms_should_IncludeProgramAddedAfterRegistration_When_CalledLiveAtRuntime` — wire `programHandlers` to a real `*services.SessionService`, call `registerProgramTools`/`ph.listPrograms` once, then call `svc.UpsertProgramConfig` for a new custom program, then call `ph.listPrograms` again and assert the new program is present (proving liveness — the point of this tool vs. the Phase 1 enum).
- Files: `server/mcp/tools_programs_test.go`

---

## Verification Checklist (maps back to requirements.md's Acceptance Criteria)

| AC | Satisfied by |
|----|-------------|
| AC1 (custom program via `create_session`) | Task 1.2.1a + Task 1.4.1b |
| AC2 (custom program via `create_session_for_pr`) | Task 1.3.1a + Task 1.4.2a |
| AC3 (`claude`/`aider` + default unchanged) | Task 1.2.1a, Task 1.3.1a (both preserve `defaultProgramID` fallback behavior) + Task 1.4.1b's `TestCreateSession_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` and Task 1.4.2a's `TestCreateSessionForPR_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` (explicit `program="aider"` regression tests, added in this repair) |
| AC4 (discoverability) | Task 1.1.1a (description text + dynamic enum) + Task 1.2.1a/1.3.1a's non-fatal unrecognized-program warning (`ProgramWarning`, now tested per Task 1.4.1b/1.4.2a's `..._SetProgramWarning_...` tests, and surfaced from the tool-level `WithDescription` text so the calling agent knows to check it) + Task 1.4.1b's `TestCreateSession_should_SucceedWithNoWarning_When_SvcIsNilAndProgramIsATypo` (covers the `svc == nil` degraded-discoverability mode explicitly — both the dynamic enum and the warning check no-op together in that case, by design) + Phase 2 (`list_programs`, additive — recommended, not required, to ship alongside Phase 1 per pre-mortem.md #1) |
| AC5 (test coverage, no regressions) | Epic 1.4 (new tests, including the validation.md-flagged `ProgramWarning`, omitted-default, and rate-limiter-infra tests) + `make test`/`go test ./server/mcp/...` run clean on the full existing suite |
| AC6 (no proto/backend change; scope limited) | Phase 1 touches only `server/mcp/tools_lifecycle.go`, `server/mcp/tools_github.go`, and the new shared helper — confirm via `git diff --stat` showing no `proto/` or `config/` changes |
