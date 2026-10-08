# Validation: triage-error-observability

## Adversarial Review

### Challenge 1: "Adding a new ent schema field is more churn than encoding in end_reason"

**Claim:** Just append `:error text` to the `end_reason` value (e.g. `"other:dial tcp refused"`)
to avoid a schema migration.

**Verdict: Rejected.** `end_reason` is used as a discrete key in multiple places:
- `retryOrphanedTriageWithBackoffGate:401` compares `EndReason == TriageEndReasonFanoutCeiling`
- `tombstoneOrphanTriageSessions:434` compares reason to `"shutdown"` for carve-out logic
- `reconcileOrphanedTriageItems` builds context strings keyed on `EndReason`

Encoding additional text into `end_reason` would break every exact-match comparison silently. A
new dedicated field is the correct approach.

### Challenge 2: "AC-4 (batch notification) is over-engineering — just fix the log"

**Claim:** The root problem is log loss after restarts. Fix that (AC-1) and batch escalation isn't
needed.

**Verdict: Partially accepted.** AC-1 fixes future incidents. But 16 items parking individually
still produces 16 notifications with no signal they share a root cause. The batch escalation is
low-effort (in-memory counter, ~2h) and delivers a qualitatively different signal: "these are
probably related." It is kept in scope but scoped down — the existing per-item notifications stay;
the batch notification is *additional*, not a replacement.

### Challenge 3: "The actual root cause (why did 16 items get `other`?) is not addressed"

**Claim:** This item should investigate and fix the underlying failure, not just add observability.

**Verdict: Out of scope per the item description.** The item explicitly says: "this issue is about
the automation gap (unclassified failures, no auto-recovery signal), not about individually
re-running triage on each idea." Without the actual error text (lost to log rotation), the root
cause cannot be determined. The observability improvements here make the root cause diagnosable for
the next occurrence.

### Challenge 4: "The `error_detail` field might expose sensitive data (diff content, API keys)"

**Claim:** Go error strings can include stack frames with file paths, or subprocess output
fragments that may contain secrets.

**Verdict: Valid concern, mitigated.** `classifyHeadlessCallError`'s `other` bucket is reached
when none of the specific error sentinels match. In practice this means:
- Wrapped standard library errors (network, I/O) — unlikely to contain secret content
- The error string is truncated at 500 chars

The primary risk would be if the subprocess output leaked into the error message. But
`captureHeadlessFailure` handles raw subprocess output separately via `failure_capture_path` (a
file, not in DB). The Go `callErr` itself comes from `headless.Pool.callBlocking`, which wraps
sentinel errors — not raw subprocess output.

**Mitigation already in place**: `failure_capture_path` handles subprocess stdout. `error_detail`
captures only the Go error chain, not raw subprocess output. Add a comment to this effect on the
new field.

### Challenge 5: "truncateErrorDetail uses `len(s)` (byte count) not rune count"

**Claim:** For UTF-8 strings, byte-based truncation can split a multi-byte rune.

**Verdict: Valid.** Use `[]rune(s)[:maxLen]` or `utf8.RuneCountInString` for the truncation.
Minor fix, document in the code.

### Challenge 6: "batchParkThreshold=3 in 10m could fire spuriously during normal rolling retries"

**Claim:** Normal backlog operation might naturally have 3+ items park in a 10-minute window during
a high-activity period.

**Verdict: Low risk.** Parking (hitting `MaxRemediationAttempts=5`) requires 5 failed triage
attempts spread across the backoff schedule (days, not minutes). Getting 3+ items to park within
10 minutes requires either a bulk-import event or a systemic infrastructure failure — exactly the
cases this alert is designed to catch. False positive rate is expected to be very low.

## Correctness Checks

| Check | Status |
|---|---|
| Adding `error_detail` to ent schema is backward-compatible (nullable/default) | ✓ |
| `UpdateItemSessionEndedWithReason` callers unchanged (signature not modified) | ✓ |
| Proto field 19 not taken by existing messages | Verify before commit |
| `reconcileOrphanedTriageItems` end_reason comparisons still work (only exact-match checks) | ✓ (new field is separate) |
| Batch counter is not persisted — resets on restart | ✓ (acceptable; rare edge case) |
| `truncateErrorDetail` handles nil error | ✓ (returns "") |
| UTF-8-safe truncation | Must use rune-count, not byte-count |

## Definition of Done

- [ ] `make build && make test` pass
- [ ] `make lint` passes
- [ ] A triage ItemSession with an unclassified error shows non-empty `error_detail` in the UI
- [ ] `error_detail` appears in the stuck-context string when `end_reason="other"`
- [ ] Batch parking notification fires when ≥ 3 items park within 10 minutes (unit test)
- [ ] Runbook entry added to `docs/how-to/debug-with-logs.md`
