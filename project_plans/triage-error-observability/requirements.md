# Requirements: triage-error-observability

**Item:** Auto-triage: 16 backlog items exhausted retries with unclassified 'other' errors, permanently parked  
**Item ID:** 4daf7ced-3f75-45c8-93de-ef74403f9975

## Problem Statement

16 backlog items from a bulk import on 2026-08-06 have permanently hit `MaxRemediationAttempts` (5)
and are parked in `idea`/`queued`. Every one failed with `errType=other` from
`classifyHeadlessCallError` (`server/services/backlog_service_triage.go:2689`).

`errType=other` is the catch-all bucket that covers "anything else, including the claude subprocess
running and exiting non-zero." When a failure lands in `other`:

- The **log line** (`[TriggerTriage] headless triage failed`, line 550–551) does include `"error",
  callErr` in full — but logs are ephemeral. The service has restarted since, and these log records
  are gone.
- The **DB** (`item_sessions.end_reason`) stores only the string `"other"` with no further context —
  no error message, no error type detail. This is the permanent, durable record, and it yields zero
  diagnostic signal.
- The **failure capture file** (`captureHeadlessFailure`) stores raw subprocess stdout, which only
  helps if the subprocess produced output. For errors that occur before or outside subprocess output
  (e.g. storage errors, early Go errors), the capture file is absent or empty.

The result: when 16 items park with `other`, there is no way to determine why from logs or DB after
the fact. The concurrent bulk-import pattern (16 items created within seconds) strongly suggests a
shared root cause, but it cannot be diagnosed.

A secondary gap: the parking notification is sent per-item (one WARN notification per item), not
aggregated. Sixteen simultaneous parkings produce sixteen low-context notifications with no signal
that they share a root cause.

## Acceptance Criteria

### AC-1: Error detail preserved durably for `other`-bucket failures
When `classifyHeadlessCallError` returns `"other"`, the underlying `error.Error()` string (truncated
to a reasonable max, e.g. 500 chars) is persisted durably alongside the `end_reason` so a future
post-incident query against the DB can recover it without needing live logs.

### AC-2: Error detail visible in existing diagnostic surfaces
The error detail added in AC-1 is surfaced in the `ItemSession` proto / UI where `end_reason` is
already shown (backlog item detail's session history panel), not hidden in a new schema-only field.

### AC-3: `classifyHeadlessCallError` unit-tested for `other` logging behavior
A unit test verifies that a non-nil error that doesn't match any known sentinel produces `"other"` AND
that the caller's error text is preserved in the persisted record (not just the bucket name).

### AC-4: Batch-parked items escalation
When `retryOrphanedTriageWithBackoffGate` calls `justParked=true` for ≥ N items within a short
window (configurable, default N=3 items in 10 minutes), a single aggregated notification is emitted
(in addition to per-item notifications) naming the count and suggesting the operator check for a
shared root cause (e.g. WIP-cap contention, environment issue).

### AC-5: `errType=other` log line includes a truncated inline error summary
The existing log line in `TriggerTriage` already logs `"error", callErr`. Ensure the same is true
for `TriggerReReview` and any other `classifyHeadlessCallError` call sites — no call site should log
only `errType` without also logging the underlying error in the same structured log call.

### AC-6: Post-incident runbook entry
A brief note is added to the existing `docs/how-to/debug-with-logs.md` (or equivalent reference)
documenting how to query for parked-with-`other` items, what the new error detail field contains,
and how to reset a batch of parked items.

## Out of Scope

- Re-triaging the 16 already-parked items from the 2026-08-06 incident.
- Fixing the underlying cause of those 16 failures (unknown; this item creates the observability to
  diagnose the next occurrence).
- Changes to `MaxRemediationAttempts` or the retry backoff schedule.
- The 3 items still retrying with `errType=timeout` — separate issue.
