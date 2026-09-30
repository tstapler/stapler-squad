# Architecture Review: cross-host-claim-dedup
**Date**: 2026-09-23 (iteration 2 of max 3)
**Verdict**: CONCERNS

Scope note: this pass re-reviews only the 3 Blockers from iteration 1 against the current
`implementation/plan.md` and the two ADRs. The Concerns/Nitpicks below are carried over verbatim
from iteration 1 (out of scope for re-verification this pass, per the re-review instructions) —
none of the blocker fixes touch the areas they flag, so none are reintroduced or resolved by this
pass.

Methodology note: the plan.md file was being actively edited by a concurrent fix subagent while
this review was in progress (observed growing from 809 → 862 → 929 → 964 → 1065 → 1067 lines
across repeated reads over several minutes). All findings below are against the version that
stabilized at 1067 lines (unchanged for 4+ consecutive 15s polls, `md5sum` `08b2995742bf1e57796b57572c768bde`) — earlier in-flight reads were discarded rather than cited.

## Blockers
None remaining. All three blockers from iteration 1 are resolved — see below.

### Resolved: Blocker 1 (import-cycle / layering violation)
**Original finding**: `crossHostClaimChecker.RecordClaim` was defined in `server/services`, but
`Storage.CreateBacklogItem` (package `session`) needed to call it — `server/services` already
imports `session`, so the reverse call would be a compile-time cycle. No task actually wired
`RecordClaim` into production data at all.

**Fix verified**: the plan now introduces `session.ClaimRecorder`, a port *defined in package
`session`* (`session/claim_recorder.go`, new — Task 1.3.1a, plan.md:506-518), with
`Storage.SetClaimRecorder(...)` (mirroring `Storage.SetCallbackDispatcher`) wired from
`server/dependencies.go` (Task 1.3.1c, plan.md:529-533) and called from
`Storage.CreateBacklogItem` (Task 1.3.1b, plan.md:520-527). `crossHostClaimChecker` is narrowed
to read-only `CheckClaim` (Story 2.1.1's revision, plan.md:548-554) — nothing in the plan calls
`crossHostClaimChecker.RecordClaim` anymore. This is exactly the `ItemChangePublisher`/
`CallbackDispatcher` pattern the iteration-1 review pointed at as precedent, and ADR-002 was
updated with an explicit "Correction (post-review)" section
(`decisions/ADR-002-record-claim-at-single-choke-point.md:35-47`) naming the same precedent.

Checked against the actual codebase, not just plan prose: `session/storage.go:284-292` does hold
`SetItemChangePublisher`/`SetCallbackDispatcher` forwarding to `s.repo`; `ItemChangePublisher`
(`session/backlog_item_change.go:84-86`) and `CallbackDispatcher`
(`session/callback_dispatcher.go:15-17`) are both plain interfaces defined in package `session`;
`Storage.CreateBacklogItem` is confirmed at `session/storage.go:906`. The new `ClaimRecorder`
port follows the identical shape. No cycle is introduced.

### Resolved: Blocker 2 (`newGHRequestForHostWithToken` GET-only mismatch)
**Original finding**: Task 3.1.1a specified building a `POST .../comments` request with a JSON
body via `newGHRequestForHostWithToken`, but that constructor is hardcoded to
`http.MethodGet`/nil body (`github/http_client.go:274-287`) and cannot produce it.

**Fix verified**: Story 3.1.1 now carries an explicit "Revision note (resolves architecture-review
Blocker 2)" (plan.md:815-824) restating the same root cause, and Task 3.1.1a
(plan.md:833-843) adds a new sibling constructor,
`newGHRequestForHostWithTokenAndBody(ctx, host, method, path, token string, body []byte)
(*http.Request, error)`, to `github/http_client.go`, explicitly added to the
`norawghrequest` analyzer's approved-constructor list. Task 3.1.1b (renumbered from 3.1.1a,
plan.md:845-849) now calls that new constructor with `http.MethodPost` instead of the old
GET-only one. Confirmed against the actual current file
(`github/http_client.go:274-287`): `newGHRequestForHostWithToken` is still
`http.NewRequestWithContext(ctx, http.MethodGet, RestBaseURLForHost(host)+path, nil)` today, so
the fix is solving the constraint as it actually exists, not a stale description.

Residual nitpick (not a blocker — see Nitpicks below): the summary tables at the top of the
document (Pattern Decisions plan.md:57, Tech Debt Disposition plan.md:73) still say "New function
via `newGHRequestForHostWithToken`," which is now inaccurate — the concrete task instructions
(3.1.1a/3.1.1b) are correct and are what an implementer would actually follow, so this doesn't
block, but it's a leftover inconsistency from the fix.

### Resolved: Blocker 3 (claim-forgery / missing TOFU cross-check)
**Original finding**: `ClaimRecord.Verify()` only checks internal self-consistency (signature
matches the record's own embedded `PublicKey`); nothing cross-checked an incoming record's
`PublicKey` against the key already pinned in `HostRegistry` for that `ClaimingHostID`, the way
`HostRegistry.Advertise` does for its own record type
(`bytes.Equal(existing.PublicKey, record.PublicKey)`, `session/host_registry.go:213`). A forged
claim from a fresh keypair would pass `Verify()` and be accepted/re-gossiped as if the victim
host issued it.

**Fix verified, at the task-implementation level, not just in prose**: Story 1.1.1 gained a
second AC (plan.md:285-297) explicitly naming the fresh-keypair-forgery scenario as distinct from
bit-flip tampering. Task 1.1.2a (plan.md:345-356) now adds a `hostRegistry *HostRegistry` field
to `ClaimIndex` and changes `NewClaimIndex`'s signature to `NewClaimIndex(stateDir string,
hostRegistry *HostRegistry)`. Task 1.1.2b (plan.md:358-373) specifies `RecordClaim` looking up
the pinned `PublicKey` for `record.ClaimingHostID` via `c.hostRegistry` and rejecting on mismatch
(`bytes.Equal`), accepting-on-first-sight only when no pin exists yet — mirroring
`HostRegistry.Advertise`'s check exactly. Task 1.1.2d adds a dedicated fresh-keypair-forgery unit
test distinct from the tamper test. The call sites that construct `ClaimIndex` were updated to
match: Task 1.2.2a (plan.md:462-472) passes `hostRegistry` to `NewClaimIndex`, and Task 1.2.1b
(plan.md:432-441) threads `*session.HostRegistry` into the claim-advertisement HTTP handler too.

I checked this is actually buildable, not just asserted: `session/host_registry.go` has
`Lookup(id HostID) (RegistryEntry, bool)` (line 304) and `RegistryEntry` carries a `PublicKey`
field (line 78), so `ClaimIndex.RecordClaim`'s described `c.hostRegistry`-based pin lookup has a
real method to call. This is a first-iteration issue I flagged in the original review as a real
risk (glossary asserting a fix that didn't appear at the task level) — confirmed here to be
resolved at the task level this time, not just in the glossary/AC prose.

## Concerns
*(carried over verbatim from iteration 1 — not re-verified this pass, per scope)*

- [ ] **Tech Debt Disposition row for `server/services/backlog_service_triage.go`** (Story
  2.2.2's target file) — the table asserts "None (no hotspot finding)" for this file, but by this
  repo's own `code-hotspot-analysis` complexity×churn criteria it reads as a textbook hotspot
  candidate: 3,534 lines, 70 top-level functions (`grep -c '^func ' server/services/
  backlog_service_triage.go`), and 107 commits touching it in the last 90 days
  (`git log --oneline --since="90 days ago" -- server/services/backlog_service_triage.go | wc
  -l`) — `DequeueNextQueuedItems` itself spans ~102 lines. The plan's other disposition-table rows
  cite a specific verification command (`grep -rn`) for their factual claims; this row cites
  none. The chosen disposition ("Isolate via seam") is still the right call even if this *is* a
  hotspot, so this isn't a reason to change the design — but stating "no hotspot finding" as a
  confirmed fact when it looks unverified against this file specifically overstates confidence.
  Recommend either citing the actual `research/architecture.md` hotspot check that covered this
  file, or softening the row to "not separately re-verified for this item; isolate-via-seam
  applies regardless."

  Note: as of this iteration, plan.md's Tech Debt Disposition row for this file (plan.md:72) has
  already been reworded to the softened form this concern recommended ("Not separately
  re-verified for this item... isolate-via-seam, regardless"). Left here unchanged per scope, but
  flagging that it reads as already addressed on inspection.

- [ ] **`ClaimVerdict` (Task 2.1.1a)** — labeled a "sum type" in the Pattern Decisions table, but
  implemented as `struct { Kind ClaimVerdictKind; Record session.ClaimRecord }` — an
  enum-plus-shared-payload, not the interface+marker-method sum type the `type-driven-design`
  skill's Technique 3 describes. This still allows illegal combinations the type system won't
  catch: `{Kind: ClaimUnclaimed, Record: <populated>}` or `{Kind: ClaimHeldByOther, Record:
  <zero-value>}`. Multiple call sites construct it via bare struct literal with no smart
  constructor (Task 2.1.2b's peer-fanout loop, Task 2.2.3a's MCP-result mapping, plus tests),
  so nothing stops one of them from pairing the wrong `Kind`/`Record` combination, especially in
  the fanout loop where `Record` could easily be left over from a prior iteration. Recommend
  adding constructor functions (`NewUnclaimedVerdict()`, `NewHeldByOtherVerdict(record)`,
  `NewIndeterminateVerdict()`) to Task 2.1.1a so the pairing is enforced at construction instead
  of by convention at every call site.

  Note: as of this iteration, Task 2.1.1a (plan.md:563-573) already specifies exactly these three
  smart constructors and states every construction site in the plan uses them. Left here
  unchanged per scope, but flagging that it reads as already addressed on inspection.

- [ ] **`ClaimGossiper.BroadcastOnce` signature/semantics are inconsistent across tasks** (Tasks
  1.2.1a, 1.2.2a, 2.1.1c) — Task 1.2.1a says to mirror `HostAdvertiser` structurally, where
  `BroadcastOnce(ctx)` (`session/host_advertiser.go:91`) takes no record argument and builds its
  *own current* advertisement fresh each call. Task 2.1.1c says `RecordClaim` "triggers
  `ClaimGossiper.BroadcastOnce` for just the new record," implying a record parameter that
  `HostAdvertiser.BroadcastOnce` doesn't have. Separately, Task 1.2.2a wires
  `go claimGossiper.Run(ctx)` on the same periodic interval as the host advertiser — if that
  periodic loop re-broadcasts the *entire accumulated claim set* on every tick the way
  `HostAdvertiser.Run`/`BroadcastOnce` re-broadcasts the full (small, O(1)) advertisement, it
  reproduces exactly the "payload grows without bound, retransmitted every cycle" problem
  ADR-001 gives as its first reason for rejecting a claims-field on `AdvertisementRecord` — just
  moved to the new channel instead of avoided. Recommend the plan pin down explicitly: (a)
  `BroadcastOnce`'s exact signature (record-scoped send vs. full-state broadcast), and (b) what,
  if anything, the periodic `Run` ticker re-sends — e.g., nothing, with propagation to
  newly-discovered peers handled by a separate, explicitly-bounded backfill path, not a periodic
  full resend.

  Note: as of this iteration, Task 1.2.1a (plan.md:411-430) already pins down
  `BroadcastOnce(ctx, record ClaimRecord)` as record-scoped and states the periodic ticker only
  backfills to newly-discovered peers, not a full resend. Left here unchanged per scope, but
  flagging that it reads as already addressed on inspection.

- [ ] **Story 2.1.2's synchronous peer fan-out sits on the creation critical path** (Task
  2.1.2b) — `localClaimChecker.CheckClaim` fans out a bounded-timeout GET to every
  `HostRegistry.Snapshot()` peer before `ImportGitHubIssue`/`DequeueNextQueuedItems` can proceed.
  The task doesn't specify whether this fan-out is concurrent (goroutines + a shared context
  deadline) or sequential. Sequential fan-out against N peers each with
  `defaultLivenessTimeout`-style bounded timeouts (2s in the existing `deep_link_resolver.go:56`
  precedent this mirrors) could add up to N×timeout latency in the worst case (multiple
  unreachable peers) — directly at odds with `research/ux.md`'s "must not spinner-gate a fast
  interaction" constraint the plan's own Unresolved Questions section already flags for bulk
  import. Recommend the task explicitly specify parallel fan-out with one shared bounded
  deadline for the whole check, not a per-peer sequential loop.

  Note: as of this iteration, Story 2.1.2's AC (plan.md:612-628) and Task 2.1.2b
  (plan.md:636-645) already specify concurrent, goroutine-per-peer fan-out against one shared
  `context.WithTimeout`. Left here unchanged per scope, but flagging that it reads as already
  addressed on inspection.

## Nitpicks
*(carried over verbatim from iteration 1, plus one new item from this pass)*

- Task 2.2.3a's MCP tool result flattens the three-way `ClaimVerdict` into two independent
  booleans (`claimed`, `checked`), leaving one combination (`claimed: true, checked: false`)
  presumably unreachable but not called out as such; worth a one-line comment or a test asserting
  it can't occur, given the underlying type doesn't structurally prevent it (see the
  `ClaimVerdict` concern above).
- `.claude/inspect.json` has no `architecture.components`/`dependency_rules` section, so
  `kibitzer`'s `component-deps` checker isn't configured for this repo and couldn't have caught
  Blocker 1's `session` → `server/services` layering violation mechanically. Once this plan's
  `session`/`server/services` boundary is fixed, consider adding a `dependency_rules` entry
  (`{component: "session", may_depend_on: []}` relative to `server/services`) so a future PR that
  reintroduces the same direction gets flagged automatically instead of relying on review.
- **New this iteration**: the Pattern Decisions (plan.md:57) and Tech Debt Disposition
  (plan.md:73) summary tables at the top of the document still describe the PR-provenance write
  as "New function via `newGHRequestForHostWithToken`," which is now stale — Story 3.1.1's fix
  for Blocker 2 introduces `newGHRequestForHostWithTokenAndBody` instead, and the concrete task
  instructions (3.1.1a/3.1.1b) correctly use the new constructor. Doesn't block implementation
  (the tasks are what an implementer follows), but worth a one-line table update so the summary
  doesn't contradict the detailed tasks.

## Recommendation Summary
All three iteration-1 blockers are resolved with concrete, buildable task-level changes (verified
against the actual current source, not just plan.md's own prose): the import-cycle is closed via
a `session`-owned `ClaimRecorder` port matching the `ItemChangePublisher`/`CallbackDispatcher`
precedent; the GitHub PR-comment write now routes through a new POST-with-body constructor added
to the `norawghrequest` approved list instead of misusing the GET-only one; and claim forgery is
closed via a `hostRegistry`-backed TOFU pin check in `ClaimIndex.RecordClaim`, mirroring
`HostRegistry.Advertise`'s own check, with a dedicated test. No new blocker-level issues were
introduced by these fixes. The plan carries four pre-existing Concerns and two Nitpicks forward
unverified this pass (per scope); on inspection while reading for the blocker re-review, three of
the four Concerns and the `ClaimVerdict`-sum-type nitpick appear to already be addressed in the
current plan text — worth a quick confirmation pass next iteration, but not re-litigated here
since re-checking Concerns was out of scope for this review.
