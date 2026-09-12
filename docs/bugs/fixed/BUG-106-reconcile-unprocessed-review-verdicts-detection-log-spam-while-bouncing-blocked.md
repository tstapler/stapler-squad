# BUG-105: `reconcileUnprocessedReviewVerdicts` Re-logs Its Own No-Verdict Detection WARNING on Every ~60s Sweep Tick While the "bouncing" Gate Is Blocked/Parked [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-09-11)
**Discovered**: 2026-09-11, live audit — item `09e91e3e-e13d-4166-a5f2-447242447f77` / session `7ce35db9` logged "review session ... exited without ever writing a verdict — processing as a failed review now" once a minute for 20+ minutes straight, almost certainly continuously since the item's bounce cap was exhausted on 2026-09-09.
**Fixed**: 2026-09-11 — `session/backlog_lifecycle_review.go`
**Impact**: Unbounded WARNING log-line growth for any review-stage item parked on the "bouncing" remediation gate (bounce cap exhausted, `RemediationBlocked` gate closed) with a dead, verdict-less review session as its newest review attempt. Purely a log-volume/noise bug — no stuck-item or data-loss impact, since `autoReopenWithBackoffGate`'s own gating (unchanged) already correctly no-ops the reopen attempt itself.

## Root Cause

This is a leftover, uncovered corner of BUG-046
(`docs/bugs/fixed/BUG-046-unprocessed-review-verdict-sweep-reprocesses-same-dead-session-every-tick.md`),
not a new bug shape. BUG-046 fixed `handleReviewSessionExited`'s no-verdict
branch (`session/backlog_lifecycle_review.go:119-131`) to check
`RemediationBlocked(bouncing)` before its own notify+log, so that downstream
side effect stops repeating once the "bouncing" gate is already known-blocked.
But `reconcileUnprocessedReviewVerdicts` — the periodic sweep that detects a
dead, verdict-less review session and forces it through
`handleReviewSessionExited` (`forcePush=true`) — has its **own** WARNING log
at the detection point (previously line 564), fired *before*
`handleReviewSessionExited` is ever reached:

```go
} else {
    log.WarningLog().Printf("[BacklogLifecycle] item %s: review session %s (the most recent review attempt) exited without ever writing a verdict — processing as a failed review now",
        item.ID, latest.SessionUUID)
}
```

BUG-046's guard never covered this call site. Nothing transitions the item out
of "review" while `autoReopenWithBackoffGate`'s downstream gate is mid-backoff
or parked (bounce cap exhausted) — the same live-DB shape BUG-046 already
established — so on every subsequent ~60s reconcile tick,
`FindReviewItemsWithUnprocessedVerdict` re-matches the identical dead
`SessionUUID`, and this sweep's own detection log fires again, unconditionally,
forever.

## Fix Applied

Apply the identical `RemediationBlocked(bouncing)` dedup check BUG-046 used,
at this sweep's own detection log call site:

```go
} else {
    blocked, blockedErr := l.storage.RemediationBlocked(ctx, item.ID.String(), domain.StuckReasonBouncing)
    if blockedErr != nil {
        log.WarningLog().Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts RemediationBlocked(bouncing) item=%s: %v", item.ID, blockedErr)
    }
    if !blocked {
        log.WarningLog().Printf("[BacklogLifecycle] item %s: review session %s (the most recent review attempt) exited without ever writing a verdict — processing as a failed review now",
            item.ID, latest.SessionUUID)
    }
}
```

Fails open on a query error (still logs) rather than going silent, matching
every other `RemediationBlocked` call site in this file.
`l.handleReviewSessionExited(...)` is still called unconditionally regardless
of `blocked` — this fix only suppresses the redundant WARNING, not the
downstream tombstone/reopen-attempt logic, which was already correctly gated.

Deliberately scoped to only this one branch (the no-verdict detection log).
The sibling branch — `latest.Edges.ReviewVerdict != nil`'s "has an unprocessed
%s verdict — applying it now" log — is structurally similar but was not
touched here: fixing it would require reasoning about whether the recorded
outcome is PASS (which always ships and leaves "review", even under
`forcePush`) vs. FAIL/PARTIAL/UNVERIFIABLE (which routes through the same
`autoReopenWithBackoffGate` gate and could plausibly loop the same way) — a
materially different, unverified change with no live evidence yet. Filed as
BUG-106 instead of guessed at here.

## Files Affected

- `session/backlog_lifecycle_review.go` — `reconcileUnprocessedReviewVerdicts`'s no-verdict detection log now checks `RemediationBlocked(bouncing)` first
- `session/backlog_lifecycle_stuck_test.go` — new regression test

## Verification

- `TestReconcileUnprocessedReviewVerdicts_should_LogDetectionOnlyOnce_AcrossRepeatedSweepTicksWhileBouncingBlocked` — reproduces the realistic timeline: tick 1 fires before any "bouncing" row exists (a genuinely fresh detection — must log once, and does; the ungated auto-reopener also fires), a "bouncing" row then opens mid-backoff between ticks (mirroring `reconcileBouncingItems` tripping independently), then tick 2 reprocesses the identical dead `SessionUUID` with the gate now blocked and asserts the captured WARNING buffer still contains the detection message exactly once (not twice) and the auto-reopener is not invoked again.
- **Verified to fail against pre-fix code**: reverted the fix in `session/backlog_lifecycle_review.go` only (keeping the new test), reran — failed with `Not equal: expected: 1 / actual: 2` on the tick-2 assertion, exactly the pre-fix double-log behavior. Restored the fix; test passes.
- `go build ./...` — clean.
- `go test ./session/...` (full package, `-timeout=20m`) — all green, no regressions.
- `go test ./session -race -run 'TestReconcileUnprocessedReviewVerdicts|TestHandleReviewSessionExited|TestAutoReopenWithBackoffGate'` — green.
- `CGO_ENABLED=0 golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./session/...` — 0 issues.
- `golangci-lint run --enable=gocyclo,gocognit,funlen,revive,dupl --new-from-rev=origin/main ./session/...` (this repo's new-code-only complexity/duplication gate, mirroring `.github/workflows/lint.yml`) — 0 issues.
- `gofmt -l` on both changed files — clean.

## Reflection

**Classification**: Same family as BUG-046 itself — "a notify-once/dedup gate added at one call site, left uncovered at a sibling call site with the identical structural shape." Not a new root cause; a direct extension of BUG-046's own scope gap.

**Earliest achievable enforcement**: The regression test is the earliest practical level — this is idempotency against a stateful DB-backed backoff gate, not something a type system or lint rule can express. `RemediationBlocked` (BUG-043) continues to serve as the one shared, documented primitive for "is this exact condition already known and gated" — this fix is its third call site (after `handleReviewSessionExited`'s own no-verdict branch and `markAbandonedReview`).

**Recurring shape**: "A gate/dedup check applied at the call site a bug report or live incident happened to surface, not audited across every structurally-identical sibling call site in the same function/file." This is now the second time this exact function (`reconcileUnprocessedReviewVerdicts` / `handleReviewSessionExited`) has needed the identical guard applied at a second location after the first fix (BUG-046 fixed `handleReviewSessionExited`'s branch; this fix fixes `reconcileUnprocessedReviewVerdicts`'s branch). BUG-106 (filed, not fixed) documents a third, structurally identical candidate spot (the verdict-present branch's FAIL/PARTIAL/UNVERIFIABLE case) that was deliberately left unfixed pending live confirmation rather than patched speculatively. A future pass fixing BUG-106 should also grep this file for any other bare `log.WarningLog()` call inside the `reconcileUnprocessedReviewVerdicts`/`handleReviewSessionExited` pair that isn't already gated, rather than assuming these two fixes exhaust the class.

## Related

- Direct follow-up to BUG-046 (`docs/bugs/fixed/BUG-046-unprocessed-review-verdict-sweep-reprocesses-same-dead-session-every-tick.md`), reusing its `Storage.RemediationBlocked` primitive (from BUG-043) rather than introducing a new one.
- BUG-106 (open) — the sibling verdict-present branch's latent, unverified risk of the same shape.
