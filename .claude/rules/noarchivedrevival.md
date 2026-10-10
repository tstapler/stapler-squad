# Every Automated Start/Revive/Retry Must Check Archival First

In the automated-lifecycle files, a call to `Start(false)`, `RecoverFromStopped()`, `restartForRetry(...)`, or `transitionToLocked(..., Active)` must be preceded by an archival check (`IsArchived()` or an `ArchivedAt != nil` comparison) — never revive a session that was deliberately archived.

Files in scope (`tools/lint/noarchivedrevival`): `session/health.go`, `session/review_queue_poller.go`, `session/session_driver.go`, `session/instance_claude.go`, `session/instance_serialization.go`, `server/dependencies.go`.

**Wrong:**
```go
func recoverMissingSession(instance *Instance) {
	if err := instance.Start(false); err != nil { // revives an archived session
		...
	}
}
```

**Right:**
```go
func (h *SessionHealthChecker) checkSingleSession(instance *Instance) {
	if reason, skip := healthCheckSkipReason(instance); skip { // reads snap.ArchivedAt
		return
	}
	h.checkTmuxHealth(instance) // -> recoverMissingSession -> Start(false)
}
```

A check in an outer caller counts when the revival lives in an unexported function whose every in-package caller is guarded. A predicate-shaped helper (last result `bool`) whose own body checks archival counts as a check.

## Escape hatch

`//nolint:noarchivedrevival <one-line reason>` on the call's line or the line above. A bare `//nolint:noarchivedrevival` with no reason is itself reported. Use it only for an explicit user action that is meant to resume an archived session:

```go
//nolint:noarchivedrevival user clicked Resume on an archived session
err := inst.Start(false)
```

Explicit-user files (`instance_hibernate.go`, `instance_crash.go`, `import_commit.go`, `retry_state.go`'s `RetryNow`, MCP hydration) are outside the file set and need no annotation.

## Limits

The check is lexical, not full dominance: a guard counts if it appears earlier in a statement list that encloses the call. Matching is by simple name, so same-named methods on different types share call sites. It does not follow calls across packages.

## Why

PR #810 fixed superseded rework sessions resurrecting themselves by adding `ArchivedAt` guards at seven auto-revival sites; before it, none of them read `ArchivedAt`. The count grew 2 → 4 → 6 → 7 over four review rounds, each increment found only by someone searching again. The invariant had lived only in ADR-001 and a manual `rg '\.Start\((false|true)\)'` recipe, so a site added later by someone who hadn't read the ADR would regress silently. The write side was already ratcheted by `make actor-field-guard`; this analyzer ratchets the read side.

Verified against the repo: at `8b13472a6` (pre-#810) the analyzer reports 13 revival calls across all six files; at `e279a56a0` (post-#810) it reports none.
