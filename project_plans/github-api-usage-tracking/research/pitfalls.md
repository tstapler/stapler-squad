# Research: Pitfalls & Risks — github-api-usage-tracking

Agent 4 (Pitfalls). Scope: known failure modes for outbound-API instrumentation,
time-series persistence, config hot-reload, and a usage-history UI panel, grounded
in this repo's actual code.

## 0. Load-bearing finding: `DefaultRateLimiter.Update()` is never called today

`github/rate_limit.go`'s doc comments assert this is already wired up:

```go
// DefaultRateLimiter is the shared GitHub API rate limiter used by all native
// HTTP calls. It is updated automatically by rateLimitTransport on every
// response; pollers check IsLimited() before dispatching work.
var DefaultRateLimiter = &RateLimiter{}
...
// Update reads GitHub rate-limit headers from resp and updates the limiter.
// Called automatically by rateLimitTransport on every response — callers do
// not need to invoke this manually.
func (r *RateLimiter) Update(resp *http.Response) { ... }
```

**Verified by grep**: no `rateLimitTransport` type exists anywhere in `github/*.go`,
and `RateLimiter.Update(` has zero call sites in the entire repo (production or
test — `grep -rn "RateLimiter.Update\|r\.Update(resp" --include="*.go" .` returns
nothing). `github/http_client.go`'s shared client is a bare
`&http.Client{Timeout: 30 * time.Second}` with no custom `Transport`, so nothing
ever calls `.Update()` on real responses. `github.DefaultRateLimiter.IsLimited()`
is checked in both pollers (`session/pr_status_poller.go:197`,
`session/worktree_pr_poller.go:187`) but since `rateLimitedUntil` is never set,
`IsLimited()` always returns `false, zero-time` — **the existing rate-limit guard
is currently a no-op in production.**

This is the single most important pitfall for Phase 3 planning: **do not assume
the existing rate-limit plumbing works today.** The new instrumentation work is
not "add tracking on top of a working limiter" — it is "build the transport-level
hook that was documented but never implemented, and add tracking through the same
seam." If the plan treats `DefaultRateLimiter` as already-functional and only
adds counters beside it, the pause-on-403/429 behavior described in the doc
comments will keep silently not happening, and the new UI panel's "quota
remaining" number will have no live data source to read from for the native
`ghHTTPClient` path either — `Update()` is also where remaining/limit/resetAt
would need to be captured and exposed for the panel, not just where the WARN log
fires. Confirm with Agent 1 (stack) whether their reading of this file reached
the same conclusion; if not, reconcile before planning proceeds.

## 1. Instrumentation pitfalls

### 1a. Undercounting: requirements.md's Scope list omits real `gh`-CLI call sites in `github/client.go`

The Scope section says to instrument "`github/client.go`'s `gh`-CLI shell-outs
(`GetPRInfoCtx`)" — singular. Grepping `github/client.go` for
`safeexec.CommandContext(ctx, "gh", ...)` shows **eight** distinct `gh`
invocations across seven functions, not one:

| Line | Function | Command |
|---|---|---|
| 266 | `GetPRInfoCtx` | `gh pr view` |
| 540 | `IsForkRepo` (scoped for deletion per requirements — verify no other caller reappears) | `gh api` |
| 567 | `GetPRComments` | `gh pr view --json comments` |
| 611 | `GetPRDiff` | `gh pr diff` |
| 634 | `PostPRComment` | `gh pr comment` |
| 667 | `MergePR` | `gh` (merge args) |
| 689 | `ClosePR` | `gh pr close` |
| 709 | `CloneRepository` | `gh repo clone` |

`GetPRComments`, `GetPRDiff`, `PostPRComment`, `MergePR`, and `ClosePR` all draw
from the same shared token/quota and are all reachable from backlog review flows
(`session/backlog_review.go`, `server/services/backlog_github_rpc*.go` per the
requirements' own RPC scope) — they were seemingly missed because the audit
focused on the two pollers' read path (`GetPRInfoCtx`) rather than the write/
comment/merge path. If only `GetPRInfoCtx` is wrapped, any near-miss caused by a
burst of PR merges/comments during backlog auto-merge will be invisible to the
new tracking data — directly undermining the Success Metric's requirement that
"the tracking data actually exist and be trustworthy enough to attribute cause."
**Recommend Phase 3 either instrument all `gh`-CLI call sites in `client.go`
uniformly (one wrapper around `safeexec.CommandContext(ctx, "gh", ...)` calls in
this file), or explicitly document which are deliberately excluded and why** —
don't let the omission be implicit.

### 1b. Double-counting risk if wrapping happens at two layers

The native path (`ghHTTPClient.Do(req)`, called from `github/commits.go`,
`etag_cache.go`, `repos.go` (x4), `user_pr_cache.go`, `device_auth.go` (x2),
`client.go` (x5)) is a single shared `*http.Client`. The cleanest place to count
native calls is a `Transport` wrapper (the same seam finding #0 says needs to be
built anyway) — one hook, one count per actual HTTP round trip. The risk is
architectural: if the plan *also* adds manual `usageTracker.Record(...)` calls at
each call site (e.g. inside `GetPRInfoCtx`, `GetPRComments`, etc.) **in addition
to** the transport-level hook, every native call gets counted twice — once by the
transport, once by the call site. This is an easy mistake because the call-site
list in requirements.md's Scope (`session/backlog_plugin_github.go`,
`server/mcp/tools_backlog*.go`, `server/services/backlog_github_rpc*.go`) reads
like "instrument these files," which invites call-site-level counters even where
those files ultimately just call through to `github/client.go` functions that
already hit the shared transport. **Design rule: exactly one counting point per
physical request** — transport-level for the native `http.Client` path,
process-level wrapper for `gh` CLI shell-outs (since those never touch
`ghHTTPClient`). Attribution (which call site triggered it) should be carried as
*metadata* through context (e.g. `context.WithValue` or a request-scoped label)
into the single counting point, not implemented as a second independent counter.

### 1c. `gh` CLI shell-outs make one process invocation but can trigger multiple GitHub API requests

`gh pr view --json <fields>` or `gh pr merge` may internally issue more than one
HTTP request (e.g. pagination, or `gh`'s own auth/rate-limit preflight checks)
depending on flags and repo state. Counting "1 per `safeexec.CommandContext`
call" is a reasonable approximation given the stated low-volume/local-dev
context, but the plan should state this is an approximation explicitly (e.g. in
the UI panel's help text or a code comment) rather than implying byte-for-byte
parity with the native path's per-request counting — otherwise a future reader
comparing "gh CLI calls: N" against GitHub's own rate-limit dashboard will find a
mismatch and assume the tracker is buggy.

## 2. Concurrency pitfalls

### 2a. Follow the existing `sync.RWMutex` pattern in `RateLimiter`, but watch the double-checked-locking trap

`github/rate_limit.go`'s `RateLimiter` already uses `sync.RWMutex` with a
read-mostly (`IsLimited`, `WaitIfLimited`) / write-rare (`setLimitedUntil`)
split — a reasonable model to extend for a running request counter incremented
from the transport hook, `gh`-CLI wrapper, pollers, and on-demand RPCs
concurrently. The repo's own rule
(`.claude/rules/go-double-checked-locking.md`, canonicalized in
`session/git/worktree_git.go`'s `IsDirty`) is directly applicable if the counter
design ends up doing a read-check-then-write-then-reread pattern (e.g. "read
current window bucket, if expired swap it, then read the bucket back for the
return value"): **always return the locally-computed value, never re-read the
shared slot after releasing/reacquiring the lock**, since a concurrent
goroutine may have already written a different value into that slot by the time
you re-read it. A per-request counter is simpler than `IsDirty`'s pattern (an
`atomic.Int64` increment has no read-then-write race at all), so prefer
`atomic.Int64`/`atomic.Uint64` counters over mutex-guarded plain ints wherever
the operation is "add one" — reserve the `RWMutex` (or a `sync.Mutex`) for the
composite state that already lives in `RateLimiter` (remaining/limit/resetAt as
a struct, swapped atomically together) since those fields must stay consistent
with each other, which a set of independent atomics cannot guarantee.

### 2b. Lock contention risk from the *pollers'* own hot path, not the counter

`checkAllSessions()` in both pollers fires `IsLimited()` on every tick and
(per `pr_status_poller.go`'s `ConcurrentFetches` config) dispatches concurrent
per-session fetches. If the new counter/attribution write happens under the same
mutex as the pre-existing `IsLimited()` read path, a burst of concurrent fetches
all incrementing the counter under a write-lock would serialize what should be a
read-mostly gate check. Keep the increment path (write-heavy, one atomic op) and
the `IsLimited`/quota-remaining read path (already `RWMutex`-guarded) as
separate synchronization primitives rather than merging them into one lock,
so heavy counter writes from concurrent fetches don't stall the throttle check
other goroutines depend on before dispatching.

### 2c. Attribution metadata plumbing risk

Per requirements.md's Rabbit Holes, per-session attribution "requires plumbing a
caller identity through call sites that don't currently carry one (e.g. `gh` CLI
shell-outs happen with no session context)." Concretely: `github/client.go`'s
functions take `ctx context.Context` already, so `context.WithValue` is the
natural carrier for a call-site/session label — but every one of the 8 `gh`
call sites in that file would need to read the label back out of `ctx` at the
point of shelling out, and `session/backlog_plugin_github.go` and the on-demand
RPC handlers would need to be the ones setting it. Missing even one of the 8 (see
1a) reproduces the undercounting problem at the attribution layer even if the
raw count is otherwise accurate — i.e. this is a second, independent place the
same "8 call sites, not 1" fact matters, so treat the counting-site inventory and
the attribution-label inventory as the same checklist, not two separate ones.

## 3. Persistence pitfalls

### 3a. `analytics_store.go` is a plausible reuse target, but has no visible retention/rotation policy

`server/services/analytics_store.go` (682 lines) is the closest existing
precedent per requirements.md's Rabbit Holes. Structurally it's a good hot-path
pattern to imitate: `Record()` does a non-blocking `select` into a buffered
channel (`analyticsBufferSize = 1000`) with a dropped-counter fallback rather
than a synchronous write, and a background goroutine (`Start`/`flush`) drains it
to SQLite via `session.Storage`. **This solves the write-amplification concern
for a hot path already** — reuse the pattern (bounded channel + async flush), not
just the storage backend.

What it does **not** appear to have: grepping its exported functions
(`^func `) turns up no `Retention`, `Prune`, `MaxAge`, or similar — i.e. no
built-in mechanism found for capping historical row count/age. If the new
feature reuses this exact table/pattern without adding its own retention job,
GitHub API usage rows will accumulate forever at "hundreds to low thousands of
requests/day" (per the requirements' own Scalability estimate) — modest per day,
but unbounded over months/years of a personal machine's uptime, and nothing in
the reused pattern will stop it. **Explicitly design a retention policy
(time-based TTL prune on a periodic tick, or a row-count cap) as part of Phase 3
planning — don't inherit "no policy" by copying the pattern uncritically.**
Confirm with Agent 1/2 whether `analytics_store.go`'s SQLite table already has
an external prune job elsewhere in the codebase that this grep missed before
concluding it's absent.

### 3b. Startup cost if the store is naively loaded fully into memory

The UI panel needs "request volume over time" with a time-range selector — if
the backing implementation loads the *entire* history table into memory on
startup (or per-request) to serve chart queries, that startup/query cost grows
unbounded with 3a's problem (no retention). Prefer time-bounded queries backed
by an index on the timestamp column (mirroring `LoadWindow`/`LoadProgramWindow`
in `analytics_store.go`, which already take a `since time.Time` parameter) —
i.e. the existing precedent already got this part right; make sure the new
feature's read path follows the same shape rather than a `SELECT *` variant.

### 3c. Synchronous write on the hot path if the transport hook itself blocks on disk I/O

The transport-level counting hook (see #0/1b) runs inline in every HTTP
round-trip's critical path (`RoundTrip` return). If that hook does anything
beyond an in-memory atomic increment — e.g. a synchronous disk/SQLite write per
request — every GitHub API call now pays disk-write latency, which is a
correctness-adjacent regression even though volume is low (the point isn't
throughput, it's that a slow/contended disk write inserted into `RoundTrip`
adds tail latency to *every* GitHub call, including ones on a poller's
time-boxed context). Mirror `analytics_store.go`'s decoupling: the transport
hook should only touch an in-memory atomic counter / channel send; the disk
persistence must happen on a separate ticker/flush goroutine, never inline in
the request path.

## 4. Config hot-reload pitfalls

**Confirmed by grep**: `config/` (`config.go`, `defaults.go`, `types.go`,
`state.go`, `singleton.go`, `claude.go`, `discovery.go`, `workspace_meta.go`) has
**no** `fsnotify`, `Reload`, `reload`, `SIGHUP`, `ConfigChanged`, or
`OnConfigChange` reference anywhere — config is read once at startup. This
directly confirms the risk requirements.md's Rabbit Holes and Feasibility Risks
sections already flagged as an open question: **there is no hot-reload
mechanism today.** "Config-driven poll intervals, adjustable without a rebuild"
(Scope) is achievable without hot-reload (a config file value takes effect on
next process start, no *rebuild* required — just a *restart*), but "no rebuild
required" reads to a user as "no restart required" unless the plan/UI is
explicit about which one it actually delivers. Phase 3 must pick one of:

- **(a) Restart-required, no hot-reload**: simplest, but doesn't fully satisfy
  the spirit of "tune without a rebuild" from a UX standpoint — the user still
  has to bounce the systemd service, which per
  `.claude/rules/tmux-keep-server-on-restart.md` is exactly the kind of
  operation this repo has documented incidents around (killing live tmux
  sessions on Linux when `--tmux-keep-server` isn't passed). State this
  trade-off explicitly rather than letting "no rebuild" imply "no restart."
- **(b) Real hot-reload via fsnotify**: the repo has a strong, repeated,
  well-tested idiom to imitate — `session/history_watcher.go`,
  `session/detection/plugin_watcher.go`, `session/unfinished/scanner.go`,
  `session/mux/autodiscover.go`, and `server/auth/setup.go` all follow the same
  shape: try `fsnotify.NewWatcher()`, fall back to periodic-rescan-only polling
  with a `log.Warn` if unavailable, and debounce bursts of write events. Reusing
  this exact idiom for a config-file watcher is the "don't build a half-
  implemented one-off" path the pollers currently lack.

### 4a. If (b) is chosen: partial hot-reload is worse than no hot-reload

The named risk in the assignment — "a half-implemented file-watcher that only
some poll-interval fields respect" — is concrete here because there are **two**
separate poller structs (`PRStatusPoller.config.PollInterval`,
`WorktreePRPoller.config.PollInterval`), each capturing its interval once at
construction and handing it to a single `time.NewTicker(...)` call inside its
own `pollLoop()`/equivalent goroutine (`pr_status_poller.go:178`,
`worktree_pr_poller.go:155`). A hot-reload implementation must update **both**
tickers via `ticker.Reset(newInterval)` — and Go's own `time.Ticker` docs require
`Reset` be called only from the same goroutine that's reading `ticker.C`, or from
a goroutine that has stopped the ticker first, to avoid a receive/reset race. A
plan that reloads the config struct in place but forgets to also signal both
poller goroutines to call `Reset` will silently leave the *old* interval active
until the next process restart — indistinguishable from a total hot-reload
failure to the user, and worse than not offering hot-reload at all, since the
config file and running behavior visibly disagree.

### 4b. Race between a config reload and an in-flight poller tick

If a reload event fires while `checkAllSessions()` is mid-flight (concurrent
per-session fetches per `ConcurrentFetches`), swapping `p.config` out from under
the running tick could produce inconsistent behavior (e.g. some in-flight
fetches see the old `ConcurrentFetches` value, others see the new one, if that
field is also made hot-reloadable later). Scope hot-reload narrowly to
`PollInterval` only for this feature (per requirements.md's Scope, which only
asks for poll intervals to be config-driven) and update it via the ticker-reset
channel/mechanism rather than swapping the whole `config` struct pointer live —
narrows the blast radius of this race to a single well-understood field.

## 5. Background-goroutine / service-restart interaction risks

This feature adds at least one new long-lived background goroutine class: the
async flush loop for the usage-history store (mirroring `AnalyticsStore.Start`/
`flush`), and potentially a config-file fsnotify watcher (4b). Two existing
house-rule risks apply directly:

- **`.claude/rules/tmux-keep-server-on-restart.md`**: not directly about this
  feature's own goroutines, but relevant context — any manual verification of
  poll-interval or tracking behavior during development must not be done via
  `make install-service` against the live deployed instance (would kill the
  current session's tmux server); use the documented second-instance pattern
  (`PORT=... STAPLER_SQUAD_INSTANCE=... /tmp/ssq-manual-test`) from this repo's
  `CLAUDE.md` instead.
- **`.claude/rules/service-restart-orphan-process.md`**: documents a *real,
  observed* incident class where orphaned `stapler-squad` processes survive a
  restart and race the new process over shared state (`sessions.json`, tmux
  server). If the new usage-history store is a SQLite file (or reuses
  `session.Storage`'s existing SQLite connection) written from a background
  flush goroutine, an orphaned old process's flush goroutine could still be
  writing to the same file concurrently with the new process's flush goroutine
  after a restart — the exact "two writers, one state file" shape the
  documented incident describes for `sessions.json`. Ensure the store either
  reuses `session.Storage`'s existing connection/locking (inheriting whatever
  protection it already has against multi-process writers) rather than opening
  an independent connection, or is instance-scoped the same way
  `STAPLER_SQUAD_INSTANCE` already scopes other state, so two live processes
  never contend over the same file at all.
- **`.claude/rules/fix-flaky-tests-dont-defer.md`**: a new ticker-driven
  goroutine (either the flush loop or a config-hot-reload watcher) is a classic
  source of new flaky tests — tests that start the goroutine and assert on
  timing (`time.Sleep` + check side effect) rather than synchronizing on an
  explicit signal are exactly the shape this rule exists to catch. When Phase 5
  implements this, tests for the flush loop / ticker-reset behavior should
  synchronize via a channel/callback the production code already exposes
  (mirroring `AnalyticsStore.Stop()`'s `<-s.done` drain-and-wait pattern) rather
  than sleeping and hoping the flush already happened. If a flaky test does
  surface here, root-cause and fix it immediately per that rule — don't defer
  it as "known pre-existing," since it would be new, not pre-existing.

## 6. UI pitfalls (flagged for Agent 5 / UX to cover in depth)

Two risks specific to the usage-history panel, noted here but owned by the UX
research track:

- **Sparse/zero data on fresh install**: a brand-new install (or right after
  this feature ships, before any history accumulates) has zero historical rows.
  The chart component must have an explicit empty/sparse state rather than
  rendering a broken/empty axis or a misleading flat-zero line that could be
  read as "confirmed zero usage" instead of "no data collected yet." Compare
  against how `ApprovalAnalyticsPanel.tsx` (the stated convention to follow)
  already handles its own empty-data case, if it does.
- **Accessibility of the time-range selector and charts**: color-only encoding
  for multiple request sources/resources (core vs. search, or per-poller
  breakdown) needs a non-color-dependent differentiator (pattern/label/legend
  with text), and the time-range selector plus any interactive chart elements
  need full keyboard navigation and ARIA labeling — WCAG AA, consistent with
  this repo's existing UX review gate (`ux:review` skill, Axe Core CI check on
  `web-app/src/` PRs per this repo's E2E/CI conventions). Flagging the risk
  exists; full treatment belongs to Agent 5.

## Summary of what Phase 3 planning must explicitly resolve

1. Build (or confirm someone is building) the `rateLimitTransport`-equivalent
   hook that `github/rate_limit.go`'s doc comments describe but that does not
   exist yet — this blocks both real rate-limit pausing *and* the new
   tracking's data source for the native HTTP path.
2. Enumerate all 8 `gh`-CLI call sites in `github/client.go`, not just
   `GetPRInfoCtx`, and decide which are in-scope.
3. Pick exactly one counting point per physical request (transport-level for
   native HTTP, wrapper-level for `gh` CLI) to avoid double-counting.
4. Add an explicit retention/rotation policy for the historical store — do not
   inherit `analytics_store.go`'s apparent lack of one by copying the pattern.
5. Decide restart-required vs. real fsnotify-based hot-reload for poll
   intervals, and if hot-reload, handle both pollers' tickers via the
   goroutine-safe `Reset` pattern, not a swapped-out config struct.
6. Scope the new background goroutine(s)' storage to avoid the documented
   orphan-process multi-writer failure mode.
