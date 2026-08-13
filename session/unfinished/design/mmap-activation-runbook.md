# Activation runbook: `gogitstore.Registry.UseMmapIndex`

Status: **not activated**. `session/unfinished/gogit_vcs_reader.go`'s `gogitstoreRegistry()`
constructs `&gogitstore.Registry{CacheMaxSize: perRepoObjectCacheSize}` — `UseMmapIndex` is
left at its zero value (`false`). This doc is what a human should check/do before flipping it
to `true`, written after the production-hardening pass documented in this task's report
(stress testing, a real-repo heap measurement, adversarial/fuzz testing of the mmap loader,
a soak test, and two bugs found and fixed by that work — see the report for full detail).
Keep this short and practical; it is not a design document (see `pluggable-gitstore.md` for that).

## What activation actually changes

`UseMmapIndex: true` only affects `SharedObjectStore`s created **after** the flag is set —
it does not retroactively convert an already-open store (`registry.go`'s doc comment). In
practice this means: flip the field, restart the process (or wait for natural eviction of
existing stores via `Registry.Prune`'s TTL), and every commondir opened from that point
forward uses the zero-copy mmap `.idx` loader (`mmapindex.go`) instead of the copy-based
`io.ReadFull` decoder, plus gains a per-commondir fsnotify watcher on `objects/pack`
(`mmapwatch.go`) for fast staleness detection after a repack.

## Before flipping the flag

1. **Re-run the full test suite with `-race`** and confirm it's still clean:
   ```
   go test ./session/unfinished/... -race -count=1 -v
   ```
   As of this hardening pass: 110 top-level+subtests passed, 0 failed, across
   `session/unfinished` and `session/unfinished/gogitstore`.

2. **Re-run the stress test** at a meaningful iteration count — this is the single test that
   found and proved the generation/refcount safety property, and the one whose repeated
   execution surfaced a real goroutine/fd leak during this hardening pass (see report):
   ```
   go test ./session/unfinished/gogitstore/... -race \
     -run '^TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack$' \
     -count=60 -timeout=8m -v
   ```
   Should be 100% clean. If ANY iteration fails, do not activate — treat it as a real bug
   (this is exactly the standard this hardening pass held itself to).

3. **Run the soak test** to re-confirm the eviction+refcount+retire lifecycle is leak-free
   under sustained concurrent load:
   ```
   go test ./session/unfinished/gogitstore/... -run '^TestGogitstore_SoakUnderSustainedLoad$' -v
   ```
   Expect `opErrorsUnexpected=0` in the log output. `opErrorsStaleIndex` being nonzero is
   normal (see "Known, accepted risk" below) — only `opErrorsUnexpected` indicates a problem.

4. **Read the design doc's honesty sections** (§5.3, §8, §9) and this task's report in full —
   they list what is and is not proven. Nothing found during this hardening pass changes the
   recommendation to proceed, but the specific residual risks below should inform the rollout
   pace, not be treated as already closed.

## Canary rollout

There is currently no per-process env var override wired for this — `UseMmapIndex` is a
`Registry` struct field set once in `gogitstoreRegistry()`. The cleanest low-risk canary
mechanism, if wanted before a blanket flip:

```go
// gogit_vcs_reader.go, gogitstoreRegistry()
g.gogitstoreReg = &gogitstore.Registry{
    CacheMaxSize: int64(perRepoObjectCacheSize),
    UseMmapIndex: os.Getenv("STAPLER_SQUAD_GOGITSTORE_MMAP") == "true",
}
```

This is a small, self-contained addition (one env var read, no new dependency) — not built as
part of this hardening pass since the task's instructions were explicit that the activation
decision itself, not just the env var mechanism, belongs to the coordinator. Recommended
canary shape: enable on one instance (e.g. a personal dev machine, via `STAPLER_SQUAD_GOGITSTORE_MMAP=true`
in the systemd unit's environment) for a few days of real usage before flipping the code-level
default.

## What to watch during canary

- **Logs**: `gogitstore: mmap index load failed, falling back to copy-based loader for this
  pack` (Warn, `store.go`) — a per-pack fallback, not fatal, but a persistent stream of these
  indicates something about the environment (non-OS-backed filesystem, permissions) is wrong
  for the mmap path specifically.
- **Logs**: `gogitstore: munmap failed for retired index` / `close failed for retired index
  file` (Warn, `mmapindex.go`) — should never appear in normal operation; if seen, investigate
  immediately (points at an OS-level mmap/fd problem).
- **Logs**: `gogitstore: leaking mmap index mapping at store eviction — pins still held`
  (Warn, `mmapwatch.go`) — indicates a caller obtained an `Entries()` iterator and never
  called `Close()` on it before its `WorktreeStorer` was released. Bounded (one leaked mapping
  per occurrence, not a crash) but should not happen given `Entries()`/`EntriesByOffset()` are
  not on the three production scanner operations' call path (`HasUncommitted`/`DiffShortstat`/
  `AheadBehind` — see design doc §6's non-goals) — if this fires, something is calling into
  gogitstore's `Entries()` API from outside the paths this hardening pass exercised.
- **Metrics** (if/when instrumented — none of this exists yet, flagged as a gap):
  process `HeapInuse`/`HeapAlloc` trend (should track lower than pre-activation given the
  measured ~92-96% reduction in per-store parse allocation on a real repo — see report),
  goroutine count (`runtime.NumGoroutine()` — one extra goroutine per currently-live
  mmap-enabled `SharedObjectStore`, bounded by `registryMaxEntries=100`, evicted along with
  its store), open FD count (`/proc/<pid>/fd` count — one extra inotify fd per live
  mmap-enabled store, same bound).
- **Scan error rate**: watch for an uptick in "object not found" / "packfile not found"-class
  errors surfacing through scan results specifically correlated with repos that repack
  frequently. See "Known, accepted risk" below — this task's soak test found and fixed the
  specific case where this error type went unrecognized by existing tolerance logic
  (`store.go`'s `dirObjectPack` now translates `dotgit.ErrPackfileNotFound` to
  `plumbing.ErrObjectNotFound`), but confirm in real logs that this is actually rare at
  realistic (not adversarial) repack frequency.

## Known, accepted risk (read before activating)

This hardening pass's soak test (`soak_test.go`) found that **both** modes — not just mmap —
can surface a transient `plumbing.ErrObjectNotFound` when a lookup races against a concurrent
repack of the underlying repo (the shared, cached index can briefly name a pack a repack has
since unlinked). This is inherited from go-git's own upstream behavior, not introduced by
gogitstore or this hardening pass (verified against `storage/filesystem/object.go`'s own
`getFromPackfile`, which has the identical raw-error-propagation gap). Under this task's
intentionally adversarial soak load (a full repack roughly once per second per repo — far more
frequent than any realistic production `git gc` schedule), **copy-based mode hit this on ~98%
of operations** (no automatic staleness recovery at all outside `Registry.Prune`'s ~30-minute
TTL) versus **mmap mode's ~19%** (sub-second fsnotify-driven recovery). In other words: mmap
mode is measurably **more** resilient to concurrent repacks than the copy-based mode already
running in production today, not less — this is a point in favor of activation, not a new risk
it introduces. The residual risk is the underlying error type itself, which now degrades
safely (a typed, tolerable error) in both modes after this hardening pass's fix, rather than
being new risk specific to flipping `UseMmapIndex`.

Separately: a `.idx` file being **truncated in place** while mapped (as opposed to git's own
unlink-and-replace repack behavior, which is safe per design doc §5.3's POSIX analysis) causes
a hard process crash (SIGBUS), proven in this hardening pass
(`TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection`). This is **not reachable
through git's own actual repack behavior** (verified: git never truncates an existing pack
index in place) — it would require external corruption or a misbehaving third-party tool
writing directly into the repo's `.git/objects/pack` directory. A proven mitigation exists
(`runtime/debug.SetPanicOnFault`, demonstrated in the paired
`TestMmapIndexHandle_TruncateWhileMapped_RecoverableWithProtection` test) but was deliberately
**not** wired into production code during this pass — see that test's doc comment for the
full cost/benefit reasoning (unverified hot-path performance impact, and the scenario isn't
reachable through gogitstore's own normal operation). Flagged here so the coordinator can
decide whether this residual risk is acceptable for activation, or whether the mitigation
should be built out first.

## Rollback procedure

Flipping `UseMmapIndex` back to `false` in `gogitstoreRegistry()` and rebuilding/restarting
is sufficient — it is a `Registry` struct field read once per `SharedObjectStore` creation
(`registry.go`'s `acquire`), not a live, hot-reloadable toggle:

- Existing mmap-backed stores are unaffected until they are evicted by `Registry.Prune` (TTL
  or budget) and recreated — at that point new stores use the copy-based loader again.
- Nothing needs to be done about mappings that are "open at flip time" — a mapped file stays
  valid and safely readable for as long as its owning `SharedObjectStore` lives, regardless of
  the `Registry`'s current `UseMmapIndex` value; the field only gates what happens on the
  *next* `ensureIndex()` call for a store that doesn't exist yet.
- For an immediate full rollback (not waiting for TTL-driven recycling): restart the process.
  `stopPackWatch` fires for every live store during process shutdown's normal goroutine
  teardown (or simply exits with the process — no explicit action needed, since nothing here
  persists mmap state to disk).

## What this runbook does not cover (explicitly out of scope)

- Wiring the canary env var itself (sketched above, not built).
- Production metrics/dashboards for the "what to watch" list above (none of this
  instrumentation exists yet — this is a real gap, not an oversight).
- The `dotgit.ErrPackfileNotFound`→`plumbing.ErrObjectNotFound` translation fix landed in
  `store.go` during this hardening pass, but a broader audit of `gogit_vcs_reader.go`'s error
  tolerance logic (are there other call sites that should but don't tolerate
  `plumbing.ErrObjectNotFound`?) was not performed — flagged as a follow-up, not blocking.
