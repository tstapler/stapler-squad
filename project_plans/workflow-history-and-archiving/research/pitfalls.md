# Pitfalls and Tradeoffs: Workflow History and Archiving

## ListSessions Default Filter Change Impact on Internal Callers

`ListSessions` now silently drops sessions with `ArchivedAt != nil` unless the caller passes `IncludeArchived: true`. This is a behavioral change to a widely-used RPC. Internal callers that iterate all sessions — the review queue poller, the `loadInstancesWithWiring` path, and any analytics code — will no longer see archived sessions in their result set. The implementation mitigates this by applying the filter only in `ListSessions` (the RPC handler), not in `storage.LoadInstances` or `reviewQueuePoller.GetInstances()`. Live in-memory instances remain fully visible to internal code that reads `reviewQueuePoller.GetInstances()` directly. However, any future internal caller that goes through the RPC layer (or `ListSessions` as a library function) will need to pass `IncludeArchived: true` explicitly or it will miss archived sessions.

## Dead-tmux Resume: Stale TmuxSession Object Requires Reinit

After `Pause()` kills the tmux session, the `TmuxBackend`'s internal `TmuxSession` object holds a stale reference: its command string was built at initial launch time (potentially without `--resume <uuid>` if the session was brand new). `Resume()` must call `buildLaunchCommand(claudeSessionID)` with the latest stored UUID and then call `tb.TmuxManager().SetSession(newSession)` to replace the stale object before calling `pm().Start()`. Forgetting the `SetSession` call would cause the resumed session to relaunch without the `--resume` flag, starting a fresh Claude conversation instead of reattaching. This reinit is guarded by a type assertion `if tb, ok := i.processManager.(*TmuxBackend); ok`, which means non-tmux backends (e.g., test fakes) silently skip it.

## Auto-Archive Goroutine Safety

`maybeAutoArchive` runs in a goroutine spawned from `OnLifecycleEvent`. It reads `inst.WorkflowID` and `inst.ArchivedAt`, then writes `inst.ArchivedAt`. The Instance `stateMutex` is not held during this call. The guard `inst.ArchivedAt != nil` prevents double-archive but is not itself mutex-protected, so two concurrent `EventExited` fires (e.g., from a race between a controller exit and a forced kill) could both pass the nil check and both set `ArchivedAt`. The result is a benign double-write of the same timestamp value, not a corruption, but this is a known race. Any auto-archive logic that performs more expensive idempotency checks (e.g., a database read before write) would need the mutex or a compare-and-swap.

## Pause Kill Fallback to Detach on Error

The kill-then-detach fallback preserves session accessibility over correctness of the memory-saving goal. If `KillSession()` fails (e.g., tmux is unresponsive, the session name has changed due to a rename race), the session is detached rather than killed. The process tree remains alive, defeating the RAM-saving intent of US-6 for that session. The failure is logged at `Warn` level, so it is visible in logs but will not surface as an error to the user. Operators on memory-constrained devices should monitor `logs/stapler-squad.log` for repeated `pause: failed to kill tmux session` lines, which indicate a systemic issue (e.g., tmux version incompatibility with the kill subcommand).
