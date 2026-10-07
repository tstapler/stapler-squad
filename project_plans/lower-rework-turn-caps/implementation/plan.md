# Implementation Plan: lower-rework-turn-caps

**Feature**: Expose `AutonomousMaxTurns` via Settings UI and fix stale proto/service comments
**Date**: 2026-10-07
**Status**: Ready for implementation
**ADRs**: ADR-001-follow-existing-pattern.md

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `AutonomousMaxTurns` | The per-session turn budget for one `AutonomousDriver` run; stored in `config.Config`, default 30, ceiling 200 | Go field at `config/config.go:311` |
| `autonomousMaxTurnsDefault` | Package-level constant (30) used when the stored value is zero or negative | `config/config.go:1077` |
| `autonomousMaxTurnsHardCeiling` | Package-level constant (200); values above it are clamped at read time by `OrDefault()` | `config/config.go:1078` |
| `AutonomousMaxTurnsOrDefault()` | Config method returning the clamped, resolved turn cap; callers should never read `AutonomousMaxTurns` directly | `config/config.go:1084` |
| `SessionDefaultsConfig` | Proto message returned by `GetSessionDefaults`; enumerates all configurable global defaults | `proto/session/v1/session.proto` |
| `UpdateGlobalDefaultsRequest` | Proto message sent by the UI when saving global defaults | `proto/session/v1/session.proto` |
| `sessionDefaultsToProto` | Go helper that constructs a `SessionDefaultsConfig` proto from a `*config.Config`; the single place to add new read fields | `server/services/defaults_service.go:536` |
| `GlobalDefaultsForm` | React component that loads and saves global defaults via `getSessionDefaults` / `updateGlobalDefaults` | `web-app/src/components/settings/GlobalDefaultsForm.tsx` |
| `newIsolatedDefaultsService` | Test helper that constructs a `DefaultsService` with its own isolated config directory | `server/services/defaults_service_test.go` |
| `sampleDefaults` | Shared mock object in `GlobalDefaultsForm.test.tsx`; must stay in sync with the proto shape | `web-app/src/components/settings/GlobalDefaultsForm.test.tsx:32` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Proto field placement | Append-in-order (field 15 in `SessionDefaultsConfig`, field 13 in `UpdateGlobalDefaultsRequest`) | `max_auto_rework_iterations` pattern, pitfalls.md | Custom field layout | Wire-format rule: field numbers must be permanently stable; appending is safest |
| Service read wiring | Single line in `sessionDefaultsToProto` via `AutonomousMaxTurnsOrDefault()` | Existing `MaxAutoReworkIterations` line at `defaults_service.go:549` | Inline clamping at the service layer | Clamping is already done by `OrDefault()` — duplicating it adds skew risk |
| Service write wiring | Unconditional assignment `cfg.AutonomousMaxTurns = int(req.Msg.AutonomousMaxTurns)` | `MaxAutoReworkIterations` assignment at `defaults_service.go:150` | Conditional assignment (only write if nonzero) | The "0 resets to default" convention requires unconditional writes; see stale-threshold bug described in test comment at line 232 |
| React state management | One `useState` per field, load via `useEffect`, submit as plain request | Existing `maxAutoReworkIterations` pattern in `GlobalDefaultsForm.tsx` | Shared config object state | Consistent with every existing field; no refactor scope |
| UI integer parsing | `parseInt(e.target.value, 10) \|\| 1` | UX research (pitfalls.md) | `Number(e.target.value)` | `parseInt` rejects decimal input that `Number` would accept silently for an integer field |
| Alternative A (chosen) | Mirror `max_auto_rework_iterations` end-to-end | Requirements, architecture.md | — | Zero learning curve; reviewers can diff-compare against the established pattern |
| Alternative B (rejected) | Refactor `UpdateGlobalDefaults` setter block into a generic mapper | — | Alternative A | Over-engineering for a small, bounded set of fields; adds complexity without benefit |
| Alternative C (rejected) | Expose via separate admin API, skip proto | — | Alternative A | Violates "everything through proto" convention; breaks UI configurability goal |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|---------------|-------------|---------------|
| `defaults_service.go` | None identified; architecture.md marks it clean | Extend as-is | No hotspots; cyclomatic complexity is well within gate; adding 2 lines follows the existing setter/getter pattern without introducing any new abstraction |
| `session.proto` stale comments | "server default (3)" at lines 2201 and 2282 — should be "(5)" since `DefaultMaxAutoReworkIterations = 5` | Fix in this PR as part of the proto edit pass | Zero wire-format risk; required by AC-7 |
| `backlog_service.go:950` stale comment | "default 3" — should be "default 5" | Fix in this PR alongside the proto edits | Single-line comment change; logically grouped with the proto fixes |

---

## Migration Plan

None — no database schema changes. Config JSON is backward-compatible: `autonomous_max_turns` is `omitempty` so existing `config.json` files without the field continue to work via `AutonomousMaxTurnsOrDefault()`.

## Observability Plan

- **Logs**: The existing `log.Info("updated global session defaults", ...)` line at `defaults_service.go:193` fires on every save. No new log lines needed; the field's value is visible in config.json.
- **Metrics**: None required for a config knob change.
- **Alerts**: None required.

## Risk Control

- **Feature flag**: None needed. The field is optional/omitempty; a missing value in a request is treated as "use default" by `AutonomousMaxTurnsOrDefault()`. Existing sessions are unaffected (the cap is read at driver-start time, not per-turn).
- **Rollback procedure**: Revert the commit. The field's `omitempty` tag means a rolled-back binary that doesn't know about the field simply ignores any `autonomous_max_turns` value stored in `config.json` and uses `AutonomousMaxTurnsOrDefault()`.
- **Staged rollout**: Not applicable. This is a local config knob; no server-side fan-out.

## Unresolved Questions

None.

## Dependency Visualization

```
proto/session/v1/session.proto
        │ make proto-gen
        ▼
gen/session/v1/session.pb.go          web-app/src/gen/ (TS types)
        │                                       │
        ▼                                       ▼
server/services/defaults_service.go   web-app/src/components/settings/
        │                             GlobalDefaultsForm.tsx
        ▼
server/services/defaults_service_test.go   GlobalDefaultsForm.test.tsx
```

All paths depend on the proto edit completing and `make proto-gen` running before any other file is touched.

---

## Phase 1: Proto and Generated Code

### Epic 1.1: Add `autonomous_max_turns` to the wire format

**Goal**: Land the new field in both proto messages and regenerate all downstream generated files.

#### Story 1.1.1: Edit `session.proto` — add field + fix stale comments

**As a** system operator, **I want** `autonomous_max_turns` in the proto schema, **so that** the gRPC client and server can exchange the turn cap.

**Acceptance Criteria**:
- `SessionDefaultsConfig` has `int32 autonomous_max_turns = 15` with comment "resolved default is 30; bounded \[1, 200\]".
  - *Given* `session.proto` has `RetryPolicyConfig retry_policy = 14` as the last field of `SessionDefaultsConfig`, *When* I append `int32 autonomous_max_turns = 15` with its comment, *Then* `buf lint` passes and field 15 is new (not a renumber of any existing field).
- `UpdateGlobalDefaultsRequest` has `int32 autonomous_max_turns = 13` with hint comment.
  - *Given* `session.proto` has `RetryPolicyConfig retry_policy = 12` as the last field of `UpdateGlobalDefaultsRequest`, *When* I append `int32 autonomous_max_turns = 13`, *Then* field 13 is new and field 12 is untouched.
- The two stale "server default (3)" comments at lines 2201 and 2282 are updated to "(5)".
  - *Given* `max_auto_rework_iterations` comments say "(3)" while `DefaultMaxAutoReworkIterations = 5`, *When* I change both occurrences to "(5)", *Then* `grep -n "server default (3)" session.proto` returns no matches.

**Files**:
- `proto/session/v1/session.proto`

##### Task 1.1.1a: Add `autonomous_max_turns = 15` to `SessionDefaultsConfig` (~3 min)
- **Pre-mortem FM-1 prevention**: Before editing, run `git fetch origin && git log --oneline origin/main -- proto/session/v1/session.proto` and confirm no concurrent proto PR has landed that claims field 15 in `SessionDefaultsConfig` or field 13 in `UpdateGlobalDefaultsRequest`. Re-verify both field numbers are free in upstream `main` before opening the PR.
- Open `proto/session/v1/session.proto` at line 2213 (end of `SessionDefaultsConfig`, after `RetryPolicyConfig retry_policy = 14`).
- Insert before the closing `}`:
  ```proto
  // Max turns a single autonomous-driver session may run. 0 in a request means
  // "use the server default"; the response always echoes the resolved value (30).
  // Bounded \[1, 200\] at read time.
  int32 autonomous_max_turns = 15;
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.1.1b: Add `autonomous_max_turns = 13` to `UpdateGlobalDefaultsRequest` (~2 min)
- Open `proto/session/v1/session.proto` at line 2292 (end of `UpdateGlobalDefaultsRequest`, after `RetryPolicyConfig retry_policy = 12`).
- Insert before the closing `}`:
  ```proto
  // 0 = use the server default (30). See SessionDefaultsConfig.autonomous_max_turns.
  int32 autonomous_max_turns = 13;
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.1.1c: Fix stale "(3)" comments in `session.proto` and `backlog_service.go` (~2 min)
- In `proto/session/v1/session.proto` at line 2201: change `"server default (3)"` to `"server default (5)"`.
- In `proto/session/v1/session.proto` at line 2282: change `"server default (3)"` to `"server default (5)"`.
- In `server/services/backlog_service.go` at line 950: change `"default 3"` to `"default 5"`.
- Files: `proto/session/v1/session.proto`, `server/services/backlog_service.go`

#### Story 1.1.2: Regenerate proto

**As a** developer, **I want** the generated Go and TypeScript code to reflect the new proto fields, **so that** the compiler can verify correctness end-to-end.

**Acceptance Criteria**:
- `make proto-gen` exits 0.
- `gen/session/v1/session.pb.go` and `web-app/src/gen/` contain `AutonomousMaxTurns` / `autonomousMaxTurns`.
  - *Given* `session.proto` has both new fields, *When* `make proto-gen` runs, *Then* `grep AutonomousMaxTurns gen/session/v1/session.pb.go` returns a hit.

**Files**:
- `gen/session/v1/session.pb.go` (generated, gitignored — do not commit)
- `web-app/src/gen/` (generated, gitignored — do not commit)

##### Task 1.1.2a: Run `make proto-gen` (~2 min)
- Run `make proto-gen` from the repo root.
- Confirm exit 0. If `web-app/node_modules` is missing, run `cd web-app && pnpm install` first.
- Files: generated only (do not stage)

---

## Phase 2: Backend Service Wiring

### Epic 2.1: Wire `autonomous_max_turns` through `defaults_service.go`

**Goal**: `GetSessionDefaults` returns the resolved turn cap; `UpdateGlobalDefaults` persists the new value.

#### Story 2.1.1: Read path — populate `autonomous_max_turns` in `sessionDefaultsToProto`

**As a** settings form, **I want** `GetSessionDefaults` to return the current `AutonomousMaxTurns`, **so that** the UI pre-populates with the live server value.

**Acceptance Criteria**:
- `GetSessionDefaults` response's `autonomous_max_turns` equals `AutonomousMaxTurnsOrDefault()`, never 0.
  - *Given* a fresh config with no `AutonomousMaxTurns` set, *When* `GetSessionDefaults` is called, *Then* `resp.Msg.Defaults.AutonomousMaxTurns == 30`.

**Files**:
- `server/services/defaults_service.go`

##### Task 2.1.1a: Add read field to `sessionDefaultsToProto` (~2 min)
- In `server/services/defaults_service.go` at line 553 (after `StaleSessionThresholdMinutes`), add:
  ```go
  // #nosec G115 -- see MaxAutoReworkIterations above.
  AutonomousMaxTurns: int32(cfg.AutonomousMaxTurnsOrDefault()),
  ```
- Files: `server/services/defaults_service.go`

#### Story 2.1.2: Write path — persist `autonomous_max_turns` in `UpdateGlobalDefaults`

**As a** system operator, **I want** saving the form to persist the new turn cap, **so that** newly started drivers pick it up.

**Acceptance Criteria**:
- Sending `autonomous_max_turns: 50` persists `50` in `config.json`.
- Sending `autonomous_max_turns: 0` stores `0` (resets to default via `OrDefault()` at next read).
  - *Given* a config with `AutonomousMaxTurns: 50`, *When* `UpdateGlobalDefaults` is called with `AutonomousMaxTurns: 0`, *Then* `config.AutonomousMaxTurnsOrDefault()` returns `30`.

**Files**:
- `server/services/defaults_service.go`

##### Task 2.1.2a: Add write assignment in `UpdateGlobalDefaults` (~2 min)
- In `server/services/defaults_service.go` at line 150 (after `cfg.MaxAutoReworkIterations = int(req.Msg.MaxAutoReworkIterations)`), add:
  ```go
  cfg.AutonomousMaxTurns = int(req.Msg.AutonomousMaxTurns)
  ```
- Note: **do not** add `AutonomousMaxTurns` to the `sharedBacklogCfg` propagation block (lines 186–191). The turn cap is consumed via `config.LoadConfig()` at every driver start; live propagation is only needed for fields that must take effect without a restart (concurrency cap, rework cap).
- Files: `server/services/defaults_service.go`

#### Story 2.1.3: Go tests for the new field

**As a** developer, **I want** `defaults_service_test.go` to cover `autonomous_max_turns` in both RPC paths, **so that** regressions are caught by CI.

**Acceptance Criteria**:
- A test asserts `GetSessionDefaults` returns `AutonomousMaxTurnsOrDefault()` (30 on a fresh config).
- A test asserts that sending `autonomous_max_turns: 0` resets to the default (30 in the response).
  - *Given* a fresh isolated service, *When* `UpdateGlobalDefaults` is called with `AutonomousMaxTurns: 0`, *Then* `resp.Msg.Defaults.AutonomousMaxTurns == 30`.
- A test asserts that sending `autonomous_max_turns: 50` persists and round-trips as 50.
  - *Given* a fresh isolated service, *When* `UpdateGlobalDefaults` is called with `AutonomousMaxTurns: 50`, *Then* `resp.Msg.Defaults.AutonomousMaxTurns == 50`.

**Files**:
- `server/services/defaults_service_test.go`

##### Task 2.1.3a: Add `TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault` (~3 min)
- After `TestGetSessionDefaults_ServesRuntimeReworkCapDefault` (line 220), add:
  ```go
  // TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault pins the settings
  // form's autonomous-turns field to the cap the server enforces.
  func TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault(t *testing.T) {
      svc := newIsolatedDefaultsService(t)

      resp, err := svc.GetSessionDefaults(context.Background(), connect.NewRequest(&sessionv1.GetSessionDefaultsRequest{}))
      require.NoError(t, err)
      require.NotNil(t, resp.Msg.Defaults)
      assert.Equal(t, int32(config.AutonomousMaxTurnsDefault), resp.Msg.Defaults.AutonomousMaxTurns)
  }
  ```
  Note: if `config.AutonomousMaxTurnsDefault` is not exported, use the literal `int32(30)` and add a comment citing `config/config.go:1077`.
- Files: `server/services/defaults_service_test.go`

##### Task 2.1.3b: Add `TestUpdateGlobalDefaults_AutonomousMaxTurns` (~4 min)
- Add a sub-test after the stale-threshold test block:
  ```go
  func TestUpdateGlobalDefaults_AutonomousMaxTurns(t *testing.T) {
      t.Run("ZeroResetsToDefault", func(t *testing.T) {
          svc := newIsolatedDefaultsService(t)
          resp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
              AutonomousMaxTurns: 0,
          }))
          require.NoError(t, err)
          assert.Equal(t, int32(30), resp.Msg.Defaults.AutonomousMaxTurns)
      })

      t.Run("ExplicitValueRoundTrips", func(t *testing.T) {
          svc := newIsolatedDefaultsService(t)
          resp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
              AutonomousMaxTurns: 50,
          }))
          require.NoError(t, err)
          assert.Equal(t, int32(50), resp.Msg.Defaults.AutonomousMaxTurns)
      })
  }
  ```
- Files: `server/services/defaults_service_test.go`

---

## Phase 3: Frontend Form

### Epic 3.1: Add `autonomousMaxTurns` to `GlobalDefaultsForm`

**Goal**: The Settings → Global Defaults form shows and saves the autonomous session turn cap.

#### Story 3.1.1: State, load, and submit wiring

**As a** system operator, **I want** the turn cap loaded from the server and sent back on save, **so that** what I type is persisted.

**Acceptance Criteria**:
- `useState(0)` for `autonomousMaxTurns` initialized to 0 (server-resolved on load).
- `loadDefaults` sets `autonomousMaxTurns` from `defaults.autonomousMaxTurns || 30`.
- `handleSave` includes `autonomousMaxTurns` in the `updateGlobalDefaults` call.
  - *Given* the server returns `autonomousMaxTurns: 45` from `getSessionDefaults`, *When* the form loads, *Then* the turn-cap input shows `45`.

**Files**:
- `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 3.1.1a: Add state declaration (~1 min)
- In `GlobalDefaultsForm.tsx` after line 38 (`maxAutoReworkIterations` state), add:
  ```tsx
  const [autonomousMaxTurns, setAutonomousMaxTurns] = useState(0); // set from server-resolved default on load
  ```
- Files: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 3.1.1b: Add load wiring (~1 min)
- In `loadDefaults`, after line 62 (`setMaxAutoReworkIterations(...)`), add:
  ```tsx
  setAutonomousMaxTurns(defaults.autonomousMaxTurns || 30);
  ```
- Files: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 3.1.1c: Add submit wiring (~1 min)
- In `handleSave`'s `updateGlobalDefaults` call (after `maxAutoReworkIterations,` at line 104), add:
  ```tsx
  autonomousMaxTurns,
  ```
- Files: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

#### Story 3.1.2: Render the input field

**As a** system operator, **I want** a labeled numeric input for the turn cap in the form, **so that** I can view and change it.

**Acceptance Criteria**:
- A `<label>` with text "Max Autonomous Session Turns" has `htmlFor="global-autonomous-max-turns"`.
- A `<input type="number" min={1} max={200}>` with matching `id` is present.
- The input is positioned between "Max Auto-Rework Iterations" and "Max Concurrent Backlog Work Items".
- The hint text names both the default (30) and the ceiling (200) and explains both failure modes.
  - *Given* the server returns `autonomousMaxTurns: 30`, *When* the form renders, *Then* `screen.getByLabelText("Max Autonomous Session Turns")` has value `30`.

**Files**:
- `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 3.1.2a: Insert the rendered field (~3 min)
- In `GlobalDefaultsForm.tsx`, after the closing `</div>` of the "Max Auto-Rework Iterations" block (after line 330), insert:
  ```tsx
  {/* Max Autonomous Session Turns */}
  <div className={field}>
    <label className={labelClass} htmlFor="global-autonomous-max-turns">
      Max Autonomous Session Turns
    </label>
    <input
      id="global-autonomous-max-turns"
      type="number"
      min={1}
      max={200}
      className={input}
      value={autonomousMaxTurns}
      onChange={(e) =>
        setAutonomousMaxTurns(Math.min(200, Math.max(1, parseInt(e.target.value, 10) || 1)))
      }
    />
    <p className={hint}>
      How many turns a single autonomous session may run before it stops and waits
      for review. Default: 30; ceiling: 200. Too low cuts off mid-task; too high
      risks runaway cost.
    </p>
  </div>
  ```
- Files: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

#### Story 3.1.3: Frontend tests

**As a** developer, **I want** `GlobalDefaultsForm.test.tsx` to cover `autonomousMaxTurns`, **so that** the field cannot silently regress.

**Acceptance Criteria**:
- `sampleDefaults` includes `autonomousMaxTurns: 30`.
- A test verifies the input renders with the server-provided value (e.g. 45).
  - *Given* `getSessionDefaults` returns `autonomousMaxTurns: 45`, *When* the form loads, *Then* `screen.findByLabelText("Max Autonomous Session Turns")` resolves to an element with value `45`.

**Files**:
- `web-app/src/components/settings/GlobalDefaultsForm.test.tsx`

##### Task 3.1.3a: Add `autonomousMaxTurns: 30` to `sampleDefaults` (~1 min)
- In `GlobalDefaultsForm.test.tsx` at line 39 (after `maxAutoReworkIterations: 3`), add:
  ```tsx
  autonomousMaxTurns: 30,
  ```
- Files: `web-app/src/components/settings/GlobalDefaultsForm.test.tsx`

##### Task 3.1.3b: Add render test for the turn-cap field (~3 min)
- After the `"shows the server-resolved Max Auto-Rework Iterations default"` test, add:
  ```tsx
  it("shows the server-resolved Max Autonomous Session Turns default, not a client constant", async () => {
    mockGetSessionDefaults.mockResolvedValue({
      defaults: { ...sampleDefaults, autonomousMaxTurns: 45 },
    });
    render(<GlobalDefaultsForm />);
    expect(await screen.findByLabelText("Max Autonomous Session Turns")).toHaveValue(45);
  });
  ```
- Files: `web-app/src/components/settings/GlobalDefaultsForm.test.tsx`

---

## Phase 4: Build Verification

### Epic 4.1: Confirm `make build && make test` pass

**Goal**: Ensure no compilation or test failures before declaring done.

#### Story 4.1.1: Full build and test run

**As a** developer, **I want** CI-equivalent validation to pass locally, **so that** the PR is green on first push.

**Acceptance Criteria**:
- `make build` exits 0 (proto gen + Go compilation).
- `make test` exits 0 with no new failures.
- Frontend Jest tests pass (`cd web-app && npx jest --no-coverage`).
  - *Given* all Phase 1–3 changes are applied, *When* `make build && make test` runs, *Then* exit code is 0 and the new test names appear in the output.

**Files**: none (verification only)

##### Task 4.1.1a: Build and test (~5 min)
- Run `make build` — confirms proto gen + Go compilation.
- Run `make ci` (not just `make test`) — also runs `fmt-check`, `registry-generate`, `lint-custom`, and other gates that CI enforces but `make test` skips. If `make ci` is too slow, at minimum run `make test fmt-check registry-generate` and confirm all exit 0 before pushing.
- Run `cd web-app && npx jest --testPathPatterns="GlobalDefaultsForm" --no-coverage` — confirms the new TSX test passes.
- Run `gofmt -w .` on every Go file touched before staging.
- Files: n/a
