# Implementation Plan: program-env-injection

**Feature**: A custom program's registered `env` map must reach the spawned session process. The production fix already landed in two commits: `cdfd4e5cf2` (PR #825 squash; `buildExtraEnv` with an inline program-env merge, and `wireTmuxSession`) and `5da2af7bf` (#852, 2026-09-23; `resolveExtraEnvVars` and `claudeSettingsEnvOverrideArgs`, VERIFIED with `git log -S`; not an ancestor of `cdfd4e5cf2`). All in `session/instance_tmux.go`. This run closes AC1 and AC4a (flag delivery, hostile-value integrity, no regression of the #852 unit tests) with recorded evidence, also executes AC4b (precedence over a global `settings.json` `env` block; evidence.md E9), proves AC3 with a real red/green run, and closes the test-coverage gaps the research found. **No production source file changes.** Epic 1.1 (AC-bearing, includes the lint repair of the already-landed test) is the first shippable unit; Epics 1.2-1.4.1 are AC-less hardening that may land in a separate PR.
**Status update (2026-10-10, supersedes any 'AC4b UNVERIFIED / NOT executed' wording below):** AC4b was EXECUTED against a real `claude` 2.1.296 with local listeners (evidence.md E9): a global `settings.json` `env` beats an ambient env var (the #852 shape), and `--settings` `env` beats the `settings.json` `env` block. Limits: the CLI flag was run directly (the tmux-launch delivery half is AC4a / E8), and no org-managed settings were present. Backlog criterion 4 therefore rests on AC4a (E6, E8) plus AC4b (E9).
**Date**: 2026-10-10
**Status**: Ready for implementation
**ADRs**: None. No new technology or architectural pattern; every task adds tests or recorded evidence. Follow-up F2 (restart semantics) should open an ADR when taken, see Production-Behaviour Decisions.
**Inputs**: `project_plans/program-env-injection/requirements.md`, `research/{stack,architecture,pitfalls,build-vs-buy}.md`, prior plan `project_plans/program-env-not-applied/implementation/plan.md` (its `decisions/` dir is empty, so there are no prior ADRs to carry).
**Evidence file**: `project_plans/program-env-injection/implementation/evidence.md` (sections E1-E9; commands and verbatim output). **Single owner: the coordinator.** Worker agents never edit `evidence.md`; each reports command, `tmux -V`, test-file hash, verbatim output and gate verdict in its hand-back message, and the coordinator appends the section (see "Evidence file structure" below). Several parallel writers on one file is how sections get lost or interleaved.

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
| `PreFixOverlay` | A `go test -overlay` JSON mapping `session/instance_tmux.go` to a scratch copy of HEAD whose `wireTmuxSession` sets only `STAPLER_SESSION_UUID`. An env-only mutation of HEAD, a RECONSTRUCTION: at `cdfd4e5cf2^` `config.ResolveProgramConfig` did not exist in non-test code (program-ID to command resolution arrived together with the env merge), so "command resolves, env absent" never existed in history. | Never committed; the tree is not modified |
| `RealTmuxGate` | The preflight plus output gate every real-tmux run must pass (Task 1.1.0b): resolved existing `TMUX_BIN`, `-v`, a `--- PASS: <name>` line, zero `--- SKIP`. | A bare exit 0 / `ok` is never evidence for a real-tmux test |
| AC4a / AC4b | AC4a: `--settings` flag delivery + hostile-value integrity + no regression of existing #852 unit tests (executed, E6/E8). AC4b: program env outranks a global `~/.claude/settings.json` `env` block (executed against real `claude` 2.1.296, E9). | See Story 1.1.3 |
| `FakeClaude` | A shell script named `claude` that writes its argv to a file then `sleep`s; `isClaude` matches on token basename (`session/instance_tmux.go:239`). | Lets the real `--settings` path run through real tmux |
| `EvidenceLog` | `project_plans/program-env-injection/implementation/evidence.md`: verbatim commands and output. Single writer: the coordinator, from agent hand-back reports. | Per-AC proof |
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
- **Test-environment risk**: the real-tmux tests skip when no tmux binary is found and fail (not skip) when tmux exists but the session never reaches `Active` (`1b82f2889`). A tmux client/server version mismatch (`session/tmux/binary_resolution.go:38-39`) therefore fails the test as an environment fault, not a regression. **A skip exits 0 and prints `ok`**, so every real-tmux run must pass the `RealTmuxGate` (Task 1.1.0b): `TMUX_BIN` resolved to an existing executable (never `$(pwd)/bin/tmux` unless that file exists; `bin/` does not exist in a fresh worktree and `tmux.Binary()` returns `TMUX_BIN` verbatim, so `exec.LookPath` fails and the test skips), `-v`, a `--- PASS: <name>` line, and no `--- SKIP`. Evidence runs record `tmux -V` (local 3.6a; CI pins 3.4, so say "3.6a only").
- **Timeout/load risk**: `sessionCreateTimeoutDefault` is 10 s and is not stretched by `wait.ScaleTimeout`; real-tmux commands therefore set `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30` as the Makefile does (`Makefile:601,682`). A failure is re-run alone on a quiet machine before it is classified as a regression.

### Production-Behaviour Decisions

Every change that would alter production behaviour is held out of this item. Each is a follow-up (F-number) with its risk.

| ID | Change | Risk if done | Decision |
|----|--------|-------------|----------|
| F1 | Redact `--settings` env values from the three INFO logs (`instance_tmux.go:817`, `tmux_session_start.go:384`, `:671`) and from persisted/returned `LaunchCommand` (`instance_serialization.go:165`, `storage.go:160-162`, `server/adapters/instance_adapter.go:124`), or move `--settings` to a 0600 file | Medium. `LaunchCommand` is shown in the UI (`SessionDetailView.tsx`, `SessionCard.tsx`) and persisted, so redacting changes a user-visible field and stored data; moving env to a file adds a file lifecycle (cleanup, permissions, remote hosts). Redaction alone is partial: the value stays in the pane process argv (`ps`/`/proc/<pid>/cmdline`) and in `tmux -e` argv | **Follow-up item.** Needs a design choice (redact vs file) and a real-launch secret-leak test. Breaks the repo's own rule (`session_service_create.go:613`: "KEY NAMES ONLY -- never values") so it should be scheduled, not dropped |
| F2 | Resolve env once per launch and feed `-e` and `--settings` from it; choose restart policy (frozen at create vs re-resolve every launch; persist or drop `EnvVars`); make `extraEnv` re-resolve on `recreateMissingSession` | Medium-high. Changes signatures in a 40-commit/90-day file; changing policy alters what existing sessions see after a restart (a rotated token starts being picked up, or a removed key disappears) | **Follow-up item** with an ADR. Needs owner policy decision. This item ships characterization tests so the change is deliberate |
| F3 | Strip or reject `STAPLER_SESSION_UUID` and `CLAUDECODE` from user maps; also escape or reject env values ending in `;` (tmux `new-session` rc=1, VERIFIED tmux 3.6a; characterized in Task 1.2.1a) | Low-medium. Silently drops a key someone may set on purpose (`CLAUDECODE` set to re-enable the nested guard). Mechanism: `expectedOwnerUUID` returns the first `STAPLER_SESSION_UUID` in `extraEnv` (`tmux_ownership.go:44-52`), but a later user entry would win in tmux, so the ownership check would see a mismatch and treat the pane as foreign (VERIFIED on tmux 3.6a: later `-e` wins, see Task 1.4.1a) | **Follow-up item.** Hostile/odd config only; no evidence it occurs |
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
for every agent in the same package/worktree.

WORKTREE MODEL (decided): ONE WORKTREE PER AGENT. Agents A-D each edit in their own git
worktree (branch off the coordinator's branch), so no agent loads another agent's
half-written package and `go -C tools/lint build -o bin/linter` writes a per-worktree
output file. Each agent worktree is a FRESH worktree: it needs Wave 0's generated code
(`make proto-gen ent-gen`, `server/web/dist` stub, per Task 1.1.0b) before its first
`go vet`/`bin/linter`; the Wave 0 preflight therefore runs in every agent worktree, not
once. Inside one agent's worktree, `bin/linter` may be scoped to that agent's own
package(s) only (A: ./server/services; B: ./session/tmux; C: ./session), never to the
other agents' packages.
INTEGRATION WORKTREE: the coordinator's worktree (the item's own branch) is the single
place where Waves 2R, 3 and 4 run. After Wave 1 the coordinator merges (cherry-pick or
`git merge`) the four agent branches, in order A, B, C, D (disjoint files, so no
conflicts expected), then re-runs `bin/linter ./server/services ./session ./session/tmux`
on the merged tree as the integration gate BEFORE Wave 2R. Only test files are edited, so
the merge is the last point at which a cross-package lint or compile break can surface.

Wave 0 (serial, before any evidence run)
  1.1.0b  Preflight: generated code (proto-gen, ent-gen, server/web/dist stub, lint
          prerequisites; in the integration worktree AND each agent worktree) +
          web-app deps (pnpm install, jscpd prerequisites; Agent D worktree and the
          integration worktree) + citation re-confirmation + RealTmuxGate (resolve
          TMUX_BIN, record tmux -V, define gate)

Wave 1  -- EDIT-ONLY: agents write test files and run only fast, non-real-tmux checks
           (go vet / bin/linter / mock-executor tests / non-tmux unit tests). Cap
           concurrent `go build`/`go test -race` processes at 2 (use `-p 1` and
           GOMAXPROCS-limited agents). NO real-tmux run is made in Wave 1: a task's
           RealTmuxGate Verify line is split into "Wave 1 check" (vet + bin/linter,
           run by the editing agent) and a "RealTmuxGate run" that executes in
           Wave 2R below, serially, on a quiet machine.
  Agent A (package server/services): 1.1.0a -> 1.1.1b -> 1.1.1c, then 1.1.3c
  Agent B (package session/tmux):     1.2.1a -> 1.2.2a
  Agent C (package session):          1.3.1a -> 1.3.2a -> 1.3.2b -> 1.3.2c
  Agent D (web-app):                  1.3.3a
      |
Wave 1.5 (merge, coordinator, integration worktree; serial)
  merge agent branches A, B, C, D; run `bin/linter ./server/services ./session ./session/tmux`
  and `go vet` on the merged tree; fix any cross-package break before any real-tmux run
      |
Wave 2R (serial real-tmux, ONE run at a time, nothing else building; one agent or the
         coordinator, in the INTEGRATION worktree)
  1.1.0a, 1.1.1b, 1.1.1c   RealTmuxGate run of the converted test (single run, gate OK)
  1.1.3c                   RealTmuxGate run of the new test
  1.1.1a baseline (E1, AFTER the three runs above), 1.4.1a (shell evidence)
      |
Wave 2 (read-only / non-tmux; runs AFTER Wave 2R completes, never concurrently with it:
        1.1.3a is a `go test ./session` compile and must not load the machine while a
        30 s-timeout real-tmux run is in flight)
  1.1.3a, 1.1.3b     (evidence; 1.1.3a is non-tmux)
      |
Wave 3 (integration worktree; -overlay does not touch the tree; real-tmux runs are serial, one at a time)
  1.1.2a -> 1.1.2b -> 1.1.2c        (AC3, AC5)
  1.1.3d                            (needs 1.1.3c)
      |
Wave 4 (integration worktree)
  1.1.1d  repeat-run flake evidence (-count=5 -race), nothing else building
  1.4.2a  follow-ups recorded
  1.4.2b  final gates (custom linters, lint, targeted test set)
  Final claim: the CI-pinned tmux 3.4 run (see Effort Estimate, wall-clock blockers)
```

Critical path: 1.1.0b -> 1.1.0a -> 1.1.1b -> 1.1.1c -> 1.1.2a -> 1.1.2b -> 1.1.2c -> 1.4.2b.

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

**Goal**: AC1 and AC4a closed with commands and output in `evidence.md` (AC4b executed, E9; see Story 1.1.3); AC3 shown red on pre-fix behaviour and green on HEAD; AC5 tied to the verified pre-fix line. **This epic is the first shippable unit** (AC evidence plus the lint repair of the landed test); supporting epics can be rejected without touching the AC proof.

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
- Rule for every new test in this plan (Tasks 1.1.3c, 1.2.1a, 1.2.2a, 1.3.1a, 1.3.2a/b/c): subprocesses use `safeexec.CommandContext`; no `time.Sleep`; wait with `wait.RequireEventually`; any `safeexec`/`exec` call whose binary is `tmux.Binary()` derives its argv from `tmux.ResolveSocket(<name>).Args(...)` (`session/tmux/tmux.go:644`, `Socket.Args` at `:604`) or another sanctioned helper (`prependSocket`, `prependIsolatedSocket`), never a hand-written `"-L", name` literal and never an unscoped call (`tmuxsocketscope`).
- Linters the single `bin/linter` pass applies (names from `Makefile:814`), and how each is satisfied by this plan: `norawexec` (`safeexec.CommandContext`), `notimesleeptest` (`wait.RequireEventually`), `tmuxsocketscope` (sanctioned socket helper, above; the analyzer is heuristic, does not trace `tmuxBin := tmux.Binary()` through a variable, and accepts a literal `-L`, so the landed test passes today by those loopholes: do not rely on them in new code), `novartestseam` (n/a: no package-level `var` is added only to be reassigned from tests; test helpers are locals/consts), `silenttransition` (n/a: no `TransitionBacklogItemStatus`/`UpdateItemSessionEnded` call is added), `entfullscan`, `hotpolllog`, `noarchivedrevival`, `nocommandpattern`, `nolegacylog`, `noliveinstanceraw`, `norawghrequest`, `norawgitopen` (n/a: test-only change, no production file in their scope is edited). The `bin/linter` run is the authority; this list is a checklist, not a substitute.
- Files: `server/services/session_service_create_test.go`
- Verify (Wave 1 check): `go -C tools/lint build -o "$(pwd)/bin/linter" ./cmd/linter && bin/linter ./server/services ./session ./session/tmux` exits 0 (the Makefile's `lint-custom` recipe, `Makefile:814-821`, scoped to the touched packages; `make lint-custom` is the repo-wide form) and `go vet ./server/services`. Verify (RealTmuxGate run, Wave 2R, serial): the `RealTmuxGate` run (Task 1.1.0b) of `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` with `-race -short -count=1 -v` and `want=1`. Re-run the linter line after every later task that adds a test file.

##### Task 1.1.0b: `RealTmuxGate` preflight and output gate (XS, run-only; do this before ANY real-tmux run; every task below that says "`RealTmuxGate`" uses it)
- Re-confirm citations first (docs-only commits have landed since the plan's `9ef8fbc68` and validation's `56857ba09` pins; HEAD at triad iteration 2 was `572df53f8`). Before any edit: `git rev-parse HEAD`, then re-run the cheap greps that the line numbers in this plan hang on and record any drift in E1's header: `grep -n 'func (i \*Instance) \(resolveExtraEnvVars\|buildExtraEnv\|claudeSettingsEnvOverrideArgs\|wireTmuxSession\|initTmuxSession\)' session/instance_tmux.go`; `grep -n 'time.Sleep\|exec.CommandContext' server/services/session_service_create_test.go` (expect :718/:740 and :729/:738/:741); `grep -n 'func seedCustomProgram' session/instance_tmux_test.go`; `grep -n 'func (s \*TmuxSession) newSessionArgs' session/tmux/tmux_session_start.go`; `git diff --stat 9ef8fbc68 HEAD -- session server/services` (expect empty: docs-only commits). A cited line that moved is corrected in the worker's own edit; a cited symbol that is gone halts the item.
- Reason: the landed test calls `t.Skip` when `exec.LookPath(tmux.Binary())` fails, and `go test` exits 0 / prints `ok` for a skip, so a bare exit code can record a vacuous run as PASS (pre-mortem Failure #1).
- Preflight (resolve to an EXISTING binary; never `export TMUX_BIN="$(pwd)/bin/tmux"` unless that file exists, which it does not in a fresh worktree):
  ```bash
  export TMUX_BIN="${TMUX_BIN:-$(command -v tmux)}"
  test -x "$TMUX_BIN" || { echo "PREFLIGHT FAIL: no executable tmux at '$TMUX_BIN'"; exit 1; }
  "$TMUX_BIN" -V || exit 1                      # paste into E1; "3.6a only, CI pins 3.4"
  export STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30   # as Makefile:601,682 do
  ```
  If a pinned CI-equivalent build is wanted, build `bin/tmux` via the Makefile first, then `test -x bin/tmux` before pointing `TMUX_BIN` at it.
- Generated-code and lint prerequisites (every FRESH worktree, once, before the first `go test`/`go vet`/`bin/linter`; the code is gitignored, so a new agent worktree has none of it, and `go test ./server/services ./session` fails to compile without it). Use the Makefile's real targets, not hand-run generators:
  ```bash
  make proto-gen   # Makefile:547 -> `buf generate proto`; needs ensure-tools and web-app/node_modules (its own prerequisites). Writes gen/proto/go/... (gitignored)
  make ent-gen     # Makefile:564 -> `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`. The sql/upsert flag is mandatory; do not hand-run without it
  test -d server/web/dist || make server/web/dist   # Makefile:208; the Go embed needs the directory. Either build it (needs web-app/out, i.e. `make web-build`) or create a stub: `mkdir -p server/web/dist && touch server/web/dist/.gitkeep` (scratch only, gitignored; never commit)
  ```
  Web-app prerequisites (Task 1.3.3a and Task 1.4.2b's `jscpd` gate; run in the worktree that executes them, i.e. Agent D's and the integration worktree; `node_modules` is not shared between worktrees; pnpm only, `docs/how-to/use-pnpm-in-web-app.md`):
  ```bash
  test -d web-app/node_modules/.bin || (cd web-app && pnpm install --frozen-lockfile)
  test -x web-app/node_modules/.bin/jest  || { echo "PREFLIGHT FAIL: jest missing"; exit 1; }
  test -x web-app/node_modules/.bin/jscpd || { echo "PREFLIGHT FAIL: jscpd missing (lint:duplicates needs it)"; exit 1; }
  test -f web-app/.jscpd.json             || { echo "PREFLIGHT FAIL: web-app/.jscpd.json missing"; exit 1; }
  ```
  If `pnpm install` cannot run (no network), Task 1.3.3a and the `jscpd` line of Task 1.4.2b are reported NOT RUN, never green.
  For Task 1.4.2b's `make lint` the prerequisites are exactly the Makefile's own (`Makefile:798`: `ensure-tools proto-gen ent-gen server/web/dist lint-custom lint-shell`), plus `golangci-lint` v2 on `PATH` (the recipe `go install`s it if missing, which needs network) and `shellcheck` for `lint-shell`. Record in E1 which of these were already present and which were generated. `make build` satisfies the first three. Never `git add` the generated output (`.gitignore` excludes it; CLAUDE.md).
- Gate script (scratchpad only, never committed): `$SCRATCH/gate.sh <pass|fail> <TestName> <wantCount> <outfile>`:
  ```bash
  #!/bin/sh
  mode=$1; name=$2; want=$3; out=$4
  awk -v mode="$mode" -v n="$name" -v want="$want" '
    /^[[:space:]]*--- SKIP/ {skip++}
    mode=="pass" && index($0,"--- PASS: " n " (")==1 {hit++}
    mode=="fail" && index($0,"--- FAIL: " n " (")==1 {hit++}
    END {
      if (skip>0)      { print "GATE FAIL: " skip " --- SKIP line(s)"; exit 1 }
      if (hit!=want)   { print "GATE FAIL: want " want " \"--- " toupper(mode) ": " n "\" line(s), got " hit+0; exit 1 }
      print "GATE OK: " hit " \"--- " toupper(mode) ": " n "\" line(s), 0 SKIP"
    }' "$out"
  ```
- Run shape (always `-v`, capture, then gate; the gate output goes into the evidence section):
  ```bash
  go test -race -short <pkg> -run '^<Name>$' -count=<N> -v >"$SCRATCH/out.txt" 2>&1; rc=$?
  cat "$SCRATCH/out.txt" | tail -n 60
  [ $rc -eq 0 ] && "$SCRATCH/gate.sh" pass <Name> <N> "$SCRATCH/out.txt"
  ```
  For an expected-red overlay run: `rc` is non-zero, so use `"$SCRATCH/gate.sh" fail <Name> 1 "$SCRATCH/out.txt"` AND `grep -F "<assertion text>" "$SCRATCH/out.txt"`; a build failure (`[build failed]`) or a `--- SKIP` does not count.
- A `--- SKIP` anywhere (including subtests) rejects the run. A missing `--- PASS: <name>` rejects it. Neither is retried silently: fix the environment, then re-run.
- Files: scratchpad only
- Verify: `test -x "$TMUX_BIN" && "$TMUX_BIN" -V` prints a version; `sh "$SCRATCH/gate.sh" pass X 1 /dev/null` prints `GATE FAIL` (self-check that the gate rejects an empty log); `test -f gen/proto/go/session/v1/session.pb.go && ls session/ent/*.go >/dev/null && test -d server/web/dist && go build ./session/... ./server/services/...` exits 0 (generated code present and the packages compile).

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
- Order: run this AFTER Tasks 1.1.0a-1.1.1c so E1 is the converted, committed test (provenance matches E4/E5); if run earlier, label E1 "pre-conversion baseline (unconverted test)".
- Run the `RealTmuxGate` (Task 1.1.0b) for `go test ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v` (`want=1`) and plain `go test ./session -run 'EnvOverride|ExtraEnv|SettingsEnv' -count=1 -v` (non-tmux).
- The worker reports the E1 material in its hand-back; the coordinator (sole owner, see "Evidence file structure") creates `evidence.md` and appends it. Section E1 holds `git rev-parse HEAD`, `git hash-object server/services/session_service_create_test.go`, `tmux -V` ("3.6a only, CI pins 3.4"), the resolved `TMUX_BIN` path, both commands, verbatim output, the gate's `GATE OK` line, and the `t.Logf` pane/show-environment lines (Task 1.1.1b) containing `ENVPROBE_probe-7f3a91_END` and `ANTHROPIC_BASE_URL=`. Every later E-section also records `git hash-object` of the test file and `tmux -V`.
- Per-run provenance table at the top of `evidence.md`: E-section, test file hash, converted (1.1.0a-1.1.1c applied) yes/no, overlay yes/no, tmux version.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: `GATE OK` printed for T1; the second command exits 0 with `ok`; E1 shows the in-pane line.

##### Task 1.1.1b: Add the unique `ProbeKey` and document scope limits (XS, test edit)
- In `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`, add `SSQ_PROGRAM_ENV_PROBE` (value `probe-7f3a91`) to the program `Env`; point the in-pane `ENVPROBE_` probe at it. Keep the `show-environment` assertion for both `ANTHROPIC_BASE_URL` and the probe key.
- Extend the doc comment with a "Scope and limits" paragraph: `-e` applies only at `new-session`; a live pane or a same-name tmux session that is reused (`tmux_session_start.go:~189-196`, `instance_tmux.go:807-810`) keeps its old env until killed and recreated; the Claude `--settings` path is covered by `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess`, not by this `bash` test.
- Make the evidence pane text, not a PASS line: add `t.Logf("show-environment:\n%s", out)` and `t.Logf("pane capture:\n%s", captured)` immediately before the assertions (so they print under `-v`, and on failure), and make the pane assertion a `require` (it is the last assertion, nothing is lost). The lines containing `ENVPROBE_probe-7f3a91_END` and `ANTHROPIC_BASE_URL=http://127.0.0.1:47000` are pasted into E1 and E5.
- Reason for the probe key: defensive only. A user `~/.bashrc` exporting `ANTHROPIC_BASE_URL` could in principle mask the pane probe; no rc file sets `SSQ_PROGRAM_ENV_PROBE`. SPECULATIVE: the adversarial reviewer's overlay-red run on this machine showed no rc masking, and no real occurrence is cited.
- Files: `server/services/session_service_create_test.go`
- Verify (Wave 1 check): `go vet ./server/services && bin/linter ./server/services`. Verify (RealTmuxGate run, Wave 2R, serial): `RealTmuxGate` (Task 1.1.0b) run of `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v` (`want=1`), plus `grep -F 'ENVPROBE_probe-7f3a91_END' "$SCRATCH/out.txt"` finds the logged pane line.

##### Task 1.1.1c: Replace the hand-rolled Active poll with `wait.RequireEventually` (XS, test edit; same agent as Task 1.1.0a)
- Replace the `for time.Now().Before(deadline) ... time.Sleep(100ms)` loop (test lines 716-719) with `wait.RequireEventually(t, cond, 30*time.Second, 100*time.Millisecond, "session must reach Active")`; inside `cond`, call `t.Fatalf` when status is `session.Stopped` so a real spawn failure still fails fast (not skips). `testutil/wait` is already imported in this file (`wait.WaitForCondition`, line ~649).
- Reason: the 30 s bound is a fixed wall-clock wait; `wait.ScaleTimeout` stretches it under machine load (`testutil/wait/load.go:84`, `ScaleTimeout`; BUG-103) and the repo's `fix-flaky-tests-dont-defer` / `deterministic-fast-tests` skills prefer it. The send-keys retry loop is converted in Task 1.1.0a (lint requires it).
- Files: `server/services/session_service_create_test.go`
- Verify (Wave 1 check): `go vet ./server/services && bin/linter ./server/services`. Verify (RealTmuxGate run, Wave 2R, serial, after the vet/linter line): `RealTmuxGate` run (`-race -short -count=1 -v`, `want=1`, `GATE OK`).

##### Task 1.1.1d: Repeat-run flake evidence (E2) (XS, run-only)
- Run via `RealTmuxGate` (Task 1.1.0b, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`, resolved existing `TMUX_BIN`; CI uses `-race -short` with `TMUX_BIN="$(pwd)/bin/tmux"`, `.github/workflows/build.yml:275,327`, so build `bin/tmux` first or use system tmux and say so): `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=5 -v`, then `gate.sh pass TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession 5`. Run it serially on a quiet machine (nothing else building). `-count=5` re-runs the test five times in ONE process, so the service/config/worktree state must not collide across iterations: the test uses the fixed Title/Branch `program-env-repro`, and a cleanup that has not freed the worktree/branch would fail iteration 2+ with a collision that reads as a flake. Before the run, either (a) make Title and Branch unique per iteration (suffix with a per-run counter or `t.Name()` plus `time.Now().UnixNano()`; `destroyCreatedSession` stays in `t.Cleanup`), or (b) confirm that `destroyCreatedSession` removes both the worktree and the branch (read it, and after a `-count=2` run check `git worktree list` and `git branch --list 'program-env-repro*'` print nothing). Record which in E2; a failure in iteration 2+ that is a worktree/branch collision is a test defect, not a flake. Paste output, per-run durations and the `GATE OK` line into E2.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: `GATE OK: 5 "--- PASS: ..." line(s), 0 SKIP`. Five SKIP lines are a gate failure, not five passes. A single failure is a defect to fix, but re-run it alone on a quiet machine once before classifying (create-timeout/load, pre-mortem Failure #6).

#### Story 1.1.2: AC3 and AC5 - the test fails on pre-fix behaviour and passes on HEAD
**As a** maintainer, **I want** an executed red/green proof against the pre-`cdfd4e5cf2` behaviour, **so that** "regression test fails pre-fix" is evidence, not a claim.
**Acceptance Criteria**:
- AC3: a committed regression test fails on the pre-fix behaviour and passes on HEAD.
  - *Given* the `PreFixOverlay` (`wireTmuxSession` calls `SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})` only) and `ProgramConfig{ID: "netflix-model-gateway", Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}`, *When* `go test -overlay <json> ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$'` runs, *Then* it fails with the assertion message `tmux show-environment must carry the program's env`, and the same command without `-overlay` passes.
- AC5: root cause documented with the failure mechanism.
  - *Given* `git show cdfd4e5cf2^:session/instance_tmux.go`, *When* its `SetExtraEnv` call is inspected, *Then* it shows `if i.UUID != "" { session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID}) }` (line 578-580) and no `resolveExtraEnvVars`/`buildExtraEnv` symbol exists in that file.
  - Root-cause wording (already applied to `requirements.md` "Root cause", which no longer carries the earlier anachronistic "no code path merged `ResolveProgramConfig(...).EnvVars`" sentence; apply the same wording to E3 and the test's doc comment): VERIFIED by `git grep ResolveProgramConfig cdfd4e5cf2^ -- '*.go'` (no non-test hits; first introduced by `cdfd4e5cf2`) that at the parent commit custom program IDs were not resolved to a command at all; `cdfd4e5cf2` added program-ID resolution AND the env merge together. The only wire-time `SetExtraEnv` carried `STAPLER_SESSION_UUID`. Consequently the parent-commit run (E4b) fails for TWO reasons (ID unresolved, "Pane is dead (status 127)"; and env absent), and only the overlay run (E4) isolates the env-only failure. `PreFixOverlay` is a reconstruction of that failure on HEAD, not a state that existed in history.
**Files**: scratchpad overlay files (never committed), `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.1.2a: Build the `PreFixOverlay` and prove it matches the historical shape (E3) (XS)
- `cp session/instance_tmux.go <scratchpad>/instance_tmux.prefix.go`; in the copy's `wireTmuxSession` replace `if extraEnv := i.buildExtraEnv(); len(extraEnv) > 0 {` with a block that builds a slice containing only `"STAPLER_SESSION_UUID="+i.UUID` when `i.UUID != ""` (use the Edit tool on the copy, never on the tracked file). Scratch only: the pre-fix shape reads `i.UUID` raw; production must keep `Snapshot()` per `.claude/rules/instance-lock-free-reads.md`, so never paste it back. Write `<scratchpad>/overlay.json` as `{"Replace": {"<abs worktree>/session/instance_tmux.go": "<scratchpad>/instance_tmux.prefix.go"}}`.
- Equivalence check, not from memory: `git show cdfd4e5cf2^:session/instance_tmux.go | grep -n 'SetExtraEnv' -B2 -A1` shows the historical block; paste both into E3, together with `diff -u session/instance_tmux.go <scratchpad>/instance_tmux.prefix.go` (the unified diff of the mutant against HEAD, so a reviewer can reproduce it) and `git grep -n ResolveProgramConfig cdfd4e5cf2^ -- '*.go'` (empty for non-test files). State in E3 that the overlay is an env-only reconstruction (see Story 1.1.2 AC5 wording).
- Files: scratchpad only.
- Verify: `go build -overlay <scratchpad>/overlay.json ./session` exits 0 and `git status --short` prints nothing.

##### Task 1.1.2b: Run red under the overlay (E4) (S, run-only)
- `RealTmuxGate` expected-red run (preflight done, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`): `go test -overlay <scratchpad>/overlay.json ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v >"$SCRATCH/out.txt" 2>&1`, then `gate.sh fail TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession 1 "$SCRATCH/out.txt"` AND `grep -F "tmux show-environment must carry the program's env" "$SCRATCH/out.txt"`. An assertion failure, not a build error (a compile error or `[build failed]` means the overlay is wrong and the step does not count); a `--- SKIP` or a pass triggers the Contingency, not a recorded result.
- Also (non-tmux) `go test -overlay <scratchpad>/overlay.json ./session -run '^TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars$' -count=1 -v`: expect `--- PASS: ...`. Record in E4 that the resolver unit test stays green while the integration test is red; this is the PIT-5 proof.
- E4b provenance (coordinator-reported; NOT a verbatim record: `adversarial-review.md` holds only prose and no verbatim output exists in the repo): on a detached worktree at `cdfd4e5cf2^` (`4dbbe7b40`) with the HEAD regression test appended (the UNCONVERTED test, before Task 1.1.0a), the run was reported to fail on `tmux show-environment must carry the program's env` with only `STAPLER_SESSION_UUID` present, and the in-pane `printenv` assertion also failed ("Pane is dead (status 127)"). Do NOT paste a paraphrase as verbatim output and do NOT count E4b toward AC3. Label it "coordinator-reported, no verbatim record, unconverted test, fails for two reasons (ID unresolved + env absent)"; re-run it only if cheap (needs connect-go v1.19.1 proto regen and ent `--feature sql/upsert` regen), in which case paste the real output and apply the gate. The overlay (E4) is the AC3 evidence.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: E4 contains the verbatim `--- FAIL` output with the assertion text, the `GATE OK: 1 "--- FAIL: ..."` line, 0 SKIP, and the test file hash. If the integration test passes or skips, stop (Contingency). E4b is either verbatim real output with a commit hash or carries the "coordinator-reported" label.

##### Task 1.1.2c: Green on HEAD and clean tree (E5) (XS)
- Re-run the same command with no `-overlay` through the `RealTmuxGate` (`-v`, `want=1`); expect `--- PASS`. Then `git status --short` and `git diff --stat` must be empty for tracked files (apart from the intended test edits already committed/staged); delete the scratch overlay files. Paste the `t.Logf` pane line with `ENVPROBE_probe-7f3a91_END` into E5.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: `GATE OK: 1 "--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession"`, 0 SKIP; E5 shows the pane line and empty `git status --short` (same test file hash as E4).

#### Story 1.1.3: AC4 split into AC4a (delivery, executed) and AC4b (precedence, executed against real `claude`, E9)
**As a** maintainer, **I want** the `--settings` override proven through real tmux and a real shell, plus the upstream precedence rule quoted and clearly labelled unexecuted, **so that** AC4 is not reported closed on the strength of a doc quote.
**Acceptance Criteria**:
- **AC4a (executable, VERIFIED when Tasks 1.1.3a, 1.1.3c, 1.1.3d and 1.3.1a pass the gate)**: no regression of `claudeSettingsEnvOverrideArgs()`: the existing #852 unit tests stay green (Task 1.1.3a), the `--settings <env JSON>` flag reaches the launched process argv through real tmux and a real shell, and hostile values arrive intact (Tasks 1.1.3c, 1.3.1a).
- **AC4b (EXECUTED, evidence.md E9)**: program env still wins over a global `~/.claude/settings.json` `env` block. Method: scratch `CLAUDE_CONFIG_DIR` whose `settings.json` sets `env.ANTHROPIC_BASE_URL` to a GLOBAL local listener; real `claude -p hi` run with `env -i`; an ambient `ANTHROPIC_BASE_URL` and/or `--settings '{"env":{"ANTHROPIC_BASE_URL":"<PROGRAM listener>"}}'` varied across scenarios A-D; the listener that receives the request shows the URL the process used. Results: A and C hit GLOBAL (settings.json beats ambient env: the #852 shape), B and D hit PROGRAM (`--settings` env beats settings.json). The original design here (probe printing a variable through Claude's Bash tool, which needs a login) was superseded by this listener probe, which needs no credentials. Limits: the flag was run directly, not through `claudeSettingsEnvOverrideArgs()` + a tmux launch (that delivery half is AC4a / E8); org-managed settings (highest precedence per the docs) were not present.
  - *AC4a scenario*, *Given* `ProgramConfig{ID: "netflix-model-gateway", Command: "<tmp>/claude" (FakeClaude), Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000", "SSQ_HOSTILE": "it's $(touch <tmp>/pwned) `touch <tmp>/pwned` \"q\" a=b"}}`, *When* `CreateSession` launches it and the fake records its argv, *Then* the argv holds `--settings` followed by JSON equal to `{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:47000","SSQ_HOSTILE":"it's $(touch <tmp>/pwned) `touch <tmp>/pwned` \"q\" a=b"}}`, `<tmp>/pwned` does not exist, and `tmux show-environment` carries both keys.
  - *Given* the same program but `claudeSettingsEnvOverrideArgs` disabled by overlay, *When* the test runs, *Then* it fails at the `--settings` assertion.
- Task 1.1.3b's doc quote (E7) is documentation only and is now corroborated by the executed E9.
- **AC4b closure rule (superseded by E9)**: AC4 is reported `pass` (`criteria_index=3`) on AC4a (E6, E8) plus the executed AC4b (E9). The earlier fallback (report with AC4b UNVERIFIED and ask the owner) is not needed.
- **How the item reaches review (backlog completion gate)**: `request_review` fails closed unless EVERY backlog criterion is `pass` via `report_progress`, and a rejection counts toward the `blockedCycleThreshold` (3) escalation (`docs/reference/backlog-completion-gate-and-cleanup.md`, "The AC-completeness gate"). The backlog item's criteria are numbered 1-5 and map 1:1 to AC1-AC5 here (backlog criterion 4 = AC4 as a whole; `report_progress` `criteria_index` is 0-based, so AC4 = `criteria_index=3`; the `backlog:done-3` skill is that call). There is no separate backlog criterion for AC4a, so the plan chooses, in order:
  1. **AC4 is reported `pass` (`criteria_index=3`)** on AC4a executed and gated (E6 + E8 `GATE OK`) plus AC4b executed (E9). The review message cites E9 and its stated limits.
  2. **If the owner has not accepted by the time AC1-AC3, AC4a and AC5 are done, do NOT mark criterion 4 `pass` and do NOT call `request_review`** (it would be rejected, add a `[request_review:rejected]` note and burn one of three cycles). Instead call `report_progress` with `pass` for `criteria_index` 0, 1, 2, 4 and then `create_guidance_request` (or `report_blocked` with the reason "AC4b needs owner acceptance or an E9 run") asking Tyler to either accept AC4b as unverified or run E9. When the answer arrives, mark criterion 4 `pass` and call `request_review` with the acceptance quoted.
  3. **Earlier-session note, to be re-qualified**: a prior session marked `criteria_index=3` as `pass` on the basis of AC4a unit tests only (the #852 string-level tests, before T8/T9 existed and before the AC4a/AC4b split). That pass is not evidence for AC4 as written in `requirements.md`. The `request_review` message must say so explicitly ("criterion 4 was earlier marked pass on AC4a unit tests only; it is re-qualified here as AC4a executed end to end plus owner acceptance of AC4b") and, if acceptance is not yet recorded, the coordinator re-marks criterion 4 `fail`/`in_progress` via `report_progress` (statuses `pending`, `in_progress`, `fail` are all treated as not done) so the stale `pass` cannot carry the gate.
- **Who runs E9**: a human with a real `claude` login and credentials (the owner, or a person the owner designates); no agent or CI job in this item has them. The coordinator hands the human the exact probe above and pastes the result into E9 verbatim with `claude --version`. Not running E9 is a recorded decision, not a blocked task.
**Files**: `server/services/session_service_create_settings_env_test.go` (new), `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.1.3a: Run the existing #852 tests (E6) (XS, run-only)
- `go test ./session -run 'TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars|TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars|TestBuildClaudeCommand_IncludesSettingsEnvOverride|TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars' -count=1 -v`
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: four `--- PASS: <name>` lines in E6 (run with `-v`; these are non-tmux unit tests, so skips are not expected, but any `--- SKIP` is still rejected).

##### Task 1.1.3b: Quote the upstream precedence rule for AC4b (E7) (XS, research; documentation only, does NOT verify AC4b)
- Read https://code.claude.com/docs/en/settings with `mcp__stapler-mcp__read_website` (save to scratchpad if large). Quote the lines giving the order of command-line arguments (`--settings`), user settings, project settings and managed settings, with URL and retrieval date. If the fetch fails, write "UNVERIFIED" and keep the in-repo comment (`instance_tmux.go:741-755`) as the only source.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: E7 contains a quoted ordering or the literal word `UNVERIFIED`, and the line "AC4b: documentation quote only; executed verification is E9".

##### Task 1.1.3c: Add `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess` (S, test-first)
- New file `server/services/session_service_create_settings_env_test.go`. Reuse `newCreateTestService`, `createTestStorage`, `initGitRepoWithCommit`, `destroyCreatedSession` (all in the same package).
- Setup (Task 1.1.0a rules apply: `safeexec.CommandContext`, no `time.Sleep`): follow the existing LookPath-skip convention (skip when `exec.LookPath(tmux.Binary())` fails; fail, not skip, once tmux exists but the session never reaches `Active`); `envtest.NewIsolatedStateDir(t)` (makes `config.IsIsolatedInstance()` true so `trustStore()` is in-memory and `~/.claude.json` is untouched, `session/claude_trust.go:165`); `t.Setenv("HOME", t.TempDir())` as defence.
- `FakeClaude`: write `<tmp>/claude` mode 0755: `#!/bin/sh`, write `printf '%s\n' "$@"` to `<out>.tmp` then `mv` to `<out>`, then `exec sleep 300`. Register program `Command: <tmp>/claude` with the two env keys above (hostile value includes `'`, `$(...)`, backticks, `"`, `=`); create a `SESSION_TYPE_NEW_WORKTREE` session.
- Wait with `wait.RequireEventually` for `<out>`; assert: argv contains `--settings`; the next element unmarshals to `map[string]map[string]string` equal to the registered env; `<tmp>/pwned` absent; `show-environment` contains `SSQ_HOSTILE=` and `ANTHROPIC_BASE_URL=`.
- Feasibility: confirmed by the adversarial review's scratch spike (experiment 3; see closed Unresolved Question). Expect a harmless `ERROR claude launch: MCP server URL unresolved` log.
- Test-first order: write the test, confirm it fails under the V-settings overlay (Task 1.1.3d) before treating it as done.
- Files: `server/services/session_service_create_settings_env_test.go`
- Socket rule (`tmuxsocketscope`): the `show-environment` call (and any other tmux call this test makes) builds its argv with `tmux.ResolveSocket(svc.testTmuxServerSocket).Args("show-environment", "-t", tmuxName)` passed to `safeexec.CommandContext(ctx, tmux.Binary(), args...)`; do not copy the landed test's literal `"-L", socket` form. Name this in the test's header comment so the next reader keeps it.
- Verify (Wave 1 check): `go vet ./server/services && bin/linter ./server/services` (all custom linters, including `tmuxsocketscope`, `norawexec`, `notimesleeptest`, `novartestseam`, `silenttransition`, reported clean). Verify (RealTmuxGate run, Wave 2R, serial): `RealTmuxGate` run (`STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`, `go test -race -short ./server/services -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$' -count=1 -v`, `gate.sh pass ... 1`, `GATE OK`, 0 SKIP) `&& bin/linter ./server/services`. Log the recorded argv with `t.Logf` so the `--settings` JSON appears in the `-v` output pasted into E8.

##### Task 1.1.3d: Overlay red/green for the AC4 test (E8) (XS)
- Same mechanism as Task 1.1.2a, second overlay: the copy's `claudeSettingsEnvOverrideArgs` body replaced with `return "", ""`. Run Task 1.1.3c's command with `-overlay` through the `RealTmuxGate`: `gate.sh fail TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess 1` plus a `grep -F` of the `--settings` assertion message (not a build error, not a SKIP); re-run without the overlay: `gate.sh pass ... 1`; `git status --short` empty. Record `tmux -V`, `tmux show-options -gv default-shell` and `$SHELL` in E8.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: E8 holds both outputs and both `GATE OK` lines (fail-mode and pass-mode), 0 SKIP.

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
- Table cases: none; `ExtraEnv` only; `extraEnv` only; both (order); hostile value as one element; **value ending in `;`** (`K=a,b;`). The `;` case is CHARACTERIZATION ONLY, documenting a known failure: the argv layer carries the value verbatim and unescaped (`-e`, `K=a,b;`; no escaping exists in `session/tmux`), and VERIFIED on tmux 3.6a (private socket, `new-session -d -s t -e 'K=v;' 'sleep 30'`) tmux then fails the whole `new-session` with rc=1 / "no server running", so the session goes to Stopped with no env-related message. Values `a;b`, `#{session_name}x`, `#(echo hi)` survive intact. Expected behaviour pinned and named in the case comment: "unescaped; tmux rejects a trailing `;`; fix is F3 (escape as `\;` or reject), not this item". No production change, no real-tmux assertion. Build with `NewTmuxSessionWithDeps("args-test", "prog", nil, MockCmdExec{})`; set `s.ExtraEnv` directly and `s.SetExtraEnv(...)`. Compare the whole slice with `require.Equal`. `t.Parallel()` is safe (no env or filesystem).
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
- Error path (validation gap G1): an unregistered program yields only instance env.
  - *Given* an `Instance{Program: "not-registered", EnvVars: {"K": "v"}}` and a config with no program `not-registered`, *When* `resolveExtraEnvVars()` is called, *Then* it does not panic and returns exactly `{"K": "v"}` (no phantom program env); with no `EnvVars` it returns an empty map.
- `EnvVars` is not persisted.
  - *Given* an `Instance{Title: "envvars-roundtrip", Path: "/path/to/repo", Status: Paused, Program: "bash", EnvVars: {"K": "v"}}`, *When* `ToInstanceData()` then `FromInstanceData()` runs, *Then* `restored.Snapshot().EnvVars` is empty.
**Files**: `session/instance_program_env_semantics_test.go` (new)

##### Task 1.3.2a: Add the two resolver characterization tests (S, test-first)
- `TestResolveExtraEnvVars_InstanceEnvVarsCopyShadowsLaterProgramEdit` and `TestResolveExtraEnvVars_InstanceWithoutEnvVarsSeesCurrentProgramEnv`. Use `seedCustomProgram` (`session/instance_tmux_test.go:1192`) once, then load config, modify `Env`, and `SaveConfig` explicitly: `seedCustomProgram` calls `t.Setenv(STAPLER_SQUAD_TEST_DIR, t.TempDir())` on every call, so calling it twice silently swaps in a fresh empty config instead of editing the first. Header comment: "characterization of current behaviour (PIT-B); changing it is follow-up F2". Not `t.Parallel()` (`t.Setenv`).
- Files: `session/instance_program_env_semantics_test.go`
- Note: the in-process half restates the CLASH case of `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` (`instance_tmux_test.go:1202`); keep it only because it is the named characterization that flips under F2.
- Verify: `go test -race -short ./session -run 'TestResolveExtraEnvVars_' -count=1 -v && bin/linter ./session`

##### Task 1.3.2b: Add `TestInstanceData_RoundTripDropsEnvVars` (XS, test-first)
- Same file; mirror `TestToInstanceData_PreservesBackend` (`session/instance_serialization_test.go:15`) but **drop its `t.Parallel()`**: `envtest.NewIsolatedStateDir` calls `t.Setenv` (`envtest/envtest.go:71-75`; its doc comment: must be called before `t.Parallel()`, "it panics otherwise"), and `t.Setenv` is forbidden in parallel tests, so the new test must not call `t.Parallel()` at all. Add `envtest.NewIsolatedStateDir(t)` because `FromInstanceData` for a `Paused` instance calls `wireTmuxSession` when the process manager is a `TmuxBackend` (`instance_serialization.go:463-467`), and that calls `config.LoadConfig()`. Read the result through `Snapshot()` per `.claude/rules/instance-lock-free-reads.md`. Name and comment it as characterization of a gap, not a guarantee: because `EnvVars` is not persisted, request-level `env_vars` are silently lost across a server restart.
- Files: `session/instance_program_env_semantics_test.go`
- Verify: `go test -race -short ./session -run '^TestInstanceData_RoundTripDropsEnvVars$' -count=1 && bin/linter ./session`

##### Task 1.3.2c: Add `TestResolveExtraEnvVars_should_ReturnOnlyInstanceEnv_When_ProgramNotRegistered` (XS, test-first; validation gap G1, accepted)
- Same file and helpers as Task 1.3.2a (`t.Setenv(STAPLER_SQUAD_TEST_DIR, t.TempDir())`, no `t.Parallel()`); config contains no program with the instance's `Program` ID. Two sub-cases via `t.Run`: with `EnvVars {"K":"v"}` (expect exactly that map) and with none (expect empty, non-panicking). Read via the resolver only; no tmux.
- Files: `session/instance_program_env_semantics_test.go`
- Verify: `go test -race -short ./session -run 'TestResolveExtraEnvVars_should_ReturnOnlyInstanceEnv_When_ProgramNotRegistered' -count=1 -v && bin/linter ./session` (expect `--- PASS: ...`, no `--- SKIP`).

#### Story 1.3.3: The web client sends the program ID, not its command
**As a** maintainer, **I want** a fixture where `id` and `command` differ, **so that** mapping `value: p.command` is caught (the existing fixture uses `id === command === "aider"`).
**Acceptance Criteria**:
- *Given* `mockList` resolving `{programs: [{id: "netflix-model-gateway", label: "Netflix Model Gateway", description: "", command: "claude", cliFlags: ""}]}`, *When* `useAvailablePrograms()` is rendered and awaited, *Then* `result.current[0].value` is `"netflix-model-gateway"` and `result.current[0].command` is `"claude"`.
**Files**: `web-app/src/lib/hooks/useAvailablePrograms.test.ts`

##### Task 1.3.3a: Add `useAvailablePrograms_should_UseProgramIdNotCommandAsValue_When_IdAndCommandDiffer` (XS, test-first)
- New `it` in the existing `describe`; leave the existing cases untouched. This locks only the hook's mapping; Omnibar `dispatch.ts` was not re-audited in this run.
- Preflight (Agent D's worktree is fresh; `node_modules` is per worktree): the web-app block of Task 1.1.0b (`pnpm install --frozen-lockfile` when `web-app/node_modules/.bin` is absent; jest, jscpd and `web-app/.jscpd.json` present). Without it the Verify line fails on a missing `jest`, which is an environment fault, not a test result.
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
- Use `tmux.Binary()`'s path (honour `TMUX_BIN`); record `tmux -V`; finish with `tmux -L ssq-dup-env-probe kill-server` (touches only that socket, never the live one). This is a shell run, outside `tmuxsocketscope`'s Go-AST scope; if it is ever turned into a Go test it must build its argv with `tmux.ResolveSocket("ssq-dup-env-probe").Args(...)` (see the Task 1.1.0a rule).
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: the evidence section shows the printed `K=...` line and the kill-server exit status 0.

#### Story 1.4.2: Follow-ups recorded and gates green
**As a** maintainer, **I want** deferred work written down with risks and the final gates run, **so that** the PR is reviewable and nothing is silently dropped.
**Acceptance Criteria**:
- *Given* the Production-Behaviour Decisions table, *When* Task 1.4.2a completes, *Then* `evidence.md` has `### F1` through `### F5`, each with title, risk, and proposed acceptance criteria.
- *Given* all new tests, *When* the Task 1.4.2b commands run, *Then* each exits 0.
**Files**: `project_plans/program-env-injection/implementation/evidence.md`

##### Task 1.4.2a: Write the follow-up section (XS)
- Copy F1-F5 from this plan (title, risk, decision) and add a one-line proposed test per item (F1: real create with a secret value, grep captured logs and persisted `LaunchCommand`; F2: flip Story 1.3.2's tests; F3: `buildExtraEnv` with `STAPLER_SESSION_UUID` in `EnvVars`; F4: argv capture on `createRemoteSession`; F5: tymux-backed session `show-environment`). File them via `create_backlog_item` only if the owner answers yes to the first Unresolved Question. Regardless of that answer, F1-F5 (title, risk, one-line proposed test) are listed verbatim in the PR body under "Follow-ups for the owner to file", so a decision not to file is a decision, not an omission. The PR body also carries two mandatory paragraphs: (i) **"Residual routes for the original symptom"**: closing this item does not prove the user-visible symptom ("registered env silently ignored") cannot recur; it can still occur through routes this item does not cover, namely edit-after-create then reuse of a live session (`initTmuxSession` reuse guard, `instance_tmux.go:807-810`, pinned not changed), in-process restart with frozen `EnvVars` (F2), the tymux gRPC backend (F5, inferred, not traced) and remote execution targets (F4); closing is a deliberate owner decision, not an implication; (ii) **"Companion bug not covered"**: the directory-collision bug named in the backlog item is out of scope here and has no separate ID known; the PR body names it by description ("directory collision named in backlog item 4bbe28f9") and asks the owner to file or link it so it is not dropped (optionally the coordinator files it via `create_backlog_item` if the owner says yes). Also reference the earlier-session criterion-4 re-qualification (Story 1.1.3). The worker drafts the section; the coordinator appends it to `evidence.md`.
- Files: `project_plans/program-env-injection/implementation/evidence.md`
- Verify: `grep -c '^### F' project_plans/program-env-injection/implementation/evidence.md` prints 5.

##### Task 1.4.2b: Final gates (S, run-only)
Prerequisites (Task 1.1.0b's generated-code block, verified present in THIS worktree before starting): `make proto-gen ent-gen`, `server/web/dist` present, and for `make lint` the Makefile's own chain (`Makefile:798`: `ensure-tools proto-gen ent-gen server/web/dist lint-custom lint-shell`) with `golangci-lint` v2 and `shellcheck` on `PATH`; if `golangci-lint` is missing the recipe `go install`s it (network). If any prerequisite cannot be satisfied, record which line was NOT run and why (not "green").
Use the invocations CI uses (`-race -short`; `.github/workflows/build.yml:275,327`) with the Task 1.1.0b preflight applied (existing `TMUX_BIN`, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`) and EVERY `go test` run with `-v` captured to a file and gated; a bare exit 0 is not accepted.
- `go test -race -short ./session/tmux -run 'TestNewSessionArgs_|TestNewSessionArgv_' -count=1 -v` (gate: `--- PASS: TestNewSessionArgs_EmitsExtraEnvPairs`, `--- PASS: TestNewSessionArgv_BothCreationPaths_CarryExtraEnv`, no `--- SKIP`)
- `go test -race -short ./session -run 'TestResolveExtraEnvVars_|TestInstanceData_RoundTripDropsEnvVars|TestClaudeSettingsEnvOverrideArgs|TestBuildClaudeCommand_IncludesSettingsEnvOverride|TestInstance_BuildExtraEnv' -count=1 -v` (gate: each named test has a `--- PASS` line, including the three `TestResolveExtraEnvVars_*`, no `--- SKIP`)
- `go test -race -short ./server/services -run 'TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession|TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess|TestCreateSession_should_NotLogEnvVarValues_When_EnvVarsProvided' -count=1 -v` (gate: `gate.sh pass <Name> 1` for EACH of the three names; `--- SKIP` anywhere rejects)
- Custom linters: `make lint-custom` (builds `bin/linter`, runs `bin/linter ./...`, `Makefile:814-817`; the same step CI runs; the single pass covers `entfullscan, hotpolllog, noarchivedrevival, nocommandpattern, nolegacylog, noliveinstanceraw, norawexec, norawghrequest, norawgitopen, notimesleeptest, novartestseam, silenttransition, tmuxsocketscope`). It can only be green because Task 1.1.0a repaired the landed test; before 1.1.0a it reports 5 findings. Record the `custom lint: ok` line in the evidence.
- `make lint` (depends on `lint-custom`).
- `dupl` is moot for new Go tests: `.golangci.yml:244-246` excludes `_test.go` from `gocyclo,gocognit,funlen,revive,dupl`, so `make ready-complexity-gate` needs no table-test refactor. The gate that can bite is web `jscpd`: `cd web-app && pnpm run lint:duplicates` for the added Jest `it` (absolute threshold ratchet, see CLAUDE.md).
- Web gate prerequisites: the web-app preflight of Task 1.1.0b (`pnpm install --frozen-lockfile`, jest and jscpd present) in the integration worktree; `pnpm run lint:duplicates` is NOT RUN, never green, if they are missing.
- Files: none
- Verify: every command exits 0 AND every gate prints `GATE OK` with 0 SKIP; output summarized in the PR body, which cites E9 (AC4b executed, with its stated limits). If Epics 1.2-1.4.1 land in a separate PR, run only the lines for the packages each PR touches. The final claim about real-tmux behaviour is worded per "CI-pinned tmux 3.4 run" in the Effort Estimate (a named blocker with exact fallback wording): until a 3.4 run is recorded the claim is "verified on tmux 3.6a only".

---

## Evidence file structure

`project_plans/program-env-injection/implementation/evidence.md` is created and edited ONLY by the coordinator. Agents return, for each run, a text block with: command (verbatim), `tmux -V`, `git hash-object` of the test file, verbatim output (or the tail with the assertion and `t.Logf` lines), and the `gate.sh` verdict line. The coordinator appends it as the section named below. One section per row; a row without an executed run stays literally `NOT RUN`.

Top of file, in this order: (1) header with `git rev-parse HEAD`, `TMUX_BIN` path, `tmux -V`, generated-code status (Task 1.1.0b); (2) the table below, filled as sections land; (3) sections `### E1` ... `### E9`, then `### F1` ... `### F5` (Task 1.4.2a).

| Section | Purpose (AC) | Command | `tmux -V` | Test-file hash | Result (verbatim ref) | Gate-script verdict |
|---|---|---|---|---|---|---|
| E1 | HEAD baseline, converted test (AC1, AC2) | `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v` | | | | `GATE OK: 1 "--- PASS"` |
| E2 | Repeat-run flake evidence (AC1, AC2) | same, `-count=5` | | | | `GATE OK: 5 "--- PASS"` |
| E3 | `PreFixOverlay` equivalence (AC3, AC5) | `git show cdfd4e5cf2^:...`, `diff -u`, `git grep ResolveProgramConfig cdfd4e5cf2^` | n/a | n/a | | n/a (static) |
| E4 | Red under overlay (AC3); E4b parent-commit run, coordinator-reported, not counted | `go test -overlay <json> ./server/services -run ... -count=1 -v` | | | | `GATE OK: 1 "--- FAIL"` + assertion `grep -F` |
| E5 | Green on HEAD, clean tree (AC3) | same as E1, no overlay; `git status --short` empty | | | | `GATE OK: 1 "--- PASS"` |
| E6 | #852 unit tests, no regression (AC4a) | `go test ./session -run 'TestClaudeSettingsEnvOverrideArgs_...' -count=1 -v` | n/a (non-tmux) | | | 4 `--- PASS`, 0 SKIP |
| E7 | Upstream precedence quote (AC4b, documentation only) | `mcp__stapler-mcp__read_website` | n/a | n/a | quote + date, or `UNVERIFIED` | n/a |
| E8 | `--settings` end to end, red/green overlay (AC4a) | Task 1.1.3c command with and without `-overlay` | | | | `GATE OK` fail-mode and pass-mode |
| E9 | Real `claude -p` precedence probe (AC4b) | Story 1.1.3 probe; run by a human with credentials | n/a (`claude --version`) | n/a | `cli` or `global`; `NOT RUN` by default | n/a |

---

## Traceability: AC -> Tasks -> Tests

| AC | Tasks | Tests (exact names) | Evidence section |
|----|-------|---------------------|------------------|
| AC1 pane `printenv` shows the var | 1.1.0a, 1.1.1a, 1.1.1b, 1.1.1c, 1.1.1d | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (`session_service_create_test.go:673`) | E1, E2 |
| AC2 `tmux show-environment` includes the var | 1.1.1a, 1.1.1b, 1.2.1a, 1.2.2a | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`; `TestNewSessionArgs_EmitsExtraEnvPairs`; `TestNewSessionArgv_BothCreationPaths_CarryExtraEnv` | E1, E2 |
| AC3 regression test fails pre-fix, passes on HEAD (and passes the custom linters) | 1.1.0a, 1.1.2a, 1.1.2b, 1.1.2c | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (red under `PreFixOverlay`); `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` (stays green, PIT-5) | E3, E4, E4b, E5 |
| **AC4a** `--settings` flag delivery + hostile-value integrity + no regression of existing #852 unit tests (executed, VERIFIED when gated) | 1.1.3a, 1.1.3c, 1.1.3d, 1.3.1a | `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`; `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars`; `TestBuildClaudeCommand_IncludesSettingsEnvOverride`; `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess`; `TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell` | E6, E8 |
| **AC4b** program env outranks a global `settings.json` `env` block (**executed, E9**) | 1.1.3b (doc quote) | listener probe against real `claude` 2.1.296: scenarios A-D, evidence.md E9 | E7 (quote); E9 (executed, verbatim) |
| AC5 root cause documented with mechanism | 1.1.2a (historical diff), 1.1.1b (doc comment) | n/a (documentation). Doc comment above `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`; `requirements.md` Root cause | E3 |
| Supporting (no AC, from research) | 1.3.2a, 1.3.2b, 1.3.2c, 1.3.3a, 1.4.1a | `TestResolveExtraEnvVars_InstanceEnvVarsCopyShadowsLaterProgramEdit`; `TestResolveExtraEnvVars_InstanceWithoutEnvVarsSeesCurrentProgramEnv`; `TestResolveExtraEnvVars_should_ReturnOnlyInstanceEnv_When_ProgramNotRegistered` (AC1 error path, G1); `TestInstanceData_RoundTripDropsEnvVars`; `useAvailablePrograms_should_UseProgramIdNotCommandAsValue_When_IdAndCommandDiffer` | E-none; Task 1.4.1a records the tmux experiment |

---

## Effort Estimate

Bands and weights are INFERRED (`~/.claude/skills/sdd/skills/ESTIMATION.md`); recalibrate after the first implementer run. CU = in + 5 x out, no cache discount (conservative). Output share: run-only tasks 5%, edit/test-writing 15%, mixed 10%. XS = 20k raw, S = 55k raw.

| Story | Tasks | Classes | Raw tokens | Output share | CU before x1.5 | CU with x1.5 | Wall-clock blocker |
|---|---|---|---|---|---|---|---|
| 1.1.0 | 2 | 1 S, 1 XS | 75k | 5-15% | 112k | 168k | Generated-code build (`make proto-gen ent-gen`, buf/ent toolchain, possible network) |
| 1.1.1 | 4 | 4 XS | 80k | 5-15% | 112k | 168k | Real-tmux runs (up to 45 s each, five in 1.1.1d) |
| 1.1.2 | 3 | 2 XS, 1 S | 95k | 5-10% | 118k | 177k | Real-tmux run |
| 1.1.3 | 4 | 3 XS, 1 S | 115k | 5-15% | 160k | 240k | Upstream doc fetch (network) |
| 1.2.1 | 1 | XS | 20k | 15% | 32k | 48k | none |
| 1.2.2 | 1 | S | 55k | 15% | 88k | 132k | none |
| 1.3.1 | 1 | S | 55k | 15% | 88k | 132k | none |
| 1.3.2 | 3 | 1 S, 2 XS | 95k | 15% | 152k | 228k | none |
| 1.3.3 | 1 | XS | 20k | 15% | 32k | 48k | none |
| 1.4.1 | 1 | XS | 20k | 5% | 24k | 36k | none |
| 1.4.2 | 2 | 1 XS, 1 S | 75k | 5-10% | 94k | 141k | `make lint` and the `dupl` gate runtime |
| **Total** | **23** | 16 XS, 7 S | **705k** | | **1,012k** | **~1.52M CU** | |

Task count check: 23 task headings (1.1.0a, 1.1.0b; 1.1.1a-d; 1.1.2a-c; 1.1.3a-d; 1.2.1a; 1.2.2a; 1.3.1a; 1.3.2a-c; 1.3.3a; 1.4.1a; 1.4.2a, 1.4.2b). Re-count with `grep -c '^##### Task' project_plans/program-env-injection/implementation/plan.md` (expect 23). The prior table (20 tasks, ~868k CU before / ~1.3M with x1.5) omitted 1.1.0a (S), 1.1.0b (XS) and 1.3.2c (XS), +95k raw, +144k CU before x1.5, +216k with x1.5. The E4b paste is coordinator-reported and costs nothing.

- Size band: Medium (0.5-3M CU), informational only.
- Critical path: 4 agent waves plus the serial real-tmux Wave 2R (see Dependency Visualization); serial chain 1.1.0b -> 1.1.0a -> 1.1.1b -> 1.1.1c -> 1.1.2a-c -> 1.4.2b because the overlay red run needs the converted test. Roughly half of the CU is AC-less hardening (Epics 1.2-1.4.1); Epic 1.1 alone is the first shippable unit.
- Overrun checkpoint: pause and report the delta and its cause (informational, not a scope cut) if cumulative spend on completed stories exceeds 1.25 x the "CU with x1.5" column, i.e. above ~1.9M CU in total (1.25 x ~1.52M = ~1.90M) or 1.25 x the row value for any single story (for example 1.1.0 above ~210k, 1.3.2 above ~285k).
- Wall-clock blockers (owners): owner answers to the three Unresolved Questions (Tyler; none blocks code in this item); a tmux binary matching `tmux.Binary()` on the machine running evidence (implementer); network access for Task 1.1.3b (implementer); generated-code build and `golangci-lint` install in each fresh worktree (implementer); **a CI-pinned tmux 3.4 run for the final claim** (CI / implementer, see below); **E9, the real-`claude` AC4b probe (a human with credentials: Tyler or his designee; optional, non-blocking, see the AC4b closure rule)**.
- **CI-pinned tmux 3.4 run (named blocker for the final real-tmux claim)**: all local evidence is tmux 3.6a, and CI pins 3.4 (`.github/workflows/build.yml:275,327`, `TMUX_BIN="$(pwd)/bin/tmux"`). Final claim for AC1/AC2/AC4a requires either (i) the CI run of the PR's tests (`TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`, `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess`) green with the gate (`--- PASS`, 0 `--- SKIP`) read from the job log, or (ii) a local run against a built `bin/tmux` whose `-V` prints 3.4, recorded in E2. Exact fallback wording if neither can be run: "Verified on tmux 3.6a only; the CI-pinned tmux 3.4 was not run for this evidence (tmux 3.2 is the documented minimum for `-e`, STK-1, not tested here)." The PR body and `report_progress` must use that sentence, not "verified" without the version.
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

---

## Repair log (validate iteration 1)

Changes made in response to `pre-mortem.md` (P1 #1, #2; P2 #3-#6; P3 #7) and `validation.md` gap G1. Docs only; no source changes.

- **P1 #1 (skipped real-tmux test recorded as pass)**: new Task 1.1.0b defines the `RealTmuxGate`: `TMUX_BIN` resolved with `command -v`/`test -x` to an existing binary (never a nonexistent `$(pwd)/bin/tmux`), `tmux -V` recorded, `-v` always, and `$SCRATCH/gate.sh <pass|fail> <Name> <count> <out>` (awk) requiring `--- PASS: <name> (` x count (or `--- FAIL:` for red runs) and rejecting any `--- SKIP`. Applied to Tasks 1.1.0a, 1.1.1a, 1.1.1b, 1.1.1c, 1.1.1d (E2, `want=5`), 1.1.2b (E4, fail-mode + assertion grep), 1.1.2c (E5), 1.1.3c, 1.1.3d (E8) and 1.4.2b (final gate, now `-v` with per-name gates). Risk Control and glossary updated.
- **P1 #2 (AC4 closed while its defining claim is unexecuted)**: AC4 split into AC4a (flag delivery, hostile-value integrity, no regression of #852 unit tests; executed) and AC4b (precedence over a global `settings.json` `env` block; upstream-documented, NOT executed). Story 1.1.3 states the exact unrun probe (real `claude -p`, scratch `CLAUDE_CONFIG_DIR`, `SSQ_PRECEDENCE_PROBE` global vs `--settings`), reserves E9, and forbids ticking AC4 as passing without it. Header, Epic 1.1 goal, glossary, Task 1.1.3b, Task 1.4.2b and the Traceability table reworded so nothing claims AC4b verified.
- **P2 #3**: Task 1.1.1b adds `t.Logf` of the `show-environment` output and the pane capture (pane assertion becomes `require`); E1/E5 paste the `ENVPROBE_probe-7f3a91_END` line; Task 1.1.3c logs the recorded argv.
- **P2 #4**: evidence provenance: E1 runs after conversion (or is labelled pre-conversion); per-run table (test file hash, converted?, overlay?, `tmux -V`); E3 pastes the overlay diff; E4b relabelled "coordinator-reported, no verbatim record, unconverted test" and not counted toward AC3.
- **P2 #5**: AC5 root cause reworded: `config.ResolveProgramConfig` did not exist non-test at `cdfd4e5cf2^`; resolution and env merge arrived together, so the parent-commit run fails for two reasons and `PreFixOverlay` is an env-only reconstruction on HEAD.
- **P2 #6**: `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30` (as `Makefile:601,682`) in every real-tmux command; Wave 0 preflight added; Wave 1 made edit-only with concurrent build/test processes capped at 2; E2/E4/E5/E8 run serially on a quiet machine.
- **P3 #7**: `;`-suffixed value added to Task 1.2.1a's table as characterization-only (argv carries it unescaped; tmux 3.6a `new-session` rc=1, VERIFIED in the pre-mortem); F3 extended with the escape-or-reject fix.
- **Gap G1 accepted**: Task 1.3.2c adds `TestResolveExtraEnvVars_should_ReturnOnlyInstanceEnv_When_ProgramNotRegistered` (Story 1.3.2 AC, Wave 1 Agent C, Traceability). Effort: +1 XS (~20k raw) and +1 XS for Task 1.1.0b, not in the table totals.
- Not changed (out of scope this pass): pre-mortem P3 #8-#10 remain advisory (NUL-delimited fake argv, evidence.md single-writer, send-keys cadence).

---

## Repair log (triad iteration 1)

Changes made in response to the engineering and product triad lenses. Docs only; nothing executed, no source changes.

- **GAP-1 (generated code)**: Task 1.1.0b now carries a generated-code preflight using the Makefile's real targets (`make proto-gen` `Makefile:547`, `make ent-gen` `Makefile:564` with `--feature sql/upsert`, `server/web/dist` `Makefile:208` or a gitignored stub) and a compile check in its Verify line; Task 1.4.2b lists the `make lint` chain (`Makefile:798`: `ensure-tools proto-gen ent-gen server/web/dist lint-custom lint-shell`, plus `golangci-lint` v2 and `shellcheck`) and says an unsatisfied prerequisite is reported NOT RUN, never green. Wave 0 updated.
- **GAP-2 (Wave 1 vs real-tmux)**: Wave 1 stays edit-only; Tasks 1.1.0a, 1.1.1b, 1.1.1c, 1.1.3c split their Verify into a "Wave 1 check" (vet + `bin/linter`) and a "RealTmuxGate run" executed in a new serial Wave 2R, one run at a time; 1.1.1a (E1) runs after them. Critical path now starts at 1.1.0b.
- **GAP-3 (linters)**: Task 1.1.0a names every linter in the `Makefile:814` pass with how each is satisfied or why n/a (`norawexec`, `notimesleeptest`, `tmuxsocketscope`, `novartestseam`, `silenttransition`, rest n/a); the rule now requires tmux calls in new tests to derive argv via `tmux.ResolveSocket(...).Args(...)` (`session/tmux/tmux.go:604,644`; sanctioned names read from `tools/lint/tmuxsocketscope/analyzer.go`), not a literal `-L` and not by relying on the analyzer's variable-tracing and literal-`-L` loopholes. Task 1.1.3c states the socket rule; Task 1.4.1a notes its shell scope; Task 1.4.2b records the `custom lint: ok` line. `validation.md` L1 and G12 mirror it.
- **GAP-4 (evidence ownership)**: the coordinator is the single writer of `evidence.md`; agents hand back text blocks (header, glossary row, Tasks 1.1.1a and 1.4.2a updated). New "Evidence file structure" section holds the E1-E9 table (command, `tmux -V`, test-file hash, result, gate-script verdict). `validation.md` G14.
- **GAP-5 (effort)**: table now 23 tasks (new row 1.1.0 for 1.1.0a S and 1.1.0b XS; 1.3.2 gains 1.3.2c XS): 16 XS + 7 S = 705k raw, 1,012k CU, ~1.52M CU with x1.5 (was 20 tasks / 610k / 868k / ~1.3M). Overrun checkpoint recomputed: 1.25 x ~1.52M = ~1.9M CU total, or 1.25 x any story row. Task count verified with `grep -c '^##### Task'` = 23.
- **GAP-6 (tmux 3.4)**: the CI-pinned tmux 3.4 run is a named wall-clock/CI blocker with two ways to satisfy it and exact fallback wording ("Verified on tmux 3.6a only; the CI-pinned tmux 3.4 was not run for this evidence ..."); Task 1.4.2b and Wave 4 reference it. `validation.md` G15.
- **Citation drift**: `wait.ScaleTimeout` -> `testutil/wait/load.go:84` (opened, `func ScaleTimeout` at :84); `seedCustomProgram` -> `session/instance_tmux_test.go:1192` (opened, `func seedCustomProgram` at :1192).
- **Product gaps**: AC4b closure rule added to Story 1.1.3 (item may be reported with AC4b explicitly UNVERIFIED and owner acceptance requested; the coordinator must not tick AC4 as a whole; AC4 closes only on an executed E9 or recorded owner acceptance); E9 is run by a human with credentials (Tyler or designee), listed as an optional non-blocking blocker; F1-F5 are listed in the PR body for the owner to file regardless of the first Unresolved Question (Task 1.4.2a). `validation.md` G16.
- Not changed: target-user/impact statement, requirements.md out-of-scope list and companion-bug pointer (requirements.md is the coordinator's); F1 owner/date (owner decision, Unresolved Question 1).

---

## Repair log (triad iteration 2)

Changes made in response to the round-2 product and engineering triad lenses. Docs only; nothing executed, no source changes.

- **Product G-A (AC4 vs completion gate)**: Story 1.1.3 gains "How the item reaches review". Backlog criteria 1-5 map 1:1 to AC1-AC5 (criterion 4 = AC4 = `report_progress` `criteria_index=3`). AC4 is reported `pass` only when AC4a is executed and gated AND owner acceptance of unverified AC4b is recorded in the `request_review` message (or E9 ran and printed `cli`); otherwise criteria 0, 1, 2, 4 are passed, criterion 4 is not, `request_review` is not called, and `create_guidance_request`/`report_blocked` asks the owner. The earlier session's `criteria_index=3` pass (AC4a unit tests only) is to be re-qualified in the review message and re-marked non-pass if acceptance is absent. The AC4b closure-rule bullet was reworded to match (no per-AC4a tick).
- **Product G-B**: Task 1.4.2a requires a PR-body paragraph "Residual routes for the original symptom" naming reuse guard after edit, F2 frozen `EnvVars`, F5 tymux and F4 remote. (requirements.md carries the matching risky assumption.)
- **Product G-C**: Task 1.4.2a requires a PR-body paragraph naming the companion directory-collision bug (no ID known) and asking the owner to file or link it.
- **Product G-D**: Story 1.1.2 AC5 wording no longer says it "replaces" a requirements.md sentence that was already reconciled away; it says the wording is already applied.
- **Product G-E**: validation.md T5-T9 retagged AC4a.
- **Product G-F**: Task 1.1.0b opens with a "re-confirm citations" step (HEAD rev, symbol and line greps, `git diff --stat 9ef8fbc68 HEAD -- session server/services`), recording drift in E1. validation.md header notes the pin drift.
- **Product G-G**: requirements.md evidence row for AC1/2 states it stays "Observed once" until E1 and E5 land with the gate; not promoted to VERIFIED earlier.
- **Engineering G1**: Dependency Visualization now fixes the worktree model: one worktree per agent (each needs Wave 0 generated code and, for Agent D, web-app deps), `bin/linter` scoped to the agent's own package inside its worktree, a new Wave 1.5 merge-and-integration-lint step by the coordinator, and the coordinator's integration worktree named as the place Waves 2R, 3 and 4 run.
- **Engineering G2**: Wave 2 now runs after Wave 2R, never concurrently; overlap claim removed.
- **Engineering G3**: Task 1.1.1d requires unique Title/Branch per iteration or a verified worktree/branch cleanup before `-count=5`; collisions in iteration 2+ are classed as a test defect, not a flake.
- **Engineering G4**: Task 1.3.2b says drop `t.Parallel()` (the `TestToInstanceData_PreservesBackend` model has it) because `envtest.NewIsolatedStateDir` uses `t.Setenv`; verified at `envtest/envtest.go:64-76` (doc comment: panics when mixed with `t.Parallel()`).
- **Engineering G5**: Task 1.1.0b gains a web-app preflight (`pnpm install --frozen-lockfile`, jest/jscpd/`.jscpd.json` present); Task 1.3.3a and Task 1.4.2b reference it and report NOT RUN if unsatisfied.
- Not changed: effort table (no task added; the new merge step is coordinator bookkeeping inside existing 1.1.0b/1.4.2b budgets, recalibrate after first run).
