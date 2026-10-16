# ADR-004: Rule-Reconciliation Runs Asynchronously After the Rule Swap, Outside `rebuildMu`

**Status**: Accepted
**Date**: 2026-09-04
**Project**: notification-revamp

## Context

Two research documents reach conclusions that are individually correct but
in tension if read as one instruction:

- `architecture.md` §4 proposes calling `rs.reconcilePendingApprovals()`
  directly inside `rebuildClassifier()`/`rebuildClaudeSettingsRules()`,
  **while `rebuildMu` is still held**, reasoning that this is safe because
  `Classify()` uses the classifier's own separate `classifier.mu`, not
  `rebuildMu` — the same nesting pattern `rebuildClassifier()` already uses
  when it calls `rs.classifier.Rules()` under `rebuildMu`.
- `pitfalls.md` §4 warns that running reconciliation **synchronously inside
  the triggering RPC handler** (`UpsertApprovalRule`, `ReloadClaudeSettingsRules`)
  blocks that RPC's response for the full duration of N `Classify()` calls
  plus any live-instance lookups (`SessionIdleMinutes`), and recommends
  reconciliation run asynchronously after the rule swap completes.

`dynamic-rule-reload`'s ADR-002 (which introduced `rebuildMu`) explicitly
anticipated a "third or fourth rebuild path" being added later and warned
that any such addition "must remember to also acquire `rebuildMu`... a
discipline requirement, not a compiler-enforced one." Reconciliation is
exactly that anticipated addition — the question is *how* it participates.

## Decision

**Reconciliation does not participate in `rebuildMu` at all, because it
never performs a read-filter-replace against the classifier's rule list —
the thing `rebuildMu` exists to serialize.** `reconcilePendingApprovals()`
only *reads* the already-fully-rebuilt rule set (via `Classify()`, which
takes its own `classifier.mu.RLock()` internally) and resolves approvals
through `ApprovalService`. It needs exactly one ordering guarantee: it must
start only after the current `ReplaceRules()` call has fully completed, so
it evaluates the fresh rule set rather than a rule set mid-swap.

That guarantee is satisfied by spawning it as a goroutine **after**
`rebuildMu.Unlock()`, inside `rebuildClassifier()`/`rebuildClaudeSettingsRules()`
themselves:

```go
func (rs *RulesService) rebuildClassifier() {
	rs.rebuildMu.Lock()
	userRules := rs.rulesStore.ToRules()
	existing := rs.classifier.Rules()
	rs.afterRebuildReadHook()
	nonUser := filterRulesBySource(existing, classifier.SourceSeed, classifier.SourceClaudeSettings)
	rs.classifier.ReplaceRules(append(nonUser, userRules...))
	rs.rebuildMu.Unlock()
	go rs.reconcilePendingApprovals()
}
```

This replaces the file's existing `defer rs.rebuildMu.Unlock()` idiom with
an explicit unlock at exactly this one call site, deliberately, so the
goroutine spawn happens after the lock is released rather than after the
function returns (which `defer` would otherwise make simultaneous with the
unlock, not clearly ordered after it in the reader's mental model).

**The triggering RPC (`UpsertApprovalRule`, `DeleteApprovalRule`,
`ReloadClaudeSettingsRules`) returns as soon as the rule swap itself
completes**, not after reconciliation finishes — satisfying pitfalls.md's
latency concern without needing reconciliation to hold or wait on
`rebuildMu`.

**Idempotency under concurrent reconciliation passes** (e.g. two rapid rule
edits each spawning their own goroutine) is already guaranteed by
`ApprovalStore.Resolve()`'s existing delete-under-lock semantics
(`approval_store.go:180-205`): whichever pass's `ResolveApprovalReconciled`
call reaches a given approval ID first wins; the other gets a clean "already
resolved" error and skips it (pitfalls.md's Idempotency finding, confirmed
by direct read).

## Alternatives Considered

- **Run reconciliation synchronously, still holding `rebuildMu`**
  (architecture.md's original proposal). Rejected: while not a deadlock
  risk, it makes every rule-change RPC's latency proportional to the size
  of the pending-approval backlog at that moment — a rule edit with dozens
  of pending items outstanding would make `UpsertApprovalRule` visibly slow,
  for no correctness benefit (reconciliation doesn't need the lock, it only
  needs the swap to be *done*).
- **A separate, dedicated mutex for reconciliation.** Rejected as
  unnecessary ceremony: reconciliation doesn't mutate any shared state that
  isn't already protected by `classifier.mu` (inside `Classify()`) and
  `ApprovalStore`'s own `s.mu` (inside `Resolve()`) — introducing a third
  lock would protect nothing new.
- **A bounded worker queue/channel instead of a bare `go` per rebuild.**
  Rejected as over-engineering for this project's Appetite: at
  single-operator scale, two rule edits landing within microseconds of each
  other (the only scenario a queue would meaningfully change) is not a
  realistic concurrency profile to design against; the soft per-pass cap
  (`maxReconcileAutoResolvesPerPass`, Task 2.2.1a) already bounds the cost
  of any one pass regardless of how many fire concurrently.

## Consequences

- `rebuildClassifier()`/`rebuildClaudeSettingsRules()` each gain one line
  (the `go rs.reconcilePendingApprovals()` call) and lose their `defer`
  in favor of an explicit unlock — a small, deliberate deviation from the
  file's otherwise-uniform `defer rs.rebuildMu.Unlock()` idiom, called out
  in both functions' doc comments so a future reader doesn't "fix" it back
  to `defer` without understanding why the goroutine spawn needs to happen
  after the unlock, not merely after the function returns.
- A test asserting reconciliation ran needs to wait on the new
  `reconcileDoneHook` (Task 2.2.1c) rather than asserting synchronously
  right after calling `rebuildClassifier()` — this is the same test
  discipline `afterRebuildReadHook` already established for the rebuild's
  own critical section, extended one step further.
- A crash between `ReplaceRules()` completing and the reconciliation
  goroutine running (extremely narrow window, process-level) would leave a
  now-coverable pending approval unreconciled until the *next* rule change
  — acceptable for a single-operator internal tool with no feature flag or
  SLA, and no worse than today's behavior (reconciliation doesn't exist
  today at all).
