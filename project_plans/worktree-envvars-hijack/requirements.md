# Requirements: worktree-envvars-hijack

**Date**: 2026-09-24
**Type**: bug fix (severe — silent isolation failure / cross-session data leak)
**Complexity**: unknown pending research — the triager's manual read of the obvious candidate code paths (below) did not find the root cause, so Phase 2 research needs to look wider than the reporter's own hypothesis.

## Problem Statement
Backlog item `a4d287ba-f050-4931-a9c9-59c7ea387b14` reports that two otherwise-identical `CreateSession` calls diverged:

- **Request A**: `sessionType: SESSION_TYPE_NEW_WORKTREE`, `program: "netflix-model-gateway"`, `branch: "test/consolette-model-gateway-check"`, no `envVars`. Result: correct, isolated worktree under `~/.stapler-squad/workspaces/<hash>/worktrees/test-consolette-model-gateway-2_<id>/`.
- **Request B**: `sessionType: SESSION_TYPE_NEW_WORKTREE`, `program: "claude"`, `branch: "test/consolette-model-gateway-check-3"`, **`envVars: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}`**. Result: `GetSession` reported `workingDir`/`activeDir`/`existingDir` all equal to the bare repo path (`/Users/tstapler/code/github.com/tstapler/consolette`), `status: SESSION_STATUS_ACTIVE`, no error anywhere. The repo path happened to already host a live, unrelated Claude Code session; the new session's tmux pane showed that other session's conversation content, including turns that postdated the new session's creation.

The reporter's own hypothesis — that `envVars` presence is the trigger — is **not yet confirmed by code reading**. Note the two requests also differ in `program` (`netflix-model-gateway`, a custom program, vs. `claude`, the built-in default), which the reporter's writeup does not treat as a variable but which this triager's investigation could not rule out. Both differences must be isolated in Phase 2 research, ideally via an actual repro (varying one field at a time) rather than by code reading alone.

## Baseline (triager's investigation — what was and wasn't found)
Read during triage, in `server/services/session_service.go`'s `CreateSession` (~line 2304) and `session/instance.go`/`session/instance_worktree.go`:

- `resolveSessionType()` (`server/services/session_service.go:3108`) resolves `SESSION_TYPE_NEW_WORKTREE` from the explicit request field unconditionally — confirmed no `envVars` branch, matching the item's own finding.
- `instanceEnvVars` merge logic (`session_service.go:2450-2532`, aliases → `config.ResolveDefaults`/`ResolveAlias` → request `EnvVars` → custom-program `EnvVars`) only ever writes into a `map[string]string` that becomes `InstanceOptions.EnvVars` (`session_service.go:2859`) — no control-flow branch reads `len(instanceEnvVars)` or `req.Msg.EnvVars` to affect `sessionType`, `resolvedPath`, or any worktree decision.
- `session.NewInstance()` (`session/instance.go:1065`) copies `opts.SessionType` straight onto `instance.SessionType` with no `EnvVars`-conditioned override.
- `session.CreateManagedInstance()` (`session/create_managed_instance.go:107`) only pre-flight-checks path existence for `SessionTypeDirectory`/`SessionTypeNewWorktree`; no `EnvVars` involvement.
- `Instance.setupFirstTimeWorktree()` (`session/instance_worktree.go:53`) switches strictly on `i.SessionType`; `SessionTypeNewWorktree`'s case (line 61) unconditionally calls `newWorktreeFromResolvedBase()` — no `EnvVars` check gates it.
- `startLocked()`/`start()` (`session/instance.go:1467`, `:1716`) call `finishFirstTimeSetup()` → `setupFirstTimeWorktree()` whenever `firstTimeSetup` is true, which `runBackgroundResolutionPipeline` (`server/services/session_creation_pipeline.go:258`) always passes as `true` for a fresh `CreateSession`. `recoverConversationBeforeLaunch()` (`session/instance.go:1421`), which is where a `DetectByPath`-style conversation-UUID recovery could plausibly cause cross-session hijack, **no-ops when `firstTimeSetup` is true** — ruling out the most obvious mechanism for "attached to another session's conversation" bug class (the same family as the separate `cold-start-uuid-loss` bug).

None of this rules the bug out — it only rules out the specific code paths inspected. Candidates *not yet checked* and left for Phase 2 research:
- Whether `program: "claude"` vs. a custom program ID changes `config.ResolveProgramConfig`'s behavior in a way that could interact with path/worktree resolution differently than `netflix-model-gateway` did (the untested variable the reporter didn't isolate).
- `buildLaunchCommand`/`resolveExtraEnvVars` (`session/instance_tmux.go:579-650`) and whatever ultimately decides the `claude` CLI's own `--resume`/continue-in-cwd behavior at process-spawn time, independent of this app's own `--resume` plumbing — i.e., whether the *Claude Code CLI itself*, not stapler-squad, is what picked up the neighboring session's conversation once it was launched in the (wrongly) unisolated directory.
- Whether the actual repro trigger is a race/duplicate-title collision, a stale/cached alias resolution, or a `SkipDefaults`/`AliasName` interaction not present in either request body verbatim but plausibly present in the reporter's full client-side request construction (not shown in the item).
- Whether this reproduces at all on current `main`, or was already fixed/changed by unrelated work since the report — a live repro (per the item's own 3-step Repro section) should be attempted before design work, not skipped.

## Users / Consumers
- Any caller of `CreateSession` with `SESSION_TYPE_NEW_WORKTREE` plus a non-default `program`/`envVars` combination — this includes the Omnibar UI, MCP tool callers, and scripted/API callers.
- Any user with multiple concurrent Claude Code sessions who could have an unrelated live session silently read from or written to by a new, ostensibly isolated session — a direct violation of the per-session workspace isolation guarantee documented in `docs/reference/state-isolation.md` and enforced elsewhere by rules like `.claude/rules/instance-lock-free-reads.md`'s `ActiveDir` semantics.

## Success Metrics
- **Repro confirmed or refuted with evidence**: Phase 2 research reproduces (or definitively fails to reproduce, with a stated reason) the divergence between Request A and Request B shapes against current `main`, isolating whether `envVars`, `program`, or something else in the two requests is the actual trigger.
- **Root cause identified with a file:line citation**, not a symptom patch — per this repo's `code-root-cause-analysis` discipline (see CLAUDE.md's Engineering Discipline).
- **No silent worktree-creation skip**: when `SESSION_TYPE_NEW_WORKTREE` is requested, either a real isolated worktree is created, or `CreateSession`/the session's terminal status surfaces a visible, non-2xx-swallowed error — never a silent fallback to the bare repo path with `status: ACTIVE`.
- **No cross-session attach**: a newly created session must never end up sharing a live tmux pane, PTY, or Claude conversation stream with a different, pre-existing session — verified by a regression test that creates two sessions against the same repo path (one pre-existing and "active", one new) and asserts the new session's pane/conversation content is disjoint from the first.

## Appetite
Unknown until Phase 2 research narrows the root cause — this item explicitly could not be root-caused by reading the obvious call sites during triage. Given the severity (data isolation / cross-session leak), treat this as a `sdd:fix-bug`-shaped Small-to-Medium appetite investigation-then-fix, not a redesign; if research surfaces something structurally deeper (e.g., a tmux/session-naming collision class), escalate appetite rather than force a narrow patch onto a wider problem.

## Constraints
- Must not regress the working case (Request A shape: explicit worktree session type, no envVars, custom program) — a regression test for that path should exist or be added alongside the fix.
- Any fix must preserve `envVars`-based program configuration (e.g., `ANTHROPIC_BASE_URL` overrides) as a supported, working feature — the companion issue #852 referenced by the item (custom-program env vars not being applied) is explicitly out of scope here but must not be made worse.
- No data loss for the *other*, unrelated live session that was inadvertently attached to — any fix must ensure the new-session code path can never write into an existing session's tmux pane/conversation, not just detect it after the fact.

## Non-functional Requirements
- **Security/isolation classification**: this is a session-isolation defect, not just a feature bug — the fix must close the cross-session attach vector definitively, not merely make it less likely.
- **Observability**: whatever the root cause turns out to be, add a hard failure or loud warning (not just a log line — see the item's own complaint that `ACTIVE` with no error was the core problem) when a `SESSION_TYPE_NEW_WORKTREE` request cannot get a real, isolated worktree path.

## Scope
### In Scope
- Reproducing the divergence (or documenting why it can't be reproduced) with a minimal, controlled test that varies `envVars` and `program` independently.
- Identifying and fixing the exact code path that lets a `SESSION_TYPE_NEW_WORKTREE` session silently resolve to the bare repo path instead of a worktree.
- Ensuring a session that resolves to a shared/pre-existing directory can never attach to another session's live tmux pane or Claude conversation — whether that means hardening tmux session-name uniqueness, the `claude` CLI launch invocation, or the worktree-path resolution itself, depending on what research finds.
- A regression test reproducing the original two-request scenario (or the actual confirmed trigger, if it differs from the reporter's hypothesis).

### Out of Scope
- Issue #852 (custom-program env vars not being applied) — explicitly flagged by the item as a separate, lower-severity companion issue.
- General session-isolation hardening beyond what's needed to close this specific silent-fallback/cross-attach vector.
- UI-level confirmation dialogs or new user-facing isolation-warning features beyond making an existing silent failure loud.

## Rabbit Holes
- **Two uncontrolled variables in the repro** (`program` and `envVars` both differ between Request A and B) — Phase 2 must not assume the reporter's `envVars` hypothesis is correct without testing `program` independently; conflating them risks fixing the wrong thing.
- **This may not be a stapler-squad bug at all in the "cross-attach" half** — if research confirms the working directory really was wrongly shared, the "read another session's conversation" symptom could be entirely an artifact of the `claude` CLI's own continue-most-recent-in-cwd behavior (external tool behavior), meaning the real, sufficient fix is just closing the directory-sharing bug, not building new anti-attach machinery inside stapler-squad.
- **Custom vs. built-in program resolution** (`config.ResolveProgramConfig`, `session_service.go:2521-2531`) is unexamined territory from this triage pass and could plausibly interact with path resolution differently for `"claude"` than for a registered custom program — worth a dedicated research pass rather than assuming it's irrelevant.

## Alternatives Considered
- **Patch `resolveSessionType` defensively regardless of root cause** — rejected for this requirements doc; the item's own analysis already showed `resolveSessionType` resolves correctly, so a patch there would be a symptom fix with no confirmed root cause, violating this repo's root-cause-first discipline.

## Feasibility Risks
- The bug may be timing/race-dependent (the item notes "I stopped the session immediately... without sending it further input once I noticed" and describes the failure as observed once, not systematically) — reproduction may require multiple attempts or careful control of ambient state (an already-running session in the same path is itself part of the repro setup, not incidental).
- If the true trigger is external (`claude` CLI behavior) rather than internal, the fix surface shifts from "stop skipping worktree creation" to "never let a new session's working directory silently collide with an existing live session's," which is a different, and possibly larger, fix.
