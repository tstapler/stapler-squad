# Implementation Plan: program-env-injection

**Feature**: A custom program's registered `env` map must reach the spawned session process. The production fix already landed in two commits: `cdfd4e5cf2` (PR #825 squash; `buildExtraEnv` with an inline program-env merge, and `wireTmuxSession`) and `5da2af7bf` (#852, 2026-09-23; `resolveExtraEnvVars` and `claudeSettingsEnvOverrideArgs`, VERIFIED with `git log -S`; not an ancestor of `cdfd4e5cf2`). All in `session/instance_tmux.go`. This run closes the open acceptance criteria AC1 and AC4 with recorded evidence, proves AC3 with a real red/green run, and closes the test-coverage gaps the research found. **No production source file changes.** Epic 1.1 (AC-bearing, includes the lint repair of the already-landed test) is the first shippable unit; Epics 1.2-1.4.1 are AC-less hardening that may land in a separate PR.
**Date**: 2026-10-10
**Status**: Ready for implementation
**ADRs**: None. No new technology or architectural pattern; every task adds tests or recorded evidence. Follow-up F2 (restart semantics) should open an ADR when taken, see Production-Behaviour Decisions.
**Inputs**: `project_plans/program-env-injection/requirements.md`, `research/{stack,architecture,pitfalls,build-vs-buy}.md`, prior plan `project_plans/program-env-not-applied/implementation/plan.md` (its `decisions/` dir is empty, so there are no prior ADRs to carry).
**Evidence file produced by the implementer**: `project_plans/program-env-injection/implementation/evidence.md` (sections E1-E8; commands and verbatim output).

---

## Scope Decision (made, not asked)

Three approaches were considered (Step 0.5 CREATIVE pass):

| Approach | Strength | Weakness |
|---|---|---|
| A. Evidence only: run the existing tests and paste output | Cheapest; zero risk | Leaves AC4 proven only at string level (`TestBuildClaudeCommand_IncludesSettingsEnvOverride`), the `--settings` path never executed end to end, AC3 never actually shown red, and no durable guard for the gaps research found |
| **B. Evidence + test-only hardening (chosen)** | Durable regression coverage for every verified gap that is expressible as a test; zero production-behaviour risk; each task verifiable by one `go test` line | Known production-level gaps (secrets in logs, restart semantics, reserved keys, remote, tymux) stay documented, not fixed |
| C. Evidence + production hardening (resolve-once refactor, log redaction, reserved-key stripping, remote `-e`) | Closes every gap in one pass | Touches `session/instance_tmux.go` (1,602 lines, 40 commits in 90 days per `research/architecture.md` section 4); three of the five changes need owner product decisions (restart policy, remote support, whether users may set `CLAUDECODE`); widens the diff far beyond AC1-AC5 |

**Chosen: B.** Each production-level gap becomes a named follow-up (F1-F5) with its risk stated, so a reviewer can accept or reject each separately.

---

## Domain Glossary

*(Names below are used verbatim in test names, helper names and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `ProgramConfig` | Persisted custom-program record: `ID`, `Label`, `Command`, `CLIFlags`, `Env map[string]string`. | Written by `DefaultsService.UpsertProgramConfig` (`server/services/defaults_service.go:709`) |
| `ResolvedProgram` | Result of `config.ResolveProgramConfig(cfg, program)`: `Command`, `CLIFlags`, `EnvVars` (after `${VAR}` expansion), `IsCustom`. | `config/defaults.go:211` |
| `ResolvedEnv` | The `map[string]string` returned by `Instance.resolveExtraEnvVars()`: program env overlaid by instance `EnvVars` (instance wins). | `session/instance_tmux.go:710` |
| `ExtraEnvPairs` | The `[]string` of `KEY=VALUE` from `Instance.buildExtraEnv()`: `STAPLER_SESSION_UUID` first, then `ResolvedEnv` in random map order. | Tests must not assert global order (`research/stack.md` section 6) |
| `SettingsEnvOverride` | The `--settings '<json>'` pair from `claudeSettingsEnvOverrideArgs()`, carrying `ResolvedEnv` so it outranks Claude's own `settings.json` `env` block (#852). | Only on Claude launches |
| `wireTmuxSession` | The single production caller of `TmuxSession.SetExtraEnv` (`session/instance_tmux.go:795`); 8 call sites total. | VERIFIED by grep this run |
| `TmuxSession.ExtraEnv` / `extraEnv` | Exported slice (VNC `DISPLAY`/CDP, written at `session/instance.go:1641,1648,1785,1792,1952,1959,2053,2060`) and unexported slice (program env, written only by `SetExtraEnv`); both are emitted as `-e` pairs, `ExtraEnv` first. | `session/tmux/tmux.go:129,251` |
| `newSessionArgs` | `TmuxSession` method building the `new-session` argv for the recreate path; `start()` builds the same argv inline. | `session/tmux/tmux_session_start.go:637` vs `:222-227` |
| `InstanceEnvVars` | `Instance.EnvVars`: request-level env plus a create-time copy of the program's env for unset keys (`session_service_create.go:280-290`). Not persisted. | Source of the frozen-vs-reloaded divergence |
| `ReservedKey` | An env key stapler-squad owns: `STAPLER_SESSION_UUID` (ownership marker) and `CLAUDECODE` (always unset via `-e CLAUDECODE=`). | No guard today; F3 |
| `ProbeKey` | A unique env key (`SSQ_PROGRAM_ENV_PROBE`) the regression test registers as a defensive measure against a user's rc files masking the injection (no occurrence observed; see PIT-G). | New in Task 1.1.1b |
| `PreFixOverlay` | A `go test -overlay` JSON mapping `session/instance_tmux.go` to a scratch copy whose `wireTmuxSession` sets only `STAPLER_SESSION_UUID`, reproducing the pre-`cdfd4e5cf2` behaviour. | Never committed; the tree is not modified |
| `FakeClaude` | A shell script named `claude` that writes its argv to a file then `sleep`s; `isClaude` matches on token basename (`session/instance_tmux.go:239`). | Lets the real `--settings` path run through real tmux |
| `EvidenceLog` | `project_plans/program-env-injection/implementation/evidence.md`: verbatim commands and output. | Per-AC proof |
| Frozen vs reloaded env | In-process restart keeps the create-time `InstanceEnvVars` copy (frozen); after a server restart `EnvVars` is empty and program env is re-resolved (reloaded). | Characterized by Story 1.3.2, policy deferred (F2) |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Proving AC3 (test fails pre-fix) | Mutation by build overlay (`go test -overlay`), run then discarded | Go toolchain `-overlay`; manual mutation testing | Whole-file `git show cdfd4e5cf2^:session/instance_tmux.go` revert; checking out `cdfd4e5cf2^` and copying the test in | Whole-file revert deletes `wireTmuxSession`, which 8 call sites use (prior pre-mortem.md Failure #1). The parent-commit run is NOT rejected as impossible: the coordinator ran it (evidence E4b, see Task 1.1.2b) and it failed on the intended assertion, but it needs heavy setup (proto regen with connect-go pinned to v1.19.1, ent regen with `--feature sql/upsert`), so the overlay stays the primary, cheap, repeatable proof and the parent run is corroboration. Overlay was run this session: a deliberate compile error in the overlay copy broke `go build -overlay` while the tree stayed clean, so it is both effective and cannot leave the tree broken. Prior plan's scratch worktree + hand-edit remains the fallback |
| `-e` argv coverage | Table-driven test over a fake executor (`MockCmdExec`) | Meszaros xUnit Test Patterns; repo convention (`session/tmux/tmux_ownership_test.go`) | Real tmux for this layer; prior plan's `spyCommandRunner` | New-session argv goes through `t.cmdExec.Run(cmd)` (`tmux_session_start.go:246`, `:655`), not `CommandRunner`; `spyCommandRunner` is the wrong seam |
| `--settings` end to end | Fake executable (`FakeClaude`) behind a real `CreateSession` and real tmux | Test Double: Fake; same trick as `session/instance_tmux_command_length_test.go:94` onward | Real `claude` binary; asserting only the command string | Real Claude needs credentials and a model call, and its precedence is upstream behaviour we cannot assert in CI. The string-level test cannot see shell/tmux mangling |
| Hostile-value quoting | Table of inputs executed through a real `sh -c` (round trip) | Property-style input table | Swapping in a quoting library | `build-vs-buy.md` verdict: no library; the only hand-rolled code (`shellQuote`, `instance_tmux.go:433`) is 1 line and is best verified by execution |
| Frozen-vs-reloaded semantics | Characterization test (Feathers): pin current behaviour, name it, do not endorse it | *Working Effectively with Legacy Code* | Changing behaviour now | Policy is an owner decision (F2); pinning makes a later change deliberate |
| Env value types | Keep `map[string]string` / `[]string` `KEY=VALUE` | type-driven-design | `EnvKey`/`EnvPair` newtypes | Test-only change; newtypes would force production edits in a hot file. Noted as part of F2's seam |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `session/instance_tmux.go` env functions (`resolveExtraEnvVars`, `buildExtraEnv`, `claudeSettingsEnvOverrideArgs`) | High-churn 1.6k-line file; env resolved in up to 5 separate `Snapshot()`/`LoadConfig()` calls per launch (PIT-2a) | **Extend as-is** (no change this item); resolve-once is a named seam in F2 | `research/architecture.md` section 5 recommends Extend as-is: seam is already single-sourced and this item adds no production code |
| Duplicate `-e` loops: `start()` inline (`tmux_session_start.go:222-227`) vs `newSessionArgs` (`:637-647`) | Two copies can drift; `start()` carries `//nolint:gocognit,gocyclo` | **Isolate via seam, deferred**; Task 1.2.2a is the regression net that makes the later consolidation safe | Still duplicated at HEAD (opened this run). Consolidation touches the nolint'd hotspot and is orthogonal to the ACs |
| Exported `ExtraEnv` vs unexported `extraEnv` | Two fields with overlapping purpose; `ExtraEnv = append(...)` at `instance.go:1641/1785` accumulates on a reused session | **Extend as-is** | Both live and non-clobbering; duplicates harmless; Task 1.2.1a pins that both are emitted in order. Rename is cosmetic (ARCH-7) |
| Two `shellQuote` copies (`instance_tmux.go:433` emits `'\''`; `session/tmux/ssh_runner.go:601` emits `'"'"'`) | Near-identical, not byte-identical | **Extend as-is** | `build-vs-buy.md` section 4 marks dedupe out of scope; Story 1.3.1 tests the one that feeds `--settings` |
| `initTmuxSession` reuse guard (`instance_tmux.go:807-810`) | Reuse skips `wireTmuxSession`, so edited program env is not applied to a live session | **Extend as-is** | Intentional (#791/#797); pinned by `TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed` (`session/instance_tmux_test.go:136`); behaviour documented by Task 1.1.1b |

---

## Migration Plan

N/A: no schema or data changes. (`EnvVars` is deliberately not persisted today; changing that is F2.)

## Observability Plan

N/A for this item: no production code, logs or metrics change. Existing INFO logs that embed the `--settings` JSON (PIT-A) are tracked as F1.

## Risk Control

- **Feature flag**: not gated; test-only and evidence-only change.
- **Rollback procedure**: revert the test commits (`git revert`); nothing in production depends on them.
- **Staged rollout**: full rollout on merge.
- **Test-environment risk**: the real-tmux tests skip when no tmux binary is found and fail (not skip) when tmux exists but the session never reaches `Active` (`1b82f2889`). A tmux client/server version mismatch (`session/tmux/binary_resolution.go:38-39`) therefore fails the test as an environment fault, not a regression; evidence runs must use the same `TMUX_BIN`/`tmux.Binary()` CI uses.

### Production-Behaviour Decisions

Every change that would alter production behaviour is held out of this item. Each is a follow-up (F-number) with its risk.

| ID | Change | Risk if done | Decision |
|----|--------|-------------|----------|
| F1 | Redact `--settings` env values from the three INFO logs (`instance_tmux.go:817`, `tmux_session_start.go:384`, `:671`) and from persisted/returned `LaunchCommand` (`instance_serialization.go:165`, `storage.go:160-162`, `server/adapters/instance_adapter.go:124`), or move `--settings` to a 0600 file | Medium. `LaunchCommand` is shown in the UI (`SessionDetailView.tsx`, `SessionCard.tsx`) and persisted, so redacting changes a user-visible field and stored data; moving env to a file adds a file lifecycle (cleanup, permissions, remote hosts). Redaction alone is partial: the value stays in the pane process argv (`ps`/`/proc/<pid>/cmdline`) and in `tmux -e` argv | **Follow-up item.** Needs a design choice (redact vs file) and a real-launch secret-leak test. Breaks the repo's own rule (`session_service_create.go:613`: "KEY NAMES ONLY -- never values") so it should be scheduled, not dropped |
| F2 | Resolve env once per launch and feed `-e` and `--settings` from it; choose restart policy (frozen at create vs re-resolve every launch; persist or drop `EnvVars`); make `extraEnv` re-resolve on `recreateMissingSession` | Medium-high. Changes signatures in a 40-commit/90-day file; changing policy alters what existing sessions see after a restart (a rotated token starts being picked up, or a removed key disappears) | **Follow-up item** with an ADR. Needs owner policy decision. This item ships characterization tests so the change is deliberate |
| F3 | Strip or reject `STAPLER_SESSION_UUID` and `CLAUDECODE` from user maps | Low-medium. Silently drops a key someone may set on purpose (`CLAUDECODE` set to re-enable the nested guard). Mechanism: `expectedOwnerUUID` returns the first `STAPLER_SESSION_UUID` in `extraEnv` (`tmux_ownership.go:44-52`), but a later user entry would win in tmux, so the ownership check would see a mismatch and treat the pane as foreign (VERIFIED on tmux 3.6a: later `-e` wins, see Task 1.4.1a) | **Follow-up item.** Hostile/odd config only; no evidence it occurs |
| F4 | Remote execution targets: pass `-e` in `createRemoteSession` | Medium. New code path over SSH; remote tmux version unknown (`-e` needs a new-enough tmux); needs a remote-runner test harness | **Follow-up item**, gated on the requirements question "is remote in scope" |
| F5 | tymux gRPC backend: env injection | Unknown. Backend is a per-session opt-in (`Instance.Backend`, `session/backend_factory.go:70`); `wireTmuxSession` builds a `TmuxSession` for it but only `*TmuxBackend` receives it, so env is INFERRED to be dropped | **Follow-up item**, starting with an investigation task, since no one has confirmed the gap |

## Unresolved Questions

- [ ] File follow-ups F1-F5 as backlog items now, or leave them in `evidence.md` and the PR body? Does not block any task; Task 1.4.2a records them either way. Owner: Tyler.
- [ ] Is remote execution target in scope for custom-program env? Blocks F4 only. Owner: Tyler.
- [ ] Restart policy (frozen vs reloaded, persist `EnvVars` or not)? Blocks F2 only. Owner: Tyler.
- [x] Does `CreateSession` accept a custom program whose `Command` is the `FakeClaude` script? Resolved (adversarial review experiment 3): yes. `UpsertProgramConfig` with `Command=<tmp>/claude` and `SESSION_TYPE_NEW_WORKTREE` reached `Active`, recorded argv `--settings <env JSON>`, hostile value intact, `pwned` absent. The expected `ERROR claude launch: MCP server URL unresolved` log line is harmless in this test setup.
- [x] Does a later `-e` for the same key win in tmux? Resolved: on tmux 3.6a (private socket) `new-session -e K=first -e K=second` then `show-environment -t dup K` printed `K=second` (later wins); VERIFIED by the adversarial review. Task 1.4.1a is reduced to pasting that result.

## Dependency Visualization

```
Epic 1.1 is the first shippable unit (AC evidence + lint repair); Epics 1.2-1.4.1 may land in a separate PR.
Parallelism is per PACKAGE, not per file: one half-written _test.go breaks `go test`
for every agent in the same package/worktree. One agent per package, or one worktree per agent.

Wave 1
  Agent A (package server/services): 1.1.0a -> 1.1.1b -> 1.1.1c, then 1.1.3c
  Agent B (package session/tmux):     1.2.1a -> 1.2.2a
  Agent C (package session):          1.3.1a -> 1.3.2a -> 1.3.2b
  Agent D (web-app):                  1.3.3a
  Read-only / run-only (any agent):   1.1.1a baseline, 1.4.1a (private tmux socket)
      |
Wave 2
  1.1.3a, 1.1.3b     (read-only evidence)
      |
Wave 3 (-overlay does not touch the tree, so these may run concurrently;
        they only need the Wave 1 test edits to be stable)
  1.1.2a -> 1.1.2b -> 1.1.2c        (AC3, AC5)
  1.1.3d                            (needs 1.1.3c)
      |
Wave 4
  1.1.1d  repeat-run flake evidence (-count=5 -race)
  1.4.2a  follow-ups recorded
  1.4.2b  final gates (custom linter, lint, targeted test set)
```

Critical path: 1.1.0a -> 1.1.1b -> 1.1.1c -> 1.1.2a -> 1.1.2b -> 1.1.2c -> 1.4.2b.

---

## Prior Plan Reconciliation

Source: `project_plans/program-env-not-applied/implementation/plan.md` (cited as PRIOR). Verified against HEAD `9ef8fbc68`; the regression test landed in `aac276425` and `1b82f2889`.

| PRIOR section | Status | Evidence / disposition |
|---|---|---|
| Header: test-only hardening of an already-fixed bug | **Still holds** | Fix at `cdfd4e5cf2`; this plan has the same shape |
| Domain Glossary line references | **No longer holds (drift)** | `resolveExtraEnvVars` 585-599 -> 710-724 (introduced by `5da2af7bf`, #852, not `cdfd4e5cf2`); `buildExtraEnv` 604-614 -> 729-739; `claudeSettingsEnvOverrideArgs` 631-642 -> 756-767; `wireTmuxSession` 646-676 -> 771-801; `initTmuxSession` 681-698 -> 806-823; `SetExtraEnv` `tmux.go:1174` -> `:1238`; `newSessionArgs` 611-624 -> 637-647; inline loop 214-220 -> 222-227. "7 call sites" is now 8 (`instance_tmux.go:822`, `instance.go:2399,2600`, `instance_serialization.go:467,472,536,550,563`). `wrapRemoteCommand` row not re-opened |
| Glossary: "`ExtraEnv` is not dead code" correction | **Still holds** | Live writers at `instance.go:1641,1648,1785,1792,1952,1959,2053,2060` (PRIOR cited 1538-1913, drifted) |
| Pattern: black-box RPC -> real tmux regression test | **Still holds, landed** | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (`server/services/session_service_create_test.go:673`). Deviation: `Command: "bash"` instead of `"claude"` so `printenv` can run; this is why the `--settings` path is not exercised (PIT-F) and why Story 1.1.3 exists. **Lint status: the landed test violates blocking custom analyzers (`norawexec` x3, `notimesleeptest` x2), repaired by Task 1.1.0a** |
| Pattern: table-driven across the wire call sites | **No longer holds** | Only `wireTmuxSession` calls `SetExtraEnv`; sites share one choke point. Replaced by the argv-layer table (Epic 1.2) |
| Pattern: spy collaborator `spyCommandRunner` for `-e` argv | **Partly holds** | Technique holds; the seam is wrong. New-session goes through `cmdExec`, so use `MockCmdExec` (Story 1.2.2) |
| Pattern: Jest fixture strengthening | **Still holds, not done** | `web-app/src/lib/hooks/useAvailablePrograms.test.ts` still uses `id: "aider", command: "aider"`. Carried as Story 1.3.3 (new `it`, not a fixture edit) |
| Tech debt: reuse guard is intentional | **Still holds** | See Tech Debt Disposition |
| Tech debt: duplicate `-e` loops deferred | **Still holds** | Re-verified duplicated at HEAD |
| Migration N/A, Risk Control (revert test commits) | **Still holds** | |
| Observability: "no new logging" | **Holds for this item, incomplete for the feature** | PIT-A found existing INFO logs that leak env values; carried as F1 |
| Unresolved: multi-process config-dir divergence | **Still holds, out of scope** | ARCH-2 |
| Epic 1.1 (Stories 1.1.1 a-c) | **Landed** | `aac276425`, hardened by `1b82f2889` (fails instead of skipping on `Stopped`; uses `tmux.Binary()`) |
| Task 1.1.1d (red/green proof) | **Not recorded; carried** | No pasted output exists anywhere in the repo. Re-done as Story 1.1.2 with the overlay mechanism. PRIOR's rule "whole-file revert is invalid" still holds and is stronger now (squash commit) |
| Story 1.2.1 (resume picks up env change, real tmux pause/resume) | **No longer holds as written** | Premise works at resolver level; real pause/resume is a heavy, flake-prone test for a path that shares `wireTmuxSession`. Replaced by Story 1.3.2 (unit-level characterization). Real-tmux variant DEFERRED, folded into F2 |
| Story 1.2.2 (`IsBackendProcessAlive()`-only reuse) | **DEFERRED** | Guard unchanged since PRIOR; case is about liveness, not env; adds a near-duplicate under the `dupl` gate. Not an env finding |
| Story 1.2.3 (argv for `start()` and `newSessionArgs`) | **Still holds; carried** | Epic 1.2, retargeted: `start()` does not call `newSessionArgs`, so the test drives `Start`/`RestoreWithWorkDir` via `MockCmdExec` as `tmux_ownership_test.go` does |
| Story 1.2.4 (`wrapRemoteCommand` keeps `-e`) | **No longer holds in value; DEFERRED to F4** | Remote creation emits no `-e` (`tmux_session_start.go:~492`, ARCH-6), so preservation is moot until F4 |
| Epic 1.3 (client sends `p.id`, MCP enum moot) | **Still holds, fixture not done** | `useAvailablePrograms.ts:24` maps `value: p.id` (re-read). MCP `mcpgo.Enum("claude","aider")` claim not re-checked |
| Epic 1.4 (AC4 run-only) | **Still holds; strengthened** | Run-only is kept (Task 1.1.3a) but is no longer the only AC4 proof: Tasks 1.1.3b-d add upstream-doc and end-to-end evidence |
| Suggestions: UX "applied keys" panel; consolidate `-e` loops | **Still deferred** | Out of scope of AC1-AC5 |
| adversarial-review.md / pre-mortem.md | **Lessons carried** | Poll with a deadline (not fixed sleeps); evidence must be real pasted failing output naming the assertion (not a build error); closing `git status --short` check |

---

## Research Findings Disposition

Every finding in `research/pitfalls.md` and `research/architecture.md` appears below. PLANNED = story/task in this plan. DEFERRED = real, held for a follow-up with the reason. OUT-OF-SCOPE = not this feature.

### From `research/pitfalls.md`

| ID | Finding | Disposition | Ref / concrete reason |
|----|---------|-------------|-----------------------|
| PIT-1 | Every `wireTmuxSession` call site is a separate place the bug can live | **PLANNED** (resolved structurally, covered once) | Only one `SetExtraEnv` caller (`instance_tmux.go:795`); 8 sites share it. Fresh-create proven by Story 1.1.1/1.1.2; argv layer by Epic 1.2. Per-site tests DEFERRED: every site calls the same function with the raw program ID, so they add no new failure shape |
| PIT-2a | Independent `Snapshot()`/`LoadConfig()` calls per launch can disagree | **DEFERRED -> F2** | Production refactor in a hot file; window is one concurrent `UpsertProgramConfig` inside a single launch (no occurrence reported) |
| PIT-2b | Field-name mismatch `Env` -> `EnvVars` | **Resolved, no action** | Real RPC passes `Env:` end to end in `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`; evidence in Task 1.1.1a |
| PIT-2c | Config loaded from a different scope than the writer | **OUT-OF-SCOPE** (single-process proven) | Dir resolution is process-global; test shares one dir via `envtest.NewIsolatedStateDir`. A multi-process divergence is a workspace/instance concern, same stance as PRIOR |
| PIT-2d | Silent swallow on marshal-failure / empty `extraEnv` | **OUT-OF-SCOPE** | `json.Marshal` of `map[string]map[string]string` cannot fail; `extraEnv` always has `STAPLER_SESSION_UUID` when UUID set |
| PIT-2e | Reuse (live session, same-name tmux session) skips `-e`, so edited env is stale | **PLANNED (documented + pinned)** | Behaviour documented in the test's doc comment (Task 1.1.1b); pinned by existing `TestStart_OwnerMatch_ReusesWithoutKilling` (`tmux_ownership_test.go:104`) and `TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed`. Changing it is F2 |
| PIT-3a | `-e` is argv, not shell-parsed; remote runner quotes per element | **PLANNED (local)** / **DEFERRED -> F4 (remote)** | Local: Tasks 1.2.1a, 1.2.2a. Remote end to end: no `-e` emitted remotely at all (ARCH-6) |
| PIT-3b | `update-environment`/global env could beat `-e` | **Resolved, no action** | Research ran tmux 3.6a on a private socket: explicit `-e` won. One tmux version only; CI pins 3.4 |
| PIT-4 | #852 is precedence, not injection; do not extend `claudeSettingsEnvOverrideArgs` | **PLANNED** | No production edit; AC4 evidence Story 1.1.3 |
| PIT-5 | Unit tests on `&Instance{}` literals miss wiring | **PLANNED** | Integration test exists; Task 1.1.2b shows unit tests stay green under the pre-fix overlay while the integration test fails, which is the proof of why both are needed; argv capture is Epic 1.2 |
| PIT-A | Env values leak via the launch command into INFO logs, persisted `launch_command`, API response; the existing no-leak test exits early | **DEFERRED -> F1** | Production behaviour change on a user-visible and persisted field with a design choice (redact vs 0600 file); redaction is partial because values remain in process argv |
| PIT-B | Two stores of program env; restart semantics differ in-process vs after server restart; `EnvVars` not persisted | **PLANNED (characterize)** / **DEFERRED -> F2 (policy)** | Tests: Tasks 1.3.2a, 1.3.2b. Policy is an owner decision |
| PIT-C | Restore paths wire with the raw ID; relaunch re-resolves command but `-e` is frozen at wire time | **PLANNED (characterize at tmux layer)** / **DEFERRED -> F2** | Task 1.2.2a's `RestoreWithWorkDir` subtest pins that the recreate path emits the wire-time `extraEnv`; making it re-resolve is F2 |
| PIT-D(i) | User key `STAPLER_SESSION_UUID` appended later and wins in tmux, breaking the ownership check | **PLANNED (verify)** / **DEFERRED -> F3 (fix)** | Task 1.4.1a turns "later `-e` wins" from INFERRED to VERIFIED. Stripping keys is a behaviour change (F3) |
| PIT-D(ii) | `CLAUDECODE=` hard-coded first; a user key overrides it | **DEFERRED -> F3** | Same reason; Task 1.2.1a asserts `-e CLAUDECODE=` comes first so the current ordering is pinned |
| PIT-D(iii) | `ExtraEnv` and `extraEnv` both applied; a second writer is silent | **PLANNED** | Task 1.2.1a pins both emitted, `ExtraEnv` first |
| PIT-E | Shell quoting of `--settings`: existing test breaks on `'`; no hostile-value or newline cases | **PLANNED** | Task 1.3.1a |
| PIT-E-len | Large `--settings` payload spends the ~16KB tmux command budget | **DEFERRED** | Env maps are small; prompt-size case is already guarded by `instance_tmux_command_length_test.go`. No evidence of a large env |
| PIT-E-utf8 | Non-UTF-8 bytes in a value | **OUT-OF-SCOPE** | `json.Marshal` replaces invalid bytes with U+FFFD; values arrive as proto strings (UTF-8 by definition) |
| PIT-F | Claude precedence relies on an in-repo comment; managed settings still outrank `--settings`; regression test uses `bash` so `--settings` is not exercised | **PLANNED** (precedence evidence + e2e) / **OUT-OF-SCOPE** (managed-settings detection) | Tasks 1.1.3b (quote upstream doc), 1.1.3c-d (real run through tmux). Detecting org-managed override is Claude Code behaviour we cannot observe |
| PIT-G | Regression-test flakiness: raw sleeps, rc-file masking, 30 s fixed poll, environment-sensitive skip | **PLANNED** | Tasks 1.1.0a (lint conversion: `safeexec`, `wait.RequireEventually`; also removes the raw sleeps), 1.1.1b (probe key, defensive only: rc-file masking is SPECULATIVE; the adversarial reviewer's overlay-red run printed `ENVPROBE__END` for `ANTHROPIC_BASE_URL` on this machine, i.e. no masking was observed), 1.1.1d (`-count=5 -race`). Fail-not-skip on `Stopped` is kept |
| PIT-R1..R5 | Recommendations 1-5 (resolve once; restart policy; redact; protect reserved keys; add tests) | **PLANNED** for 5; **DEFERRED -> F2, F2, F1, F3** for 1-4 | As above |

### From `research/architecture.md`

| ID | Finding | Disposition | Ref / concrete reason |
|----|---------|-------------|-----------------------|
| ARCH-1 | `buildExtraEnv` runs before `new-session` (wire -> SetSession -> Start) | **PLANNED (evidence)** | Held by `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`; red under overlay (Task 1.1.2b) |
| ARCH-2 | `LoadConfig` reads the dir the API writes: single process only; preference-file `SwitchDatabase` can change it at runtime | **OUT-OF-SCOPE** | Process-global inputs; the switch is workspace semantics, not injection |
| ARCH-3 | Every local start/restart/revive path funnels through `wireTmuxSession`; relaunch of a confirmed-missing pane re-resolves command but keeps frozen env | **PLANNED (characterize)** / **DEFERRED -> F2** | See PIT-C. "VERIFIED by code reading; not executed" for the relaunch path stays labelled as such |
| ARCH-4 | Other `NewTmuxSession*` constructions without `SetExtraEnv` (`backend_factory.go:110,116`; remote pre-creation; `*FromExisting*`) | **OUT-OF-SCOPE** | `backend_factory` placeholder is overwritten by `wireTmuxSession` (INFERRED from the doc comment); `*FromExisting*` never spawns |
| ARCH-5 | tymux backend skips `wireTmuxSession`'s tmux path; env INFERRED not injected | **DEFERRED -> F5** | Opt-in per-session backend; needs a running tymuxd to confirm, so it starts as an investigation |
| ARCH-6 | Remote execution target: `createRemoteSession` argv has no `-e`; `STAPLER_SESSION_UUID` also never stamped; program is the pre-`buildLaunchCommand` value | **DEFERRED -> F4** | Requirements question unanswered; new SSH code path; remote tmux version unknown |
| ARCH-7 | `ExtraEnv`/`extraEnv` two-field trap; `ExtraEnv` accumulates on a reused session | **PLANNED (pin)** / **OUT-OF-SCOPE (rename)** | Task 1.2.1a; rename is cosmetic, duplicates harmless |
| ARCH-8 | PRIOR findings vs current code (line drift; redundant merge in `session_service_create.go:282` and `instance_tmux.go:715`; client program-ID not re-checked) | **PLANNED** (reconciled above; client check = Story 1.3.3) / **DEFERRED -> F2** (redundant merge is part of the frozen-copy question) | Prior Plan Reconciliation |
| ARCH-9 | Hotspot coverage: file listed in the 2026-07-01 and 2026-09-07 audits; no proposal targets env injection; disposition Extend as-is | **PLANNED** | Tech Debt Disposition table |
| ARCH-Q1 | Is remote in scope? | **DEFERRED** (Unresolved Questions) | Owner decision, F4 |
| ARCH-Q2 | Do tymux sessions need program env? | **DEFERRED** (F5) | |
| ARCH-Q3 | Unify `ExtraEnv`/`extraEnv`? | **OUT-OF-SCOPE** | Cosmetic |

### From `research/stack.md` and `research/build-vs-buy.md` (checked for completeness)

| ID | Finding | Disposition |
|----|---------|-------------|
| STK-1 | tmux minimum version for `-e` (3.2) unverified | **OUT-OF-SCOPE**: pinned 3.4 and local 3.6a both exceed it; no version gate exists to change |
| STK-2 | Map iteration order makes `-e` argv order nondeterministic | **PLANNED**: Task 1.2.2a asserts pairs and adjacency, never global order; Task 1.2.1a uses explicit slices |
| STK-3 | `${VAR}` unset in server env silently drops the key | **OUT-OF-SCOPE**: already covered by `config/defaults_test.go:395` (`TestExpandEnvVars_OmitsKey_WhenVarNotSetInEnvironment`); Task 1.3.1a deliberately uses instance-level values so expansion does not interfere |
| STK-4 | Second `LoadConfig` in `claudeSettingsEnvOverrideArgs` could diverge | **DEFERRED -> F2** (same as PIT-2a) |
| BVB-1 | Build nothing, buy nothing; reuse existing helpers (`envtest`, `createTestStorage`, `tmux.Binary()`) | **PLANNED**: all new tests reuse them |
| BVB-2 | Optional hostile-value table test | **PLANNED**: Task 1.3.1a |
| BVB-3 | Dedupe the two `shellQuote` copies | **OUT-OF-SCOPE** (Tech Debt Disposition) |

---

## Phase 1: Evidence and Coverage

### Epic 1.1: Close the open acceptance criteria with recorded evidence

**Goal**: AC1 and AC4 closed with commands and output in `evidence.md`; AC3 shown red on pre-fix behaviour and green on HEAD; AC5 tied to the verified pre-fix line. **This epic is the first shippable unit** (AC evidence plus the lint repair of the landed test); supporting epics can be rejected without touching the AC proof.

**Contingency**: if Task 1.1.2b shows the integration test passing under `PreFixOverlay`, halt: the assertion is vacuous and the item reverts to root-cause analysis. If Task 1.1.3d shows `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess` passing with the override disabled, halt and fix the test before relying on it.

#### Story 1.1.0: The landed regression test passes the repo's blocking custom linters
**As a** maintainer, **I want** `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` to satisfy `norawexec` and `notimesleeptest`, **so that** AC3's committed regression test survives CI (`make lint-custom` runs `bin/linter ./...` repo-wide, `.github/workflows/lint.yml:217`, not new-code-only).
**Acceptance Criteria**:
- *Given* the HEAD test file, *When* the custom linter runs on `./server/services`, *Then* it currently reports 5 findings in `server/services/session_service_create_test.go` (VERIFIED by the adversarial review: `norawexec` at 729, 738, 741; `notimesleeptest` at 718, 740); after this story it reports none.
**Files**: `server/services/session_service_create_test.go`

##### Task 1.1.0a: Convert raw `exec` and `time.Sleep` to the approved helpers (S, test edit)
- Open `tools/lint/norawexec/analyzer.go` and `tools/lint/notimesleeptest/analyzer.go` (read this run): `norawexec` requires `safeexec.CommandContext` (`github.com/tstapler/stapler-squad/executor/safeexec`, sets `WaitDelay`; already imported by sibling tests in this package, e.g. `connectrpc_websocket_test.go:24`) or a `//nolint:norawexec <reason>`; `notimesleeptest` (ADR-003, `docs/adr/003-no-static-sleeps-in-tests.md`) rejects `time.Sleep` in `_test.go` files, pointing at fake clocks, channels, `require.Eventually` or the repo helper `wait.RequireEventually` (`testutil/wait/eventually.go:20`).
- Add the `github.com/tstapler/stapler-squad/executor/safeexec` import to this file (it is not imported here yet). Replace the three `exec.CommandContext` calls (lines ~729, 738, 741) with `safeexec.CommandContext` (same signature); keep the `os/exec` import because `exec.LookPath` still needs it.
- Replace the line-~718 Active poll with `wait.RequireEventually` here (so this task's own `bin/linter` verify passes; Task 1.1.1c then only tunes its timeout via `wait.ScaleTimeout`) and the line-~740 send-keys retry: put the `send-keys` + `capture-pane` into a `wait.RequireEventually` condition (one side effect per tick is fine) with a 15 s base timeout and 300 ms tick, so no `time.Sleep` remains. This supersedes the earlier "keep the send-keys retry loop as is".
- Rule for every new test in this plan (Tasks 1.1.3c, 1.2.1a, 1.2.2a, 1.3.1a, 1.3.2a/b): subprocesses use `safeexec.CommandContext`; no `time.Sleep`; wait with `wait.RequireEventually`.
- Files: `server/services/session_service_create_test.go`
- Verify: `go -C tools/lint build -o "$(pwd)/bin/linter" ./cmd/linter && bin/linter ./server/services ./session ./session/tmux` exits 0 (the Makefile's `lint-custom` recipe, `Makefile:814-821`, scoped to the touched packages; `make lint-custom` is the repo-wide form), then `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1`. Re-run the linter line after every later task that adds a test file.

#### Story 1.1.1: AC1 / AC2 hold at HEAD and the test cannot be masked or flake on a loaded runner
**As a** maintainer, **I want** the existing regression test run, recorded, and made hermetic, **so that** AC1 and AC2 rest on evidence rather than on "it passed once".
**Acceptance Criteria**:
- AC1: a `SESSION_TYPE_NEW_WORKTREE` session created with a custom program shows the registered env var via `printenv` in the pane.
  - *Given* a `ProgramConfig{ID: "netflix-model-gateway", Command: "bash", Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000", "SSQ_PROGRAM_ENV_PROBE": "probe-7f3a91"}}` registered through `UpsertProgramConfig`, *When* `CreateSession` runs with `SessionType: SESSION_TYPE_NEW_WORKTREE, Program: "netflix-model-gateway"` and the instance reaches `Active`, *Then* `echo ENVPROBE_$(printenv SSQ_PROGRAM_ENV_PROBE)_END` sent to the pane captures `ENVPROBE_probe-7f3a91_END`.
- AC2: `tmux show-environment -t <session>` includes the var.
  - *Given* the same session, *When* `tmux -L <svc.testTmuxServerSocket> show-environment -t <tmux name>` runs, *Then* the output contains `ANTHROPIC_BASE_URL=http://127.0.0.1:47000` and `SSQ_PROGRAM_ENV_PROBE=probe-7f3a91`.
- The test passes five consecutive runs.
  - *Given* the hardened test, *When* run with `-count=5`, *Then* all five report `PASS`.
**Files**: `server/services/session_service_create_test.go`, `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.1.1a: Record the HEAD baseline (E1) (XS, run-only)
- Run `go test ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v` and `go test ./session -run 'EnvOverride|ExtraEnv|SettingsEnv' -count=1 -v`.
- Create `evidence.md`; section E1 holds `git rev-parse HEAD`, `tmux -V`, the `tmux.Binary()` path, both commands and verbatim output.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: both commands exit 0; E1 contains two `PASS`/`ok` lines.

##### Task 1.1.1b: Add the unique `ProbeKey` and document scope limits (XS, test edit)
- In `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`, add `SSQ_PROGRAM_ENV_PROBE` (value `probe-7f3a91`) to the program `Env`; point the in-pane `ENVPROBE_` probe at it. Keep the `show-environment` assertion for both `ANTHROPIC_BASE_URL` and the probe key.
- Extend the doc comment with a "Scope and limits" paragraph: `-e` applies only at `new-session`; a live pane or a same-name tmux session that is reused (`tmux_session_start.go:~189-196`, `instance_tmux.go:807-810`) keeps its old env until killed and recreated; the Claude `--settings` path is covered by `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess`, not by this `bash` test.
- Reason: defensive only. A user `~/.bashrc` exporting `ANTHROPIC_BASE_URL` could in principle mask the pane probe; no rc file sets `SSQ_PROGRAM_ENV_PROBE`. SPECULATIVE: the adversarial reviewer's overlay-red run on this machine showed no rc masking, and no real occurrence is cited.
- Files: `server/services/session_service_create_test.go`
- Verify: `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1`

##### Task 1.1.1c: Replace the hand-rolled Active poll with `wait.RequireEventually` (XS, test edit; same agent as Task 1.1.0a)
- Replace the `for time.Now().Before(deadline) ... time.Sleep(100ms)` loop (test lines 716-719) with `wait.RequireEventually(t, cond, 30*time.Second, 100*time.Millisecond, "session must reach Active")`; inside `cond`, call `t.Fatalf` when status is `session.Stopped` so a real spawn failure still fails fast (not skips). `testutil/wait` is already imported in this file (`wait.WaitForCondition`, line ~649).
- Reason: the 30 s bound is a fixed wall-clock wait; `wait.ScaleTimeout` stretches it under machine load (`testutil/wait/load.go:31-43`, BUG-103) and the repo's `fix-flaky-tests-dont-defer` / `deterministic-fast-tests` skills prefer it. The send-keys retry loop is converted in Task 1.1.0a (lint requires it).
- Files: `server/services/session_service_create_test.go`
- Verify: `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 && go vet ./server/services && bin/linter ./server/services`

##### Task 1.1.1d: Repeat-run flake evidence (E2) (XS, run-only)
- Run `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=5 -v` (CI uses `-race -short` with `TMUX_BIN="$(pwd)/bin/tmux"`, `.github/workflows/build.yml:275,327`; set `TMUX_BIN` the same way); paste output and per-run durations into E2.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: five `--- PASS` lines in E2. A single failure is a defect to fix, not to re-run.

#### Story 1.1.2: AC3 and AC5 - the test fails on pre-fix behaviour and passes on HEAD
**As a** maintainer, **I want** an executed red/green proof against the pre-`cdfd4e5cf2` behaviour, **so that** "regression test fails pre-fix" is evidence, not a claim.
**Acceptance Criteria**:
- AC3: a committed regression test fails on the pre-fix behaviour and passes on HEAD.
  - *Given* the `PreFixOverlay` (`wireTmuxSession` calls `SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})` only) and `ProgramConfig{ID: "netflix-model-gateway", Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}`, *When* `go test -overlay <json> ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$'` runs, *Then* it fails with the assertion message `tmux show-environment must carry the program's env`, and the same command without `-overlay` passes.
- AC5: root cause documented with the failure mechanism.
  - *Given* `git show cdfd4e5cf2^:session/instance_tmux.go`, *When* its `SetExtraEnv` call is inspected, *Then* it shows `if i.UUID != "" { session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID}) }` (line 578-580) and no `resolveExtraEnvVars`/`buildExtraEnv` symbol exists in that file, matching `requirements.md`'s Root cause section and the test's doc comment.
**Files**: scratchpad overlay files (never committed), `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.1.2a: Build the `PreFixOverlay` and prove it matches the historical shape (E3) (XS)
- `cp session/instance_tmux.go <scratchpad>/instance_tmux.prefix.go`; in the copy's `wireTmuxSession` replace `if extraEnv := i.buildExtraEnv(); len(extraEnv) > 0 {` with a block that builds a slice containing only `"STAPLER_SESSION_UUID="+i.UUID` when `i.UUID != ""` (use the Edit tool on the copy, never on the tracked file). Scratch only: the pre-fix shape reads `i.UUID` raw; production must keep `Snapshot()` per `.claude/rules/instance-lock-free-reads.md`, so never paste it back. Write `<scratchpad>/overlay.json` as `{"Replace": {"<abs worktree>/session/instance_tmux.go": "<scratchpad>/instance_tmux.prefix.go"}}`.
- Equivalence check, not from memory: `git show cdfd4e5cf2^:session/instance_tmux.go | grep -n 'SetExtraEnv' -B2 -A1` shows the historical block; paste both into E3.
- Files: scratchpad only.
- Verify: `go build -overlay <scratchpad>/overlay.json ./session` exits 0 and `git status --short` prints nothing.

##### Task 1.1.2b: Run red under the overlay (E4) (S, run-only)
- `go test -overlay <scratchpad>/overlay.json ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v`; expect `FAIL` naming `tmux show-environment must carry the program's env` (an assertion, not a build error; a compile error means the overlay is wrong and the step does not count).
- Also `go test -overlay <scratchpad>/overlay.json ./session -run '^TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars$' -count=1`: expect `PASS`. Record in E4 that the resolver unit test stays green while the integration test is red; this is the PIT-5 proof.
- E4b (corroborating, already executed by the coordinator; paste the record, do not re-run unless cheap): on a detached worktree at `cdfd4e5cf2^` (`git rev-parse cdfd4e5cf2^` = `4dbbe7b40`) with the HEAD regression test appended, `go test ./server/services -run TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` FAILED on `tmux show-environment must carry the program's env` with only `STAPLER_SESSION_UUID` present; the in-pane `printenv` assertion also failed there ("Pane is dead (status 127)"). Setup required: regenerate protos with connect-go pinned to v1.19.1, and ent with `--feature sql/upsert`. This is the literal reading of AC3 ("fails on the pre-fix commit"); the overlay (E4) is the repeatable form.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: E4 contains the verbatim `--- FAIL` output with the assertion text. If the integration test passes, stop (Contingency). E4b contains the commit hash and the verbatim failure.

##### Task 1.1.2c: Green on HEAD and clean tree (E5) (XS)
- Re-run the same command with no `-overlay`; expect `PASS`. Then `git status --short` and `git diff --stat` must be empty for tracked files; delete the scratch overlay files.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: `go test ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1` exits 0; E5 shows empty `git status --short`.

#### Story 1.1.3: AC4 - `--settings` env override delivery shown end to end; precedence over `settings.json` is upstream-documented
**As a** maintainer, **I want** the `--settings` override proven through real tmux and a real shell, plus the upstream precedence rule quoted, **so that** AC4 is not just a string comparison.
**Acceptance Criteria**:
- AC4 (stated honestly): no regression to `claudeSettingsEnvOverrideArgs()`. Provable locally: the `--settings <env JSON>` flag reaches the launched process argv through real tmux and a real shell, with hostile values intact. NOT provable locally: that program env wins over a global `~/.claude/settings.json` `env` block is upstream Claude Code behaviour (Task 1.1.3b quotes the doc); nothing in 1.1.3a-d executes it, so it is UNVERIFIED locally.
  - *Given* `ProgramConfig{ID: "netflix-model-gateway", Command: "<tmp>/claude" (FakeClaude), Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000", "SSQ_HOSTILE": "it's $(touch <tmp>/pwned) `touch <tmp>/pwned` \"q\" a=b"}}`, *When* `CreateSession` launches it and the fake records its argv, *Then* the argv holds `--settings` followed by JSON equal to `{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:47000","SSQ_HOSTILE":"it's $(touch <tmp>/pwned) `touch <tmp>/pwned` \"q\" a=b"}}`, `<tmp>/pwned` does not exist, and `tmux show-environment` carries both keys.
  - *Given* the same program but `claudeSettingsEnvOverrideArgs` disabled by overlay, *When* the test runs, *Then* it fails at the `--settings` assertion.
- The "wins over settings.json" half is Claude Code behaviour: recorded as a quoted upstream rule (Task 1.1.3b), and remains UNVERIFIED locally even if the page is fetched (a doc quote is not an execution); labelled UNVERIFIED outright if the page cannot be fetched.
**Files**: `server/services/session_service_create_settings_env_test.go` (new), `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.1.3a: Run the existing #852 tests (E6) (XS, run-only)
- `go test ./session -run 'TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars|TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars|TestBuildClaudeCommand_IncludesSettingsEnvOverride|TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars' -count=1 -v`
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: four `--- PASS` lines in E6.

##### Task 1.1.3b: Quote the upstream precedence rule (E7) (XS, research)
- Read https://code.claude.com/docs/en/settings with `mcp__stapler-mcp__read_website` (save to scratchpad if large). Quote the lines giving the order of command-line arguments (`--settings`), user settings, project settings and managed settings, with URL and retrieval date. If the fetch fails, write "UNVERIFIED" and keep the in-repo comment (`instance_tmux.go:741-755`) as the only source.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: E7 contains a quoted ordering or the literal word `UNVERIFIED`.

##### Task 1.1.3c: Add `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess` (S, test-first)
- New file `server/services/session_service_create_settings_env_test.go`. Reuse `newCreateTestService`, `createTestStorage`, `initGitRepoWithCommit`, `destroyCreatedSession` (all in the same package).
- Setup (Task 1.1.0a rules apply: `safeexec.CommandContext`, no `time.Sleep`): follow the existing LookPath-skip convention (skip when `exec.LookPath(tmux.Binary())` fails; fail, not skip, once tmux exists but the session never reaches `Active`); `envtest.NewIsolatedStateDir(t)` (makes `config.IsIsolatedInstance()` true so `trustStore()` is in-memory and `~/.claude.json` is untouched, `session/claude_trust.go:165`); `t.Setenv("HOME", t.TempDir())` as defence.
- `FakeClaude`: write `<tmp>/claude` mode 0755: `#!/bin/sh`, write `printf '%s\n' "$@"` to `<out>.tmp` then `mv` to `<out>`, then `exec sleep 300`. Register program `Command: <tmp>/claude` with the two env keys above (hostile value includes `'`, `$(...)`, backticks, `"`, `=`); create a `SESSION_TYPE_NEW_WORKTREE` session.
- Wait with `wait.RequireEventually` for `<out>`; assert: argv contains `--settings`; the next element unmarshals to `map[string]map[string]string` equal to the registered env; `<tmp>/pwned` absent; `show-environment` contains `SSQ_HOSTILE=` and `ANTHROPIC_BASE_URL=`.
- Feasibility: confirmed by the adversarial review's scratch spike (experiment 3; see closed Unresolved Question). Expect a harmless `ERROR claude launch: MCP server URL unresolved` log.
- Test-first order: write the test, confirm it fails under the V-settings overlay (Task 1.1.3d) before treating it as done.
- Files: `server/services/session_service_create_settings_env_test.go`
- Verify: `go test -race -short ./server/services -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$' -count=1 -v && bin/linter ./server/services`

##### Task 1.1.3d: Overlay red/green for the AC4 test (E8) (XS)
- Same mechanism as Task 1.1.2a, second overlay: the copy's `claudeSettingsEnvOverrideArgs` body replaced with `return "", ""`. Run Task 1.1.3c's command with `-overlay`: expect `FAIL` at the `--settings` assertion; re-run without: `PASS`; `git status --short` empty.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: E8 holds both outputs.

---

### Epic 1.2: `-e` flags survive both tmux argv-construction paths

**Scope label**: AC-less hardening (supporting, from research); may land in a separate PR from Epic 1.1.

**Goal**: unit-level proof, with no tmux binary, that every `SetExtraEnv`/`ExtraEnv` entry becomes a `-e KEY=VALUE` argv pair in both `start()` and the recreate path (`newSessionArgs`). Closes the "fake-runner argv capture not implemented" gap (PIT-5) and pins PIT-C and PIT-D(ii)-(iii).

#### Story 1.2.1: `newSessionArgs` emits `ExtraEnv` and `extraEnv` pairs in a fixed order
**As a** maintainer, **I want** a table test on `newSessionArgs`, **so that** a future edit cannot drop, reorder or shell-split env pairs unnoticed.
**Acceptance Criteria**:
- `-e CLAUDECODE=` is first; `ExtraEnv` pairs precede `extraEnv` pairs; a value with spaces, quotes, `$` and `=` stays one argv element.
  - *Given* a `TmuxSession` named `args-test` with `ExtraEnv = ["DISPLAY=:99"]` and `SetExtraEnv(["STAPLER_SESSION_UUID=11111111-1111-1111-1111-111111111111", "K=a b 'c' $D =x"])`, *When* `newSessionArgs("/w", "prog")` is called, *Then* it returns `["new-session","-d","-s","staplersquad_args-test","-e","CLAUDECODE=","-e","DISPLAY=:99","-e","STAPLER_SESSION_UUID=11111111-1111-1111-1111-111111111111","-e","K=a b 'c' $D =x","-c","/w","prog"]` (session name per the constructor's sanitising; assert against `s.sanitizedName`).
**Files**: `session/tmux/new_session_args_extra_env_test.go` (new)

##### Task 1.2.1a: Add `TestNewSessionArgs_EmitsExtraEnvPairs` (XS, test-first)
- Table cases: none; `ExtraEnv` only; `extraEnv` only; both (order); hostile value as one element. Build with `NewTmuxSessionWithDeps("args-test", "prog", nil, MockCmdExec{})`; set `s.ExtraEnv` directly and `s.SetExtraEnv(...)`. Compare the whole slice with `require.Equal`. `t.Parallel()` is safe (no env or filesystem).
- Files: `session/tmux/new_session_args_extra_env_test.go`
- Verify: `go test -race -short ./session/tmux -run '^TestNewSessionArgs_EmitsExtraEnvPairs$' -count=1 && bin/linter ./session/tmux`

#### Story 1.2.2: `Start` and the missing-session recreate path both put the wire-time env into `new-session`
**As a** maintainer, **I want** the executed argv captured on both paths, **so that** the duplicated inline loop in `start()` is covered before anyone consolidates it.
**Acceptance Criteria**:
- Both creation paths pass the same pairs to the executor.
  - *Given* a `TmuxSession` built with `newTmuxSessionWithSocket("argv-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))` and `SetExtraEnv(["STAPLER_SESSION_UUID=22222222-2222-2222-2222-222222222222", "ANTHROPIC_BASE_URL=http://127.0.0.1:47000"])`, *When* `Start(t.TempDir())` runs against a `cmdExec` that reports no existing session, *Then* the captured `new-session` argv contains the adjacent pairs `-e`,`ANTHROPIC_BASE_URL=http://127.0.0.1:47000` and `-e`,`STAPLER_SESSION_UUID=22222222-2222-2222-2222-222222222222` after `-e`,`CLAUDECODE=`; *and* *When* `RestoreWithWorkDir(t.TempDir())` runs against the same kind of `cmdExec`, *Then* the captured argv contains the same pairs.
**Files**: `session/tmux/new_session_argv_creation_paths_test.go` (new)

##### Task 1.2.2a: Add `TestNewSessionArgv_BothCreationPaths_CarryExtraEnv` (S, test-first)
- Two subtests `Start` and `RestoreWithWorkDir_MissingSession`, driven by one helper `newSessionCaptureExec(sessionName) (MockCmdExec, func() []string)` that records `cmd.Args` of the invocation containing `new-session` and flips `list-sessions` output from "no server" to the session name afterwards (model on `ownerMismatchFixture`, `tmux_ownership_test.go:25-65`, and `createMockExecutorForMissingSession`, `session_recovery_test.go:349`).
- The recording closure is invoked from helper goroutines (the `ownerMismatchFixture` bools it is modelled on are unsynchronised and only get away with it): guard the captured argv slice with a `sync.Mutex` and return a copy from the getter, so `-race` stays clean.
- Cost note: the `RestoreWithWorkDir` recreate-path subtest spends about 1.5 s in `probeSessionExistsWithRetries` backoff (100+200+400+800 ms) with a mock executor; acceptable.
- Assertion helper `requireArgPair(t, args, "-e", kv)` checks adjacency; never assert the order of program-env pairs relative to each other (STK-2).
- Files: `session/tmux/new_session_argv_creation_paths_test.go`
- Verify: `go test -race -short ./session/tmux -run '^TestNewSessionArgv_BothCreationPaths_CarryExtraEnv$' -count=1 -v && bin/linter ./session/tmux`

---

### Epic 1.3: Session-layer coverage and client invariant

**Scope label**: AC-less hardening (supporting, from research); may land in a separate PR from Epic 1.1.

**Goal**: execute the one hand-rolled piece (`shellQuote` feeding `--settings`), pin the frozen-vs-reloaded behaviour as characterization, and lock the client's program-ID mapping.

#### Story 1.3.1: Hostile env values survive `--settings` through a real shell
**As a** maintainer, **I want** the `--settings` value round-tripped through `sh -c`, **so that** quoting is proven for values that break the existing prefix/suffix test.
**Acceptance Criteria**:
- *Given* an `Instance{EnvVars: {"V": "it's"}}` and further cases `$(touch <tmp>/pwned)`, `` `touch <tmp>/pwned` ``, `say "hi"`, `a=b=c`, `with space`, `line1\nline2`, `back\\slash`, `${HOME}`, *When* `claudeSettingsEnvOverrideArgs()` output is executed as `sh -c 'printf %s ' + val`, *Then* stdout unmarshals to `{"env":{"V": <original>}}` byte for byte and `<tmp>/pwned` is never created.
**Files**: `session/instance_tmux_test.go`

##### Task 1.3.1a: Add `TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell` (S, test-first)
- Table-driven with `t.Run` per value; use `safeexec.CommandContext` (already imported in this file). Use instance-level `EnvVars` so `config.ExpandEnvVars` does not rewrite `${HOME}`; set `t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())` as the sibling tests do.
- Files: `session/instance_tmux_test.go`
- Verify: `go test -race -short ./session -run '^TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell$' -count=1 -v && bin/linter ./session`

#### Story 1.3.2: The frozen-vs-reloaded env behaviour is pinned and named
**As a** maintainer, **I want** the current behaviour captured as tests labelled "characterization", **so that** changing the restart policy (F2) is a deliberate edit that flips named tests.
**Acceptance Criteria**:
- In-process: the create-time copy wins over a later program edit.
  - *Given* a `ProgramConfig{ID: "claude-250k-proxy", Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}` and an `Instance{Program: "claude-250k-proxy", EnvVars: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}`, *When* the program's `Env` is changed to `http://127.0.0.1:47001` and `resolveExtraEnvVars()` is called, *Then* the result is still `http://127.0.0.1:47000`.
- Reloaded: an instance without `EnvVars` sees the current program env.
  - *Given* the same edited program and `Instance{Program: "claude-250k-proxy"}` (no `EnvVars`), *When* `resolveExtraEnvVars()` is called, *Then* the result is `http://127.0.0.1:47001`.
- `EnvVars` is not persisted.
  - *Given* an `Instance{Title: "envvars-roundtrip", Path: "/path/to/repo", Status: Paused, Program: "bash", EnvVars: {"K": "v"}}`, *When* `ToInstanceData()` then `FromInstanceData()` runs, *Then* `restored.Snapshot().EnvVars` is empty.
**Files**: `session/instance_program_env_semantics_test.go` (new)

##### Task 1.3.2a: Add the two resolver characterization tests (S, test-first)
- `TestResolveExtraEnvVars_InstanceEnvVarsCopyShadowsLaterProgramEdit` and `TestResolveExtraEnvVars_InstanceWithoutEnvVarsSeesCurrentProgramEnv`. Use `seedCustomProgram` (`session/instance_tmux_test.go:1190`) once, then load config, modify `Env`, and `SaveConfig` explicitly: `seedCustomProgram` calls `t.Setenv(STAPLER_SQUAD_TEST_DIR, t.TempDir())` on every call, so calling it twice silently swaps in a fresh empty config instead of editing the first. Header comment: "characterization of current behaviour (PIT-B); changing it is follow-up F2". Not `t.Parallel()` (`t.Setenv`).
- Files: `session/instance_program_env_semantics_test.go`
- Note: the in-process half restates the CLASH case of `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` (`instance_tmux_test.go:1202`); keep it only because it is the named characterization that flips under F2.
- Verify: `go test -race -short ./session -run 'TestResolveExtraEnvVars_' -count=1 -v && bin/linter ./session`

##### Task 1.3.2b: Add `TestInstanceData_RoundTripDropsEnvVars` (XS, test-first)
- Same file; mirror `TestToInstanceData_PreservesBackend` (`session/instance_serialization_test.go:15`), plus `envtest.NewIsolatedStateDir(t)` because `FromInstanceData` for a `Paused` instance calls `wireTmuxSession` when the process manager is a `TmuxBackend` (`instance_serialization.go:463-467`), and that calls `config.LoadConfig()`. Read the result through `Snapshot()` per `.claude/rules/instance-lock-free-reads.md`. Name and comment it as characterization of a gap, not a guarantee: because `EnvVars` is not persisted, request-level `env_vars` are silently lost across a server restart.
- Files: `session/instance_program_env_semantics_test.go`
- Verify: `go test -race -short ./session -run '^TestInstanceData_RoundTripDropsEnvVars$' -count=1 && bin/linter ./session`

#### Story 1.3.3: The web client sends the program ID, not its command
**As a** maintainer, **I want** a fixture where `id` and `command` differ, **so that** mapping `value: p.command` is caught (the existing fixture uses `id === command === "aider"`).
**Acceptance Criteria**:
- *Given* `mockList` resolving `{programs: [{id: "netflix-model-gateway", label: "Netflix Model Gateway", description: "", command: "claude", cliFlags: ""}]}`, *When* `useAvailablePrograms()` is rendered and awaited, *Then* `result.current[0].value` is `"netflix-model-gateway"` and `result.current[0].command` is `"claude"`.
**Files**: `web-app/src/lib/hooks/useAvailablePrograms.test.ts`

##### Task 1.3.3a: Add `useAvailablePrograms_should_UseProgramIdNotCommandAsValue_When_IdAndCommandDiffer` (XS, test-first)
- New `it` in the existing `describe`; leave the existing cases untouched. This locks only the hook's mapping; Omnibar `dispatch.ts` was not re-audited in this run.
- Files: `web-app/src/lib/hooks/useAvailablePrograms.test.ts`
- Verify: `cd web-app && pnpm exec jest --testPathPatterns="useAvailablePrograms" --no-coverage`

---

### Epic 1.4: Close out

**Scope label**: Story 1.4.1 and the follow-up write-up (1.4.2a) are AC-less supporting work; Task 1.4.2b's gates apply to whichever PR(s) the work lands in.

**Goal**: convert the one INFERRED tmux fact into evidence, record follow-ups, and run the repo gates.

#### Story 1.4.1: Record the duplicate `-e` precedence result (already observed: later wins)
**As a** maintainer, **I want** the duplicate-key precedence result pasted into evidence, **so that** F3's mechanism rests on evidence.
**Acceptance Criteria**:
- *Given* a private tmux server on socket `ssq-dup-env-probe`, *When* `tmux -L ssq-dup-env-probe -f /dev/null new-session -d -s dup -e K=first -e K=second 'sleep 30'` runs, *Then* `tmux -L ssq-dup-env-probe show-environment -t dup K` prints `K=second` (already observed by the adversarial review on tmux 3.6a: `K=second`, later wins; F3's mechanism note is therefore VERIFIED, not INFERRED).
**Files**: `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.4.1a: Paste the recorded experiment (XS, evidence paste; optional 1-minute re-run)
- Result already known: tmux 3.6a, private socket, `-e K=first -e K=second`, `show-environment -t dup K` printed `K=second`. Paste it with `tmux -V`; re-run only if cheap.
- Use `tmux.Binary()`'s path (honour `TMUX_BIN`); record `tmux -V`; finish with `tmux -L ssq-dup-env-probe kill-server` (touches only that socket, never the live one).
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: the evidence section shows the printed `K=...` line and the kill-server exit status 0.

#### Story 1.4.2: Follow-ups recorded and gates green
**As a** maintainer, **I want** deferred work written down with risks and the final gates run, **so that** the PR is reviewable and nothing is silently dropped.
**Acceptance Criteria**:
- *Given* the Production-Behaviour Decisions table, *When* Task 1.4.2a completes, *Then* `evidence.md` has `### F1` through `### F5`, each with title, risk, and proposed acceptance criteria.
- *Given* all new tests, *When* the Task 1.4.2b commands run, *Then* each exits 0.
**Files**: `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.4.2a: Write the follow-up section (XS)
- Copy F1-F5 from this plan (title, risk, decision) and add a one-line proposed test per item (F1: real create with a secret value, grep captured logs and persisted `LaunchCommand`; F2: flip Story 1.3.2's tests; F3: `buildExtraEnv` with `STAPLER_SESSION_UUID` in `EnvVars`; F4: argv capture on `createRemoteSession`; F5: tymux-backed session `show-environment`). File them via `create_backlog_item` only if the owner answers yes to the first Unresolved Question.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: `grep -c '^### F' project_plans/program-env-injection/implementation/evidence.md` prints 5.

##### Task 1.4.2b: Final gates (S, run-only)
Use the invocations CI uses (`-race -short`, `TMUX_BIN="$(pwd)/bin/tmux"`; `.github/workflows/build.yml:275,327`).
- `go test -race -short ./session/tmux -run 'TestNewSessionArgs_|TestNewSessionArgv_' -count=1`
- `go test -race -short ./session -run 'TestResolveExtraEnvVars_|TestInstanceData_RoundTripDropsEnvVars|TestClaudeSettingsEnvOverrideArgs|TestBuildClaudeCommand_IncludesSettingsEnvOverride|TestInstance_BuildExtraEnv' -count=1`
- `go test -race -short ./server/services -run 'TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession|TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess|TestCreateSession_should_NotLogEnvVarValues_When_EnvVarsProvided' -count=1`
- Custom linters: `make lint-custom` (builds `bin/linter`, runs `bin/linter ./...`, `Makefile:814-817`; the same step CI runs). It can only be green because Task 1.1.0a repaired the landed test; before 1.1.0a it reports 5 findings.
- `make lint` (depends on `lint-custom`).
- `dupl` is moot for new Go tests: `.golangci.yml:244-246` excludes `_test.go` from `gocyclo,gocognit,funlen,revive,dupl`, so `make ready-complexity-gate` needs no table-test refactor. The gate that can bite is web `jscpd`: `cd web-app && pnpm run lint:duplicates` for the added Jest `it` (absolute threshold ratchet, see CLAUDE.md).
- Files: none
- Verify: every command exits 0; output summarized in the PR body. If Epics 1.2-1.4.1 land in a separate PR, run only the lines for the packages each PR touches.

---

## Traceability: AC -> Tasks -> Tests

| AC | Tasks | Tests (exact names) | Evidence section |
|----|-------|---------------------|------------------|
| AC1 pane `printenv` shows the var | 1.1.0a, 1.1.1a, 1.1.1b, 1.1.1c, 1.1.1d | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (`session_service_create_test.go:673`) | E1, E2 |
| AC2 `tmux show-environment` includes the var | 1.1.1a, 1.1.1b, 1.2.1a, 1.2.2a | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`; `TestNewSessionArgs_EmitsExtraEnvPairs`; `TestNewSessionArgv_BothCreationPaths_CarryExtraEnv` | E1, E2 |
| AC3 regression test fails pre-fix, passes on HEAD (and passes the custom linters) | 1.1.0a, 1.1.2a, 1.1.2b, 1.1.2c | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (red under `PreFixOverlay`); `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` (stays green, PIT-5) | E3, E4, E4b, E5 |
| AC4 no regression of the `--settings` override: flag delivery + hostile-value integrity VERIFIED locally; precedence over global `settings.json` is the upstream-documented rule, UNVERIFIED locally | 1.1.3a, 1.1.3b, 1.1.3c, 1.1.3d, 1.3.1a | `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`; `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars`; `TestBuildClaudeCommand_IncludesSettingsEnvOverride`; `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess`; `TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell` | E6, E7, E8 |
| AC5 root cause documented with mechanism | 1.1.2a (historical diff), 1.1.1b (doc comment) | n/a (documentation). Doc comment above `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`; `requirements.md` Root cause | E3 |
| Supporting (no AC, from research) | 1.3.2a, 1.3.2b, 1.3.3a, 1.4.1a | `TestResolveExtraEnvVars_InstanceEnvVarsCopyShadowsLaterProgramEdit`; `TestResolveExtraEnvVars_InstanceWithoutEnvVarsSeesCurrentProgramEnv`; `TestInstanceData_RoundTripDropsEnvVars`; `useAvailablePrograms_should_UseProgramIdNotCommandAsValue_When_IdAndCommandDiffer` | E-none; Task 1.4.1a records the tmux experiment |

---

## Effort Estimate

Bands and weights are INFERRED (`~/.claude/skills/sdd/skills/ESTIMATION.md`); recalibrate after the first implementer run. CU = in + 5 x out, no cache discount (conservative). Output share: run-only tasks 5%, edit/test-writing 15%, mixed 10%. XS = 20k raw, S = 55k raw.

| Story | Tasks | Classes | Raw tokens | Output share | CU before x1.5 | CU with x1.5 | Wall-clock blocker |
|---|---|---|---|---|---|---|---|
| 1.1.1 | 4 | 4 XS | 80k | 5-15% | 112k | 168k | Real-tmux runs (up to 45 s each, five in 1.1.1d) |
| 1.1.2 | 3 | 2 XS, 1 S | 95k | 5-10% | 118k | 177k | Real-tmux run |
| 1.1.3 | 4 | 3 XS, 1 S | 115k | 5-15% | 160k | 240k | Upstream doc fetch (network) |
| 1.2.1 | 1 | XS | 20k | 15% | 32k | 48k | none |
| 1.2.2 | 1 | S | 55k | 15% | 88k | 132k | none |
| 1.3.1 | 1 | S | 55k | 15% | 88k | 132k | none |
| 1.3.2 | 2 | 1 S, 1 XS | 75k | 15% | 120k | 180k | none |
| 1.3.3 | 1 | XS | 20k | 15% | 32k | 48k | none |
| 1.4.1 | 1 | XS | 20k | 5% | 24k | 36k | none |
| 1.4.2 | 2 | 1 XS, 1 S | 75k | 5-10% | 94k | 141k | `make lint` and the `dupl` gate runtime |
| **Total** | **20** | 14 XS, 6 S | **610k** | | **868k** | **~1.3M CU** | |

- Size band: Medium (0.5-3M CU), informational only.
- Critical path: 4 agent waves (see Dependency Visualization); serial chain 1.1.0a -> 1.1.1b -> 1.1.1c -> 1.1.2a-c -> 1.4.2b because the overlay red run needs the converted test. Task 1.1.0a (S, ~55k raw tokens, roughly +88k CU with x1.5) and the E4b paste are not yet in the table totals above. Roughly half of the CU is AC-less hardening (Epics 1.2-1.4.1); Epic 1.1 alone is the first shippable unit.
- Overrun checkpoint: if cumulative spend on completed stories exceeds 1.25 x the "CU with x1.5" column, pause and report the delta and its cause (informational, not a scope cut).
- Wall-clock blockers (owners): owner answers to the three Unresolved Questions (Tyler; none blocks code in this item); a tmux binary matching `tmux.Binary()` on the machine running evidence (implementer); network access for Task 1.1.3b (implementer).
- Basis: cost bands INFERRED; planning overhead not included.

---

## Repair log (iteration 1)

Changes made in response to `adversarial-review.md` (verdict BLOCKED):

- **B1**: added Story 1.1.0 / Task 1.1.0a converting the landed test's three raw `exec.CommandContext` (lines ~729,738,741) to `safeexec.CommandContext` and both `time.Sleep` sites (~718,740) to `wait.RequireEventually`; verified against `tools/lint/norawexec/analyzer.go`, `tools/lint/notimesleeptest/analyzer.go`, `testutil/wait/eventually.go` and `Makefile:814-821` (`lint-custom` runs `bin/linter ./...`). Every test-writing task now carries a `bin/linter <pkg>` verify line and the helper rule; Task 1.1.1c no longer keeps the send-keys sleep loop; Task 1.4.2b made realistic (custom linter green only after 1.1.0a).
- **C1**: added corroborating evidence E4b (coordinator's pre-fix run at `cdfd4e5cf2^` = `4dbbe7b40`, failure on `show-environment`, setup with connect-go v1.19.1 and ent `--feature sql/upsert`); removed the "parent cannot host test helpers" claim from Pattern Decisions; overlay stays primary.
- **C2**: attribution corrected: `resolveExtraEnvVars` / `claudeSettingsEnvOverrideArgs` come from `5da2af7bf` (#852), verified with `git log -S`; `cdfd4e5cf2` has `buildExtraEnv` + `wireTmuxSession`. Header and reconciliation table updated.
- **C3**: AC4 restated honestly in Story 1.1.3 and the traceability table: flag delivery and hostile-value integrity VERIFIED locally; precedence over `settings.json` is upstream-documented, UNVERIFIED locally.
- **C4**: `-race -short` (and `TMUX_BIN`) added to verify lines and the E2 repeat run; 1.1.3c follows the LookPath-skip convention; 1.2.2a requires a mutex around captured argv; recreate-path backoff cost noted.
- **C5**: wave plan rewritten per package (one agent per package or per worktree); removed the false "overlay shares the tree" justification for serial Wave 3.
- **C6**: Epic 1.1 declared the first shippable unit; Epics 1.2, 1.3, 1.4 labelled AC-less hardening that may land separately; 1.3.2a overlap with the CLASH test and 1.3.2b's lost-`env_vars` gap labelled.
- **C7**: PIT-G rc-file masking justification relabelled defensive/SPECULATIVE (no masking in the reviewer's run).
- **C8**: `dupl` noted as moot for `_test.go` (`.golangci.yml:244-246`); Task 1.4.2b names `jscpd` as the gate that can bite.
- **Minors**: line drift fixed (`newSessionArgs` :637-647, inline loop :222-227, extra `ExtraEnv` append sites at `instance.go:1952,1959,2053,2060`, `server/adapters/instance_adapter.go:124`); fake-claude Unresolved Question closed (feasible); Task 1.4.1a reduced to pasting the observed tmux 3.6a result (later `-e` wins) and F3 mechanism marked VERIFIED; `seedCustomProgram` double-call caveat added to 1.3.2a; scratch-only/`Snapshot()` warning added to 1.1.2a.
