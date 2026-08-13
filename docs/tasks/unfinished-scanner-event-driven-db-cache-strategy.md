# Strategy: Make the Unfinished-Changes Scanner Fully Event-Driven and DB-Cached

**Status**: Proposal — not implemented. Written 2026-07-22 following the BUG-034 fix (the
scanner's watch list leak). This is the requested follow-up strategy for closing the
remaining gaps between the current architecture and "surface changes, trigger updates
asynchronously only when needed, minimal reprocessing, persist results instead of
recomputing them in memory on every restart."

## Current State (give credit where due — this is more event-driven than it first looks)

`session/unfinished`'s `Scanner` already has real event-driven infrastructure, not a naive
poll loop:

| Mechanism | What it does | File |
|---|---|---|
| fsnotify watch per repo | A real filesystem change to a watched `.git` dir triggers an immediate, *targeted* rescan of just that repo — this is the primary trigger today, not the ticker | `scanner.go` `fsnotifyLoop`, `watchRepo`/`unwatchRepo` |
| Session-lifecycle subscription | `EventSessionCreated`/`EventSessionUpdated` auto-spider a session's repo into the watch set the moment it exists — no manual registration needed | `scanner.go` `subscribeToSessionEvents` (now also handles `EventSessionDeleted`, added in the BUG-034 fix this doc follows) |
| Circuit breaker | A repo that times out 3 times in a row backs off for 5 minutes instead of being retried every tick | `scanner.go` `shouldScan`/`recordTimeout` |
| 30s TTL result cache | `worktreeCache` short-circuits a repeat scan within 30s of the last one | `cache.go` |
| Budget-based cache pruning (every 1 min) | Evicts cold/over-budget cache entries proactively so memory pressure doesn't have a chance to build up; escalates to a full `ClearCache` only under severe pressure | `scanner.go` `Start`'s 1-minute ticker |
| 5-minute self-pruning backstop | Drops any registered repo whose path no longer exists on disk (added in the BUG-034 fix) | `scanner.go` `pruneMissingRepos` |

**What's still missing relative to the ask:**

1. **The 5-minute coordinator tick (`enqueueAll`) unconditionally re-scans every registered
   repo, not just repos that plausibly changed.** fsnotify is the primary trigger for *real*
   changes, but the tick is a blanket "just in case fsnotify missed something" sweep with no
   staleness check of its own — `shouldScan`'s circuit breaker only gates on *recent
   timeouts*, not on "has anything about this repo actually changed since the last scan." At
   ~130 registered repos (now correctly shrinking again post-BUG-034, but still large), this
   tick alone was doing a meaningful chunk of the 63.7GB/32min allocation churn that led to
   this investigation.
2. **No scan result is ever persisted.** `resultStore` (the published `ScanResult` per
   `repoPath|branch`) and `cacheStore` (`worktreeCache`, the 30s short-circuit) are both plain
   in-memory `sync.Map`s. Every service restart starts from a completely cold cache — every
   registered repo needs a full, real scan again before the "unfinished changes" UI has
   anything to show. This directly compounded the BUG-034 leak: the restart that triggered
   this whole investigation forced a cold-start scan across an already-inflated, leaked repo
   list, which is a plausible large contributor to the acute memory pressure observed
   immediately after that restart.
3. **fsnotify only catches direct filesystem writes to `.git`.** It doesn't know about
   git-level state changes that don't touch `.git` on this box — e.g., a worktree whose
   *base* branch (`main`) advanced on `origin`, changing what "unfinished changes" even means
   for that worktree, isn't itself an fsnotify-visible event locally until something local
   also changes. (Lower priority — likely acceptable as-is, noted for completeness.)

## Proposed Design

### 1. Persist scan results in the database, not just in memory

Add an ent schema (new table, not reusing `DiffStats` — that's a 1:1-with-`Session` schema for
a different purpose, per-session line-added/removed counts, not a standalone per-repo scan
result keyed independent of any specific session):

```go
// session/ent/schema/unfinished_scan_result.go (sketch)
type UnfinishedScanResult struct{ ent.Schema }

func (UnfinishedScanResult) Fields() []ent.Field {
	return []ent.Field{
		field.String("repo_path"),
		field.String("branch"),
		field.String("worktree_path").Optional(),
		field.Bool("has_unfinished_changes"),
		field.Int("files_changed").Default(0),
		field.String("last_commit_sha").Optional(),
		// The staleness key: if this doesn't match what's actually on disk
		// at scan-decision time, the cached row is known-stale and a real
		// rescan is warranted. Comparing this single field is far cheaper
		// than re-walking/re-diffing the repo just to find out nothing changed.
		field.String("head_ref_hash").Optional(),
		field.Text("summary_json").Optional(), // the full ScanResult, serialized
		field.Time("scanned_at"),
	}
}
// Unique index on (repo_path, branch).
```

On `Scanner.Start`, load all rows into `resultStore`/`cacheStore` before the first tick or
fsnotify event ever fires — the "unfinished changes" UI has real (if momentarily stale) data
immediately after a restart instead of an empty state until the first full cold scan
completes. Every successful scan writes its result back to this table (async, best-effort —
a failed write should never block or fail the in-memory update consumers already read from).

### 2. Make the periodic backstop staleness-aware instead of unconditional

Before `enqueueAll`'s tick does real work for a repo, compare a **cheap** staleness signal —
the repo's current `HEAD` ref hash (a single, fast git call, not a full diff) — against the
persisted `head_ref_hash` from the last scan. Only enqueue an actual rescan if they differ.
This turns the "backstop for anything fsnotify missed" tick from "unconditionally redo
everything" into "cheaply check everything, expensively rescan only what fsnotify plausibly
missed" — directly the "minimal reprocessing" goal. The existing circuit breaker and 30s TTL
cache stay as they are; this is an additional, earlier-and-cheaper gate before either of
them.

### 3. Keep fsnotify as the primary trigger; this proposal doesn't change that

fsnotify already correctly makes "something changed" event-driven and targeted at just the
repo that changed. Nothing here proposes replacing it — the changes above shrink the cost of
the *other* two triggers (the periodic backstop, and cold-start-after-restart) that
currently do far more work than fsnotify's targeted path.

## What This Doesn't Change

- `AddRepo`/`RemoveRepo`/the fsnotify wiring itself — those are correct as of the BUG-034 fix
  and don't need to change for this proposal.
- The 30s `worktreeCache` TTL and the 1-minute budget-based memory pruning — both already
  serve their purpose (short-lived hot-path dedup, and general memory-pressure response,
  respectively) and are orthogonal to result *persistence*.

## Scope and Sizing

This is a genuine architecture change — new ent schema + migration, a new read-on-startup
path, a new write-on-scan-completion path, and a new staleness-check gate in the coordinator
— not a quick patch. Per this repo's own convention for changes of this shape (`.claude/rules/
session-creation-registry.md`'s "many touchpoints must move in lockstep" pattern, and the
`backlog-feature-improvement` skill's Phase 5 routing table categorizing data-model +
migration + behavior changes as `sdd:full`-scale work), this should go through the full MDD/
SDD pipeline (`/plan:mdd-start` or `sdd:1-ideate` seeded directly from this doc, since the
*what* and *why* are already answered here — skip straight to research/planning) rather than
being hand-implemented inline. Recommended phasing within that process:

1. **Phase A** — the ent schema + read-on-startup + write-on-scan-completion, with the
   in-memory `sync.Map`s becoming a write-through cache in front of the DB rather than the
   sole source of truth. Ships value on its own (faster/warmer restarts) independent of Phase B.
   Regression test: kill and restart a Scanner instance mid-test, assert `resultStore` is
   pre-populated from the DB before any scan runs.
2. **Phase B** — the staleness-aware backstop gate (`head_ref_hash` comparison before
   enqueueing real work on the periodic tick). Depends on Phase A's persisted
   `head_ref_hash` field existing. Regression test: seed a DB row with a matching hash for an
   unchanged repo, assert the periodic tick does *not* enqueue a real scan for it; seed a
   mismatched hash, assert it does.

## Related

- BUG-034 (`docs/bugs/fixed/BUG-034-unfinished-scanner-never-removes-completed-session-repos.md`)
  — the leak this strategy's Phase A partly compounds-with-restarts; fixed independently and
  first, since it was the acute, live issue.
