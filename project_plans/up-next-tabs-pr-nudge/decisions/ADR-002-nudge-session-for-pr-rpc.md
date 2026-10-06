# ADR-002: `NudgeSessionForPR` RPC, server-built prompt, per-session guard

**Status**: Accepted | **Date**: 2026-10-05

## Context
A one-click, un-editable prompt injects untrusted PR comment text into an agent PTY. Existing steering (`steerActiveSessionForPRFix`) is keyed by backlog item and has an item-keyed `steerInFlight`; MCP `steer_session`/`diagnose_nudge_session` are role-gated and not UI-callable. `session_ids` holds titles, and `FindLiveInstance` -> `MatchesID` accepts a title (`session/instance_terminal.go:72`).

## Decision
- New RPC `GitHubUserService.NudgeSessionForPR(PRKey, session_id)`; client never supplies text.
- Handler order: `parsePRKey` -> PR in cache by host+owner+repo+number -> session in PR's `LinkedSessions` (keys are host+owner+repo+branch; fixed in Task 1.3.1a, since `github/repo_ref.go:51,55` keys omit host and repo today) -> resolve the instance once -> token via PR's owning account -> fresh detail fetch -> `BuildPRNudgePrompt` (pure; empty = NOTHING_TO_FIX) -> `SessionService.SteerInstanceGuarded` on the resolved instance: atomic `TryBegin` (in-flight + 60s duplicate window under one lock) -> single idle gate `IsReadyForSteer` -> write. `VerifyNudgeSafeToWrite` is not a second gate; one gate shared with auto-steer avoids disagreeing idle notions and the re-resolve-by-title TOCTOU.
- Safety order inside the guarded steer: `TryBegin` -> readiness gate -> `VerifyPaneOwnershipBeforeWrite` (unconditional; `steerInstance` does not perform it) -> write. Auto-steer reaches it via UUID method `SteerSessionGuarded`; a guard BUSY/DUPLICATE there skips the tick (never `degradeToRespawnBlocked`), and the signature is the full reason signature. Sessions with no active controller get BUSY with distinct copy.
- Typed outcomes: DELIVERED, BUSY, PAUSED, DUPLICATE, NOTHING_TO_FIX, SESSION_NOT_LINKED, PR_NOT_FOUND. Rate limit = `ResourceExhausted`; unsupported program = `FailedPrecondition`.
- New `sessionNudgeGuard` keyed by session stable ID, owned by `SessionService` behind the existing `SessionSteerer` seam (one added interface method; no new `BacklogService` field), shared with `steerActiveSessionForPRFix` so click and automation cannot interleave and a click suppresses an identical auto-steer. Manual click bypasses the auto-steer 5-minute cooldown but not the 60s duplicate window.
- Prompt carries thread URL + path + author and failing check names only; **no third-party comment body** and **no appended `/github:pr-ship`** (a manual nudge keeps the user in the loop; untrusted text plus an auto-executing command widens injection blast radius). All GitHub-sourced strings pass `sanitizeUntrusted` (shared in `session/`), are framed as data, and the total is bounded by `session.MaxSteerMessageLength` (`session/instance.go:200`).
- The Nudge button shows only when `nudgeable` (failing checks, unresolved threads or merge conflict); changes-requested alone is hidden, matching server `NOTHING_TO_FIX`.
- Merge conflicts are a third `NudgeReason`.
- Prompt builder is new and pure over `PRNudgeDetail`, not `PRStatus.render()` (needs a worktree), but reuses `buildSteerMessage` for program suffix and truncation.

- **Access guard (write-capable RPC; VERIFIED middleware, `server/server.go:1678-1700`, `server/middleware/probeguard.go`, `auth.go`):** the local :8543 chain has no auth unless configured, and only `ProbeProgram` is protected there (ProbeGuard: POST-only, loopback-bound, loopback/allowed Host and Origin; the Host check stops DNS rebinding, which CORS does not). CORS only reflects allowed origins and there is no CSRF token. `NudgeSessionForPR` types into a live PTY, so it is added to a generalized ProbeGuard path set; on :8444 the existing `authMW` (WebAuthn cookie or Bearer) is the boundary, asserted by a test. No extra per-RPC rate limit: the in-flight + 60 s per-(session, signature) window caps writes and the single GraphQL query is limited by `AdmitOrigin` (Task 2.3.1f).
- **PR visible to two accounts:** `fetch()` dedups by URL and the first account in `resolveAllLogins` order wins (stable across polls); the nudge uses that account's token and never falls back to the other account (`PR_NOT_FOUND` if it cannot see the PR). Reorder accounts to change the owner (Task 1.4.1c).
- **State lifetime:** the guard's `last` map expires entries after 10 min and caps at 1,024; it, `firstSeenAttention` and the `nudge_followup` map are in-memory and lost on restart (a nudge just before a restart may repeat once). Code placement: new `SessionService` methods in `session_service_guarded_steer.go`, the guard in `session_nudge_guard.go`, the handler in `github_user_nudge.go`; the 6.8k-line `session_service.go` gains one field and a delegation.

## Alternatives rejected
Client-composed text via `UpdateSession` steer (no gate, stale data); calling `steerActiveSessionForPRFix` (item-coupled, status side effects); exposing the MCP nudge (role-gated).

## Open items
Seed path for "+ Session": decided in Phase 1 Task 1.4.2a (timeboxed 15 min; at most one additive optional `initial_prompt` + `PRFixSeeder` port, else deferred; fork PRs get a plain session). "Most recently active" field (`UpdatedAt` proxy).
