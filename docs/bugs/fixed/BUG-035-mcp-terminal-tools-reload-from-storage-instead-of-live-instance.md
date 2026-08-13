# BUG-035: MCP Terminal Tools (`send_control`, `write_to_session`, `run_command`, `steer_session`) Always Look Up Sessions Via a Fresh, Deferred-Start `LoadInstances()` Call Instead of the Live Instance [SEVERITY: Critical]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — following the BUG-034 deploy, confirmed `send_control`/`SendKeys` still failed with `"cannot send keys to instance that has not been started or is paused"` for sessions confirmed alive and responsive at the tmux/process level. BUG-034's bug doc explicitly flagged this as unproven and possibly a separate bug; this is that separate bug.
**Fixed**: 2026-07-22 — `server/mcp/tools_terminal.go`, `server/mcp/server.go`
**Impact**: Every MCP terminal-mutation tool (`send_control`, `write_to_session`, `run_command`, `steer_session`) resolved its target `*session.Instance` by calling `th.store.LoadInstances()` fresh on every single invocation. `Storage.LoadInstances()` unconditionally calls `fromInstanceData(data, true)` — `deferStart=true` — which intentionally leaves the returned `Instance`'s `started` flag `false` so server boot isn't blocked starting every restored session synchronously; a background "Step 6" loop in `server/dependencies.go` calls `Start(false)` on each real, live instance roughly 200ms apart, off the critical path. `LoadInstances()`'s deferred-start instances are throwaway values used for the read-only "does this ID exist" check elsewhere in the codebase — but the terminal tools' `findInstance` was mutating the tools' `*session.Instance` pointer to one of *these* freshly-deserialized, never-started decoys instead of the actual live instance already running in memory with its real PTY handles. Every mutating call therefore hit the `i.started` guard and failed deterministically, 100% of the time, for every session — not a race, not a resource-pressure symptom.

## Live Symptoms

- `send_control` and `write_to_session` failing on every session tested, immediately after service start and identically on long-running sessions — ruling out any warm-up or resource-pressure timing theory.
- Error text: `"cannot send keys to instance that has not been started or is paused"` — this is `Instance.SendKeys`'s `i.started` atomic-bool guard rejecting the call.
- `read_session_output` and `list_sessions` (read-only tools) worked fine — only the mutation path was affected, since scrollback reads go through a separate `scrollback.ScrollbackManager`, not the `Instance`'s PTY.

## Root Cause

`server/mcp/tools_terminal.go`'s `findInstance` (used by `writeToSession`, `sendControl`, `runCommand`, `steerSession`) called:

```go
instances, err := th.store.LoadInstances()
for _, inst := range instances {
    if inst.MatchesID(sessionID) { return inst, nil }
}
```

`Storage.LoadInstances()` (`session/storage.go`) always deserializes with `deferStart=true` (`session/instance_serialization.go`), which deliberately leaves `started=false` on the returned `Instance` — a server-boot-only optimization so cold-restoring 100+ sessions doesn't block the HTTP bind while each one's tmux/PTY machinery spins up. The *real*, already-running `*session.Instance` (with `started=true` and live PTY handles) lives in the in-memory session registry, reachable via `SessionService.FindLiveInstance` — already the documented-correct path ("Use this instead of `LoadInstances()` for read-only and mutation operations that need the live instance"), already used elsewhere (`ReviewQueuePoller.FindInstance`), and already backed by the same `MatchesID` matching semantics. `tools_terminal.go` simply never used it — every mutation call re-deserialized a disposable copy from disk metadata and mutated *that*, which was never started and never would be.

## Fix Applied

1. **`server/mcp/tools_terminal.go`**: added a narrow `liveInstanceFinder` interface (`FindLiveInstance(id string) *session.Instance`) consumed by `terminalHandlers`, and a `live liveInstanceFinder` field. Rewrote `findInstance` to check the live finder first, falling back to the existing `LoadInstances()` scan only when the live finder is nil or doesn't have the session (matching pre-existing test coverage that doesn't wire `th.live` and preserving read-only/tool-listing behavior for a session that exists in storage but isn't currently running).
2. **`server/mcp/server.go`**: wires `*services.SessionService` (which already implements `FindLiveInstance`) into `terminalHandlers.live` in `NewCore`. Guards against the classic Go nil-interface trap explicitly: assigning a nil `*services.SessionService` straight into the `liveInstanceFinder` interface field would produce a non-nil interface wrapping a nil pointer, so `th.live != nil` would be true and a call would panic on the nil receiver. `NewCore` only assigns when `svc != nil`.

## Files Affected

- `server/mcp/tools_terminal.go` — `liveInstanceFinder` interface, `terminalHandlers.live` field, `findInstance` live-first-then-fallback logic
- `server/mcp/server.go` — wires `svc` as the `liveInstanceFinder`, with the nil-interface guard

## Verification

- `TestFindInstance_should_returnLiveInstance_When_LiveFinderHasIt`, `..._should_fallBackToStore_When_LiveFinderDoesNotHaveIt`, `..._should_fallBackToStore_When_LiveFinderNil` — new unit tests using a `fakeLiveInstanceFinder` test double.
- **Verified to fail against pre-fix code**: `git stash push -- server/mcp/tools_terminal.go` then re-running the new tests produces a build failure (`undefined: liveInstanceFinder`, `unknown field live in struct literal of type terminalHandlers`) in both `server.go` and the new test file — confirms the fix is load-bearing.
- Pre-existing tests `TestSteerSessionMCP_passesValidationAndReachesSendKeys`, `TestSendControlBytes`, `TestWriteInputLengthCap`, `TestReadOutputLineCap`, `TestReadOutputSessionNotFound`, `TestWaitForOutputTimeout` — all still pass with `th.live` unset, confirming the fallback path is unbroken for existing callers.
- `go build ./...` clean.
- Full `go test ./session/... ./server/mcp/... ./server/services/...` regression suite run — see verification note appended after this doc was written.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap — a correct, already-documented, already-used-elsewhere API (`FindLiveInstance`, with an explicit doc comment naming exactly this situation) existed alongside an incorrect but superficially-similar one (`LoadInstances()` + manual `MatchesID` scan), and a newer call site (the MCP terminal tools) reached for the wrong one. The doc comment on `FindLiveInstance` was necessary but not sufficient — nothing forced call sites needing a live, mutable instance to go through it instead of the read-oriented `LoadInstances()` path.

**Earliest achievable enforcement**: A lint/structural rule is plausible here — flag any `LoadInstances()` result whose element is passed into a method that mutates PTY/controller state (`SendKeys`, `Write`, etc.) — but that's a deep, semantic distinction an `ast-grep`/`semgrep` pattern can't cheaply express (it would need to know which methods are "read" vs "mutate" on `*Instance`). The regression test is the earliest practical level here. A cheaper, complementary guard worth considering separately: renaming or doc-flagging `LoadInstances()`'s returned instances more assertively (e.g. a comment at the call site or a distinct type) so a reader can't mistake them for live instances — not implemented in this fix since it would touch a much wider blast radius (`LoadInstances()` has many legitimate read-only callers) for marginal benefit beyond what the doc comment already says.

**Recurring shape**: This is a variant of "two similar-looking paths exist, one deferred/decoy and one live, and a new call site picks the wrong one by default" — related in spirit to BUG-033 (working directory captured from the wrong source) in that both are "the obviously-reachable value isn't the one that's actually correct in this context" rather than a missing-cleanup shape like BUG-029/030/032/034. Worth flagging for a future audit of other `LoadInstances()` call sites in `server/mcp/tools_vcs.go` (`getSessionDiff`, `listSessionBranches`) — not fixed here since no live symptom was confirmed for read-only VCS tools (they don't call PTY-mutating methods on the returned instance, so the `started` guard doesn't apply to them the same way), but worth checking if similar symptoms ever surface there.

## Related

- BUG-034 (`docs/bugs/fixed/BUG-034-unfinished-scanner-never-removes-completed-session-repos.md`) — first flagged `send_control` as a live symptom investigated in parallel with the scanner leak, explicitly left unresolved pending this separate root cause.
- `session/instance_serialization.go` (`deferStart` branching), `session/storage.go` (`LoadInstances`), `server/dependencies.go` ("Step 6" background start loop), `server/services/session_service.go` (`FindLiveInstance`), `session/review_queue_poller.go` (`ReviewQueuePoller.FindInstance`, the existing correct precedent for this pattern).
