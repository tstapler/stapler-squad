# Research: Pitfalls — backlog-bounce-escalation

Scope: alert-fatigue risk for a new durable escalation signal, race conditions in
multi-reason severity computation, flapping/thrash on rapid reason
open/close/reopen, and the risk of a naive flaky-test classification heuristic —
grounded in this repo's actual code (`session/stuck_decisions.go`,
`session/ent_repository_backlog.go`, `session/backlog_lifecycle.go`,
`session/backlog_remediation.go`) and in the directly-prior
`backlog-stuck-item-visibility` project's own pitfalls doc
(`project_plans/backlog-stuck-item-visibility/research/pitfalls.md`), which this
project builds directly on top of.

---

## 0. What the prior project already solved — don't re-litigate it

The prior project's pitfalls doc (§1) worried about `MarkStuck`/`ResolveStuck`
being check-then-act (read, compare in Go, unconditional write) and recommended
moving to a single atomic `UPDATE ... WHERE` per write. **That recommendation
shipped.** Current code (`session/ent_repository_backlog.go`):

- `ResolveStuck` (line 1270): single `UPDATE ... WHERE item_id=? AND reason=? AND
  resolved_at IS NULL` — no read-then-write gap.
- `MarkStuckNotified` (line 1294): same shape, `WHERE notified_at IS NULL`.
- `SnoozeStuckState` (line 1321), `RecordRemediationAttempt` (line 1351): same
  shape, all scoped `WHERE resolved_at IS NULL`.
- `MarkStuck` (line 1199) upserts via `OnConflictColumns(item_id, reason)` inside
  a transaction, plus a second atomic conditional UPDATE for the reopen-in-place
  case (clearing `resolved_at`/`notified_at` only on a row that was already
  resolved).

So the single-row write race the prior project flagged is closed. **What is
still open, and specific to this project**, is a different kind of race:
computing *severity from a read across multiple rows* (count of currently-open
reasons for one item, or "did remediation cap+bounce-still-open co-occur") is
inherently a point-in-time snapshot across N independently-written rows — see
§2 below.

---

## 1. Alert fatigue: don't make this the "boy who cried wolf" toast, twice

The existing one-time "use Reset to try again automatically" notification
(`session/backlog_lifecycle_review.go:208` and 3 near-identical siblings in
`backlog_lifecycle_pr.go`, `backlog_lifecycle_stale.go`, `backlog_lifecycle_triage.go`)
is already the thing the source item complains is too easy to ignore — it fires
once, has no persistent visual weight beyond the toast, and requires the user to
separately go query `backlog_stuck_states` to learn "how bad is this really."
Layering a second escalation signal on top risks the same failure mode unless it
is deliberately *more* durable and *more* visually distinct, not just another
notification:

- **Requirements already call for this** ("durable... marker/notification
  distinct from the existing one-time toast... stays visibly flagged until
  resolved or explicitly acknowledged"). The risk is implementing "durable" as
  "yet another notify-once boolean" (matching `notified_at` on the per-reason
  row) without a corresponding **persistent UI surface** that reads it back —
  i.e., building the backend signal but leaving it undiscoverable except via
  direct DB query, which is the exact status quo the source item is escaping.
- Concretely: if the new escalation marker is implemented as a new column on
  `backlog_stuck_states` (e.g. `escalated_at`) with `notified_at`-style
  notify-once semantics, that's fine for *notification* dedup but the escalated
  *state* itself (is this item currently escalated) must be computed from live
  data on every read (open reason count, cap+bounce co-occurrence), not frozen
  at the moment `escalated_at` was set — otherwise an item that briefly hit 2
  reasons, got notified, then dropped to 1 reason stays permanently marked
  "escalated" in the UI even after de-escalating. Decide explicitly whether
  severity is a live-computed view or a stored, sticky flag — the prior
  project's §3 backfill-notification-storm concern applies here too if this
  ships as "compute once and freeze."
- **Single-user, self-hosted**: there's no SNR problem from multiple tenants,
  but there IS one from repetition — an item stuck at 3 reasons for days
  shouldn't re-notify every tick just because it's still true. Mirror the
  existing `notified_at`-per-reason dedup key precisely: notify on the
  *transition into* escalated severity (e.g. reason-count crossing the
  threshold, or cap+bounce-still-open becoming newly true), not on every tick
  while it remains true. This is exactly `MarkStuckNotified`'s existing
  contract (`WHERE notified_at IS NULL`) — reuse the same idiom for a new
  `escalation_notified_at`-style column rather than inventing new semantics.

---

## 2. Race: computing "N open stuck reasons" while another goroutine
   closes/opens a reason concurrently

Per-row writes (`MarkStuck`, `ResolveStuck`) are now atomic (§0), but the new
feature's core computation — "count open reasons for item X right now" — reads
*across* those independently-written rows, so it cannot itself be atomic against
concurrent single-row writes without adding a new locking mechanism (which the
codebase does not currently have and the requirements' "keep it a simple
count/threshold" rabbit-hole guidance argues against over-engineering).

Concretely, this is `session/backlog_lifecycle.go`'s reconcile tick: separate
stuck detectors (`runStuckDetector("bouncing", ...)` at line 1014, plus whatever
runs for `abandoned_review`/`autonomous_stuck`/etc.) each call their own
`MarkStuck`/`ResolveStuck` for the *same item* within the same tick, sequentially
or via goroutines depending on how `ReconcileStuckItems` is structured. A
severity-count read that happens:
- **mid-tick, between two detectors' writes** — will see a transient N that
  doesn't reflect the tick's final state (e.g. reads 1 open reason right after
  `bouncing` closed but right before `abandoned_review` opens, when the
  "real" answer for that tick is 2 total events but never simultaneously true).
- **against a different goroutine's write in flight** (if reconcile ever
  parallelizes per-item or per-reason) — same shape, worse without a defined
  ordering.

**Design against this:**
- **Compute severity as a fresh read at the point of use, never cache it**
  across a tick or across a reconcile pass. This is squarely the concern
  `.claude/rules/go-double-checked-locking.md` targets — it doesn't apply
  literally here (there's no `read-lock miss compute write-lock` cache pattern
  in the stuck-state code, everything already goes to Postgres per-call) but the
  underlying principle transfers: **never return a value computed at a different
  point in time than the state actually being reported.** If the implementation
  introduces any in-memory memoization of "current severity" (e.g. to avoid a
  second DB round-trip in the same request), it must return the value it just
  locally computed from its own read, not a shared/cached slot another
  goroutine might have overwritten mid-flight — exactly the wrong-value bug the
  rule documents in `session/git/worktree_git.go`'s `IsDirty`.
- **Recompute severity from `FindOpenStuckStates`-equivalent query, at the END
  of a full reconcile pass for that item** (after all detectors for that item
  have run and settled), not interleaved mid-pass. `ReconcileStuckItems`
  already iterates per-item; the natural seam is "after this item's stuck
  detectors have all executed this tick, do one severity read and act on it" —
  avoids the transient-mid-tick read entirely without adding new
  synchronization primitives.
- **Severity is not itself state to persist as a race-prone counter** — derive
  it by counting rows in the same read that surfaces open reasons today
  (`FindOpenStuckStates`/equivalent, `WHERE resolved_at IS NULL`), so it is
  always consistent with whatever the DB actually holds at read time, same as
  every other stuck-state consumer. Don't add a denormalized
  `open_reason_count` column on `backlog_items` that a separate increment/
  decrement call must keep in sync — that reintroduces exactly the
  check-then-act race class §0 already closed for single-row writes, at the
  aggregate level this time.

---

## 3. Flapping: a reason opening/closing/reopening rapidly thrashes
   escalation state

`MarkStuck`'s reopen-in-place path (line 1246) explicitly supports a reason
cycling closed→open on the same row (`ResolvedAtNotNil()` → clear it), and nothing
in the existing detectors debounces against a reason flapping across consecutive
ticks (e.g. `abandoned_review` clearing because a review session briefly spawned,
then reopening 60s later because that session immediately died). This is a
plausible real shape given `abandonedReviewGrace = 15 * time.Minute` — a marginal
item sitting near that threshold could cross it back and forth if review-gate
respawns are themselves flaky.

If a 2-reason-crossing escalation notifies on *every* threshold crossing (2→1→2→1...),
a flapping item generates a notification storm worse than the status quo it's
replacing — directly undermining the alert-fatigue goal in §1.

**Design against this:**
- **Debounce the escalation *notification* (not the underlying open-reason
  state) with a minimum dwell time** before firing — e.g. require the elevated
  condition (N≥threshold, or cap+bounce-open) to hold across two consecutive
  60s reconcile ticks before notifying, mirroring how `abandonedReviewGrace`
  itself exists to avoid flagging on a single-tick blip. A simple "was elevated
  last tick AND is elevated this tick" check needs no new schema — it's a
  re-read of the same row set the count itself came from.
- **Do not let de-escalation immediately clear the durable marker on a single
  good tick either** — if the escalation marker is meant to "stay visibly
  flagged until resolved or explicitly acknowledged" (per requirements), a
  single-tick dip below threshold clearing it (only to reappear next tick) is
  the write-side mirror of the same flapping problem. Either require the same
  dwell time to de-escalate, or make de-escalation deliberately require an
  explicit acknowledgment (matching the "or explicitly acknowledged" language
  in the requirements) rather than an automatic clear on any tick where the
  count drops.
- **Keep the notify-once key scoped to the escalation event's *identity*, not
  just "currently escalated," same as the prior project's §3 finding about
  `staleWorkNotified`/`stuckReviewNotified` losing the "recovered then
  re-stuck should re-notify" distinction once made durable and permanent.** A
  reasonable default: dedupe on `(item_id, escalation kind)` with a fresh
  notify allowed only after a full de-escalation-then-re-escalation cycle, not
  on every tick the condition continues to hold.

---

## 4. Naive flaky-test classification heuristic: over/under-application risk

Requirements' own Rabbit Holes section already flags this ("keyword match...
needs an explicit, cheap decision in planning; don't let this become a general
intent-classification subsystem") and explicitly permits deferring full
implementation. Concrete risks with the cheapest version (title/description
keyword match on "flaky", "-race", "intermittent"):

- **False positives (over-application) mis-trigger the differentiated review
  strategy for ordinary items.** Any backlog item whose *title* happens to
  mention "flaky" as part of unrelated work — e.g. "investigate why the flaky
  detector itself sometimes double-fires" (a meta item *about* flakiness, not
  a flaky-test fix) — would get the loosened/stricter verdict threshold though
  its own change has nothing non-deterministic to verify.
- **False negatives (under-application) miss real cases with no keyword.** The
  three motivating live items are described as "flaky-test root-cause fixes" in
  the requirements — but the requirements text doesn't establish that their
  actual titles contain any of the candidate keywords; a fix item auto-created
  from a stuck detector or GitHub issue import may describe the *symptom*
  ("TestFoo intermittently times out") without ever using the word "flaky" at
  all, or may only surface the non-determinism in the PR diff/test file touched
  (`_test.go` files under `-race`, `t.Skip`/retry-loop removal), not the prose.
- **This repo already has a working precedent for the *right* shape of
  signal, and it isn't text classification.** `session/stuck_decisions.go`'s
  `IsRepeatedFailure` / `IsRepeatedNoVerdictFailure` classify a *behavioral*
  pattern (two consecutive review attempts producing an identical failure
  summary, or consecutive review sessions that never produced a verdict at
  all) — not a keyword match on the item's own text. The bouncing detector
  itself (`isBouncing`, same file) is behavioral too: cycle count + absence of
  a PASS verdict, not a title scan. A flaky-test signal built the same way —
  e.g. "did review verdict N pass and verdict N+1 fail (or vice versa) on an
  otherwise-unchanged diff" or "does the PR touch `_test.go` files across
  rework cycles without the non-test diff changing" — would be strictly more
  reliable than keyword matching and consistent with the codebase's existing
  house style for this exact class of problem.
- **If keyword matching is used anyway for the "cheap decision" the
  requirements explicitly permit**, treat it as a coarse, openly-labeled
  heuristic (log/surface *which* signal fired), not a silent classifier —
  matches this repo's `.claude/CLAUDE.md` "document AI decisions in edge
  cases" memory precedent (self-heal/auto-close actions should post a visible
  comment, not act silently) applied to classification instead of remediation:
  if an item gets the loosened flaky-test review strategy, that decision and
  its basis should be visible on the item, not inferred only from behavior
  changing.
- **Recommendation for planning**: given the fallback-increment guidance already
  in requirements (multi-reason escalation is separable and lower-risk; the
  flaky-test differentiation is "the more speculative, lower-confidence piece"),
  and given a materially better-fitting behavioral signal already exists in this
  codebase's style, planning should seriously weigh deferring item 3 to its own
  follow-up rather than shipping a keyword heuristic under time pressure — the
  cost of shipping a wrong classifier (miscalibrated review strictness on
  misclassified items) is arguably worse than shipping no differentiation yet.

---

## Summary of concrete design constraints to carry into planning

1. Single-row stuck-state writes are already race-safe (atomic `UPDATE ...
   WHERE`, shipped by the prior project) — don't re-solve that. The open risk
   here is aggregate reads (open-reason counts) taken at a different point in
   time than the writes that produced them; always recompute severity fresh
   from a live query, never from a cached/denormalized counter, and read it
   only after a full per-item reconcile pass has settled for that tick.
2. The new escalation marker must be more durable/visible than the existing
   one-time toast to actually solve the source problem — decide explicitly
   whether "escalated" is a live-computed view (recommended) or a sticky
   stored flag, since a sticky flag needs its own de-escalation/acknowledgment
   rule or it goes stale.
3. Debounce both escalation and de-escalation notifications against
   single-tick flapping (require the condition to hold across ≥2 consecutive
   reconcile ticks, mirroring `abandonedReviewGrace`'s own existing
   single-tick-blip protection) — otherwise a marginal item thrashing near a
   threshold generates more noise than the status quo, undermining the
   alert-fatigue goal.
4. Prefer a behavioral flaky-test signal (review-verdict flip-flop across
   otherwise-identical diffs, or test-file-only rework cycles) over
   title/description keyword matching, matching this codebase's existing
   `IsRepeatedFailure`/`isBouncing` house style — and if keyword matching ships
   anyway as the requirements' permitted "cheap decision," surface which
   signal fired rather than classifying silently. Seriously consider deferring
   this piece entirely per the requirements' own fallback-increment guidance.
