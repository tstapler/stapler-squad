# ADR-002: RulesService-Level Mutex for Reload Serialization (Not Classifier-Level Locking)

**Status**: Accepted
**Date**: 2026-08-06
**Project**: dynamic-rule-reload

## Context

`RuleBasedClassifier.ReplaceRules()` (`pkg/classifier/classifier.go:392-400`) is already
atomic for what it does: it builds a sorted slice off-lock, then swaps `c.rules` under a
short `Lock()`. That part is correct and untouched by this project.

The bug is one layer up. `RulesService.rebuildClassifier()` (`server/services/rules_service.go:431-443`,
the existing DB-rule reload path triggered by `UpsertApprovalRule`/`DeleteApprovalRule`/
`BulkUpsertRules`) does an **unsynchronized read-filter-replace**:

```go
existing := rs.classifier.Rules()       // read
var nonUser []classifier.Rule
for _, r := range existing { ... }      // filter
rs.classifier.ReplaceRules(append(nonUser, userRules...))  // write
```

This project adds a second, independent trigger for the same shape of operation:
`rebuildClaudeSettingsRules()`, called both from fsnotify events (`ClaudeSettingsWatcher`)
and the new `ReloadClaudeSettingsRules` RPC. Concretely:

1. Goroutine A (`rebuildClassifier`, triggered by a user upserting a new `auto_deny` rule)
   reads `existing` — a snapshot that does *not* yet include a claude-settings change B is
   about to make.
2. Goroutine B (`rebuildClaudeSettingsRules`, triggered by an fsnotify event) reads its own
   `existing` snapshot, filters, and calls `ReplaceRules` first — the classifier now reflects
   B's claude-settings update plus the *stale* pre-A user rules.
3. Goroutine A finishes its own filter (computed from A's earlier, now-stale read) and calls
   `ReplaceRules` — this clobbers B's just-written claude-settings update, reverting to
   whatever claude-settings rules existed in A's stale snapshot.

Either ordering silently drops one side's update. The most concerning case: a user's
just-added `auto_deny` rule for a destructive command is silently reverted, or a
claude-settings tightening is silently reverted in favor of stale looser rules. This is the
same failure shape `.claude/rules/go-double-checked-locking.md` warns about generalized to a
two-independent-writers race instead of a cache-miss race.

## Decision

Add a single `sync.Mutex` field, `rebuildMu`, to `RulesService` (not to
`RuleBasedClassifier`). Hold it for the **entire** read-filter-replace sequence in both
`rebuildClassifier()` and the new `rebuildClaudeSettingsRules()`. This serializes the two
triggers into one critical section: whichever goroutine acquires `rebuildMu` first completes
its full read-filter-replace before the other begins its read, so the second goroutine's
filter is always computed from a snapshot that includes the first goroutine's write.

Both functions are also switched from exclusion-based filtering (`if r.Source != "user"`)
to symmetric allow-list filtering via a new `filterRulesBySource(rules, allowed...)` helper —
`rebuildClassifier` allow-lists `{seed, claude-settings}`, `rebuildClaudeSettingsRules`
allow-lists `{seed, user}`. This closes a secondary latent bug: an exclusion filter silently
keeps rules from any future 4th source that neither filter's exclusion list happens to
mention, while an allow-list requires every rebuild path to explicitly opt a new source in.

## Consequences

- **Positive**: closes the lost-update race with the smallest possible primitive; no change
  to `RuleBasedClassifier`'s public API or internal locking; both rebuild paths become
  trivially easy to reason about (each is now a strict critical section).
- **Negative**: `rebuildClassifier()` and `rebuildClaudeSettingsRules()` cannot run
  concurrently even when they'd touch disjoint rule sources in practice (they always touch
  the *whole* slice via read-filter-replace, so this isn't a real loss of parallelism —
  reload frequency for both paths is human-edit-paced, not a hot path).
- **Negative**: a third or fourth rebuild path added later (e.g. a hypothetical future
  config-file-rules reload) must remember to also acquire `rebuildMu` — this is a discipline
  requirement, not a compiler-enforced one. Mitigated by both existing rebuild functions
  living in the same file (`rules_service.go`) directly adjacent to the mutex declaration and
  its doc comment.

## Alternatives Considered

1. **Lock inside `RuleBasedClassifier`** (e.g. a `ReplaceRulesIf` / compare-and-swap style
   API, or exposing a classifier-level `Lock()`/`Unlock()` for callers to hold across their
   own read-filter-replace) — rejected. `ReplaceRules` is already correctly atomic for the
   slice-swap; the actual bug is in the *caller's* multi-step composition (read this
   service's own view of "non-X rules", filter, write), which the classifier has no
   visibility into and shouldn't need to. Pushing the lock down would also force
   `RuleBasedClassifier` to know about `RulesService`'s specific reload semantics
   (which sources exist, which rebuild call is "in progress"), coupling a generic
   priority-ordered-rule-evaluator to one specific caller's business logic.
2. **Full Unit-of-Work-style transactional rebuild abstraction** (tracking "dirty" rule
   changes across a broader transaction with explicit commit/rollback, per PoEAA's Unit of
   Work pattern) — rejected as over-engineering. Unit of Work solves the problem of
   coordinating multiple *different* object changes across a business transaction with
   partial-failure rollback; here there are exactly two call sites doing the identical
   read-filter-replace shape, and a plain mutex fully closes the race with no rollback
   semantics needed (a failed reload never partially applies — `LoadClaudeSettingsRulesDetailed`
   is fail-safe per-path, and `ReplaceRules` is atomic for the final write).
3. **Optimistic concurrency (version counter + retry on conflict)** — rejected as
   unnecessary complexity for a low-contention, human-edit-paced workload; a mutex with
   effectively no contention in practice is simpler to verify correct and simpler to read.
