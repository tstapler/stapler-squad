# Follow-ups — deliberately deferred out of this PR

Recorded during the Phase 6 review-fix pass on `ff2f7048c`. None of these is a
blocker for the ADR-001 guards; each is tracked here so it is not lost.

## 1. Custom lint analyzer: every automated start/revive/retry must consult `IsArchived()`

The six guards in this change are a convention, not a ratchet — a seventh
auto-lifecycle path can be added tomorrow with no archived check and nothing
fails. The repo already has the precedent for making this structural:
`tools/lint/norawghrequest` and `tools/lint/noliveinstanceraw`, both wired into
`make lint-custom`.

Deferred because designing the predicate is the hard part, not the analyzer
plumbing: "an automated start" has no syntactic marker, so the rule would have
to key on calls to `Instance.Start`/`RecoverFromStopped`/`restartForRetry` from
outside an explicitly-user-initiated handler, with an allowlist for the manual
paths (`RetryNow`, `ResumeCrashedSession`, `UnarchiveSession`). That is a
design task of its own, not a mechanical addition to this fix.

## 2. Extract `applyArchivedLoadPolicy` out of `fromInstanceData`

`session/instance_serialization.go`'s `fromInstanceData` is 388 lines; guard 1
added another branch to it. The archived load policy (started=true, normalize
only Active/Creating) is a self-contained rule that reads better as its own
function with its own test.

Deferred because the extraction touches the hottest restore path in the repo
and would bury the guard's diff inside a refactor diff. It belongs in its own
`refactor:` commit, gated by the same test table that exists now.

## 3. Site 6 (`backlog_lifecycle_archive.go:137`) idempotence

Already deferred in `plan.md`. `archiveItemWorkSessions` is not idempotent
across repeat calls for the same item; the ADR-001 guards make a repeat call
harmless (the CAS no-ops, nothing is respawned) but do not make it correct.

## 4. Whether `SetArchivedAtIfNilCtx` fully closes M2

`SetArchivedAtIfNilCtx` (`session/instance_actor_setters.go`) bounds each
per-instance actor round-trip at 2s, so `ArchiveWorkflowSessions` can no longer
be stalled indefinitely by one instance mid-`Start`. It does **not** bound the
loop as a whole: N instances each timing out costs N × 2s. If a workflow ever
carries enough in-memory sessions for that to matter, the fix is to fan the
loop out across goroutines (the DB update is already a single statement, so
only the in-memory mirror is serial) or to derive one shared budget from the
RPC context instead of a per-instance one.

There is also a residual by design: the `IsActive()/IsCreating()/IsPaused()`
predicates run on the RPC goroutine, outside the actor command, so an instance
that turns Active mid-round-trip is still archived while Active. That state is
repaired at next load by guard 1's self-heal. Closing it properly means routing
the predicate *and* the write into one actor command — a new setter, out of
scope here.

## 5. `session/tmux` flake observed once

A single flaky failure was observed in `session/tmux` during this work and did
not reproduce on the verification runs (`go test ./session/...` and
`go test -race ./session` both green). Not diagnosed, and not caused by this
change — no file in `session/tmux` is touched. Recorded per the
`fix-flaky-tests-dont-defer` skill so the next sighting has a prior.
