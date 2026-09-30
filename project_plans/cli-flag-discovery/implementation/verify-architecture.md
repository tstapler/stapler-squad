# Verify: architecture + refactor candidates (cli-flag-discovery)

Scope: `git diff main...HEAD` (82 files, +9485/-37). Method: read the diff hunks of every seam file, `go list` import graph, `go build ./...` (clean, no output), kibitzer `architecture_assessment` scoped to `config/clihelp/**`. Not run: full test suite, tsc, `server/middleware` kibitzer scope (only clihelp scoped).

## Tech Debt Disposition (a)

| Row | Verdict | Evidence |
|---|---|---|
| defaults_service.go Isolate via seam | CARRIED OUT | Diff adds 1 field, 1 handler, 1 setter, 1 starter + 3 small mapping funcs (`probeResultToProto`, `flagsToProto`, `probeStatusToProto`); all logic in `config/clihelp`. |
| session_service.go Isolate | CARRIED OUT | `session_service.go:5225-5233`: two 1-line delegators (ProbeProgram + StartProgramProbeLoginPath). |
| OmnibarCreationPanel.tsx Isolate | CARRIED OUT | Net one JSX element (`<ProgramProbeSection>`) + `aria-describedby`; removed inline warning and `isProgramRecognized`. |
| server.go Isolate | CARRIED OUT | Guard only in `localChain()` (Start), `remoteChain()` has none. |
| ProgramsManager Extend as-is | OK (see C3) | +122 lines. |
| session.proto Extend additive | OK | Field numbers 1..N contiguous, new enum has `_UNSPECIFIED = 0`, no NO_SIDE_EFFECTS, new rpc + messages only. |

No skipped Isolate/Refactor-first row. **Zero BLOCKERs.**

## Structure / coupling (b)

- Import graph VERIFIED (`go list`): `config/clihelp` imports only `executor/safeexec` (+ stdlib, x/sync); no `server/`, no `config` parent, no cycles. `server/middleware` imports only `log`.
- `probeguard.go` is layered correctly: pure `net/http`, config passed as lazy funcs; `server.go` only wires it.
- Wiring: `DefaultsService` gets prober via constructor + `SetProber` (test seam) — acceptable.

## Findings

### CONCERN

C1. `config/clihelp/prober.go:199-213` (`logProbe`): audit line omits fields the plan's Observability Plan specifies for AC8: `command_token`, `is_wrapper`, `remote_addr`, and names the field `duration` not `duration_ms`. Rationale: audit record is the sole compensating control (ADR-002 forgoes executor audit) and, with `resolved_path=""` on NOT_FOUND, a rejected/unfound probe records nothing about what was asked. Fix: pass `Target.Name` (first token, never args/env) and `IsWrapper` into `logProbe`; add remote addr via ctx from the guard (or drop it from the plan). Verified by reading; no test asserts field set.

C2. `server/services/defaults_service.go:73-75` `StartProgramProbeLoginPath` dereferences `d.prober`; `SetProber(nil)` or a zero-value `DefaultsService{}` would panic (grep found only the constructor building one, so no current caller does this). Fix: nil-guard in `ProbeProgram`/`Start...` or make `SetProber` reject nil. Low likelihood.

C3. `web-app/.../ProgramsManager.tsx` (~+70 lines of logic in the component body): focus-restoration refs (`checkClicked`, `wasChecking`, effect), `settledFlags` blur logic, `flagsDescribedBy` computed inline. The plan's "Extend as-is" assumed thin additions; this mixes UI state machines into a form. Fix: extract `useCheckButtonFocus(state)` and `useFlagValidation(cliFlags, probeState)` hooks (also removes the same validate-then-warn pair repeated in ProgramProbeSection). Effort ~1-1.5h.

C4. Duplicated Check button: `ProgramsManager.tsx` Check button (`data-testid=prog-command-check`) AND `ProbeStatusBadge.tsx:188-192` (`onConfirm` "Check"/"Check again") both call `probe.check({explicit:true})`; on needsConfirm/timeout the form shows two "Check" buttons. Both wired for the same action, plus badge `onRetry` (`:184`) as a third path. Fix: pick one owner (badge renders button, form passes `commandRowAction` slot, or form hides the badge button via prop `hideCheck`). Effort ~1h incl. tests/e2e locators. Verified by reading both files.

C5. `config/clihelp` totals 3112 lines (kibitzer `[package-size]` advisory; ~1600 of it is test/fixtures, non-test code is ~1580 lines / 14 files, so the advisory overstates). Real signal: `Prober` struct has 16 fields with 10 exported `With*` options (`prober.go:22-81`), several used only by tests (`WithShell`, `WithClock`, `WithHome`, `WithEvalSymlinks`, `WithReadHead`). Not urgent; consider splitting `loginpath*` into its own small type behind an interface `DirSource`.

### NITPICK

N1. Dead code: `web-app/src/components/sessions/OmnibarPresetList.css.ts:71` `programWarning` is now unreferenced (its only consumer was removed from `OmnibarCreationPanel.tsx` in this diff; grep shows only the definition). Delete it (project rule: leave no dead code from your own diff). Effort 5 min.

N2. `config/clihelp/types.go:33-40` `ProbeStatus.String()` has a redundant `case ProbeStatusUnspecified` identical to `default`.

N3. `config/clihelp/runner.go:90` function 53 lines (kibitzer long-function advisory, verified in report); acceptable but `runWith` could split spawn vs collect.

N4. `defaults_service.go` `probeStatusToProto` `found` bool returned by switch duplicates knowledge in the proto comment; consider a `ProbeStatus.Found()` method in clihelp so the definition of "found" lives in one place with a test. Effort 20 min.

N5. Over-engineering check (c): every major piece maps to an AC or ADR (ProbeGuard -> AC8/ADR-001; NEEDS_CONFIRM/resolve_only/confirm set -> ADR-001 addition, plan Flagged Choice 12; wrapper detection -> Flagged Choice 7; feature flag -> Flagged Choice 8; login PATH -> Flagged Choice 10). These exceed the literal 10 ACs but are plan-approved; note the `FlagNotes.css.ts` style file, `useListboxNav`, and `AvailableFlags` are supported by Flagged Choices 5/9. None flagged as unrequested.

## Testability (d)
Prober seams (lookPath/stat/run/readHead/evalSymlinks/clock) allow isolated tests; guard takes lazy funcs; `localChain`/`remoteChain` are factored for chain tests (`server_probeguard_test.go`). Good.

## Top 5 refactor candidates
1. Merge duplicate Check buttons (C4) — ~1h
2. Extract focus/flag-validation hooks out of ProgramsManager (C3) — ~1.5h
3. Complete audit log fields per plan (C1) — ~45m
4. Remove dead `programWarning` style (N1) — 5m
5. `ProbeStatus.Found()` in clihelp instead of switch-derived bool in service (N4) — 20m
