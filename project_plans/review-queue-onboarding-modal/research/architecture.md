# Research: Is AC4 satisfiable? (pressure-test of requirements.md's conclusion)

Scope: verify, with fresh evidence, whether requirements.md's conclusion — AC4
("fix changes only the dismissal step; no assertion/selector/test-intent
modified; no production/source code touched") cannot be satisfied together
with AC2 ("0 failed") given Bug A and Bug B — actually holds, or whether a
test-only alternative exists that the prior pass missed.

## 1. Bug A: is a test-only fix for the sentinel test possible?

**Verdict: not possible reliably — confirmed.**

The pre-fix code (parent of `2811df54e`) gated the
`[data-testid="review-queue-loaded"]` sentinel inside the *non-empty-queue*
render branch only — a three-way ternary `loading ? … : items.length === 0 ?
(…three empty-state variants…) : (<>{sentinel}{items}…</>)`. Confirmed via
`git show 2811df54e^:web-app/src/components/sessions/ReviewQueuePanel.tsx |
sed -n '1185,1225p'`: the sentinel div appears only in the final `else`
branch, never in the `items.length === 0` branches, regardless of `loading`
state. The applied fix hoists it above the whole conditional, gated only on
`!loading` (`web-app/src/components/sessions/ReviewQueuePanel.tsx:906-909`,
`git show 2811df54e -- web-app/src/components/sessions/ReviewQueuePanel.tsx`).

Considered and rejected test-only alternatives:

- **Poll/wait for non-empty via `waitForReviewQueue`** — exists in
  `tests/e2e/helpers/session-client.ts:163-174` (polls `getReviewQueue()` up
  to `minItems`, 15s default timeout). Rejected: AC2 itself documents that
  "2 of the 8 active tests may self-skip on empty test-server queue state,"
  i.e. the backlog author already accepts a legitimately-empty queue as a
  possible, non-buggy runtime state. Making the sentinel test *wait for*
  non-empty converts a real defect into a new flaky/hanging test on exactly
  that legitimate state — it doesn't fix anything, it just changes which
  assertion times out.
- **Seed a session and `waitForReviewQueue(1)` in a per-test `beforeEach`,
  scoped to just this test** — technically test-only (uses existing
  `createIdleSession`/`waitForReviewQueue` from `session-client.ts:163-186`,
  no server change). Rejected for two independent reasons:
  1. `review-queue.spec.ts`'s own file docblock (lines 4-8) states real
     session creation "requires tmux sessions, git worktrees, and program
     execution which is not suitable for E2E testing without mock
     infrastructure" — that's exactly why this file's UI-contract tests
     avoid `createIdleSession`. `escalation-reasoning.spec.ts` is the file
     that *does* pay this cost (see its docblock, lines 1-26: "creates a
     real session and posts directly to the hook endpoint," and even then
     needs `WriteToSession` to fabricate PTY text matching an approval
     pattern — `session/detection`'s `NeedsApproval`, lines 17-23 — because
     "there is no direct ApprovalStore -> queue link"). Getting a session
     into the *review* queue (not just "idle") is not the one-line
     `createIdleSession` call implied by AC4's "test-only" framing; it's the
     same heavyweight machinery `escalation-reasoning.spec.ts` already
     needs, imported into a file whose whole design premise is to avoid it.
  2. Even if seeding worked, it doesn't fix the underlying claim under test.
     The sentinel's own inline comment (unchanged by the fix, still true
     pre- and post-fix) says it signals "the loading state resolved" — not
     "the queue is non-empty." A seed-based workaround makes the test pass
     by manufacturing a non-empty queue, while leaving the real defect (the
     sentinel still never appears on a legitimately-empty resolved-load
     state) live in production for any other consumer — including
     `escalation-reasoning.spec.ts:177`, which also waits on this exact
     sentinel (`waitForSelector('[data-testid="review-queue-loaded"]', {
     timeout: 10000, state: 'attached' })`) before proceeding, with no
     seeding of its own guaranteeing a non-empty queue at that point either.
- **No review-queue-specific debug seed endpoint exists.** `server/services/
  backlog_debug_seed_handler.go:42-44` registers only
  `/api/debug/backlog/seed-stuck`, `/api/debug/backlog/seed-queued`, and
  `/api/debug/backlog/seed-headless-triage-session` — confirmed via
  `handleSeed`/`handleSeedQueued`/`handleSeedHeadlessTriageSession`
  (lines 58, 130, 238) that each calls `h.storage.CreateBacklogItem`, an
  entirely different domain object (`BacklogItem`, the item-queue/pipeline
  feature) from the session **review queue** under test here
  (`ReviewItem`/`ApprovalStore`/`ReviewQueuePoller`, per
  `escalation-reasoning.spec.ts:5-6,11-23`). Adding an equivalent
  review-queue seed endpoint would itself be new server-side/production
  code — explicitly out of bounds for a "test-only" fix, and AC4 forbids
  touching anything outside `tests/e2e/`.
- **Ordering/isolation note:** `tests/e2e/playwright.config.ts:23,29`
  (`fullyParallel: false`, `workers: 1`) means there is no true
  multi-worker race on the shared queue — but the queue is still shared,
  mutable, and drained across spec files in file/test execution order
  (`seedLiveSessions()` creates 3 idle sessions once, globally, in
  `test-server.ts:81,183-197`; both `review-queue.spec.ts`'s own
  `acknowledge button removes item` test and `escalation-reasoning.spec.ts`
  consume/acknowledge items from that same pool). Nothing in the sentinel
  test's file re-seeds before it runs, so its pass/fail is dependent on
  file-execution order today — a further sign the *sentinel* condition
  itself, not test sequencing, is the defect.

**Conclusion for Bug A:** every test-only path either (a) reintroduces
flakiness against a state AC2 already declares legitimate, (b) requires
production-adjacent server changes AC4 also forbids, or (c) papers over
without actually fixing the sentinel's broken "resolved" signal that a
second spec file (`escalation-reasoning.spec.ts`) also depends on. The
5-line `ReviewQueuePanel.tsx` change is the only fix that addresses the
actual defect. Requirements.md's conclusion holds.

## 2. Bug B: is a non-rewrite fix for the SessionWizard assertions possible?

**Verdict: rewrite is unavoidable — confirmed.**

`grep -rln SessionWizard web-app/src` returns exactly two files:
`web-app/src/components/sessions/SessionWizard.tsx` (self) and
`web-app/src/gen/session/v1/session_pb.ts` (an unrelated generated proto
symbol, not a component reference) — zero importers of the component.
`SessionWizard.tsx:4-7`'s own docblock: `@deprecated Use the Omnibar
(OmnibarContext) for session creation. This component is no longer
rendered.` Confirmed the actual route: `web-app/src/app/sessions/new/
page.tsx` is a pure client-side redirect (`router.replace('/?new=true')` or
`/?duplicate=...`, lines 11-18) with no reference to `SessionWizard`
anywhere in its 30 lines — it renders "Redirecting..." and hands off to the
Omnibar (`OmnibarCreationPanel.tsx`) on the root route.

Because `SessionWizard`'s elements (`wizard-step-label`, `session-title`,
`session-path`, `auto-yes-checkbox`, `create-session-button`) do not exist
in the DOM under any reachable code path, no selector strategy — data-testid,
ARIA role, text, or otherwise — can find them. An **additive-only** approach
(leave the two SessionWizard-asserting tests untouched, add two new tests
asserting on `OmnibarCreationPanel`) was considered explicitly: it satisfies
AC4's "no assertion/selector/test intent modified" literally, but the two
original tests would still fail forever (their target elements are
permanently absent), which directly violates AC2 ("0 failed"). There is no
way to satisfy both AC2 and AC4's literal "unmodified assertions" clause for
Bug B simultaneously — the rewrite in `2811df54e`
(`tests/e2e/review-queue.spec.ts:61-100`, using ARIA role/label locators
against the real Omnibar flow, with an inline comment explaining the
`SessionWizard` history at lines 62-69) is the only route to 0 failed.

## 3. Precedent for formally failing an unsatisfiable acceptance criterion

**Found: one directly relevant precedent, for the *mechanism*, not an exact
match for the *cause*.**

`project_plans/launchd-shell-sourcing/` documents the pipeline's explicit,
named support for this situation:

- `project_plans/launchd-shell-sourcing/requirements.md:89-93`: "This is a
  tooling capability gap outside this session's control, not a design or
  code question — no amount of research/planning resolves it. These two
  criteria will be reported via `/backlog/fail-N` with this explanation,
  per the pipeline's explicit allowance to fail a criterion that 'cannot be
  met as written.'"
- `project_plans/launchd-shell-sourcing/implementation/plan.md:54`: "`fail-N`
  reporting | The `/backlog/fail-N` skill convention for this pipeline:
  reports a specific acceptance-criteria index as blocked/not-implemented
  with reasoning, as opposed to `/backlog/done-N` for a passed criterion."
- `project_plans/launchd-shell-sourcing/implementation/plan.md:321-324`: "No
  implementation work — requirements.md is explicit that criteria 3 and 4
  are blocked by a missing MCP tool capability … and that only the
  `/backlog/fail-N` reporting step should be planned, not a workaround
  implementation."
- `project_plans/launchd-shell-sourcing/implementation/validation.md:11-13`:
  "...then all six acceptance criteria have an explicit, evidenced
  disposition (pass or blocked) recorded via `report_progress`/
  `/backlog/fail-N` — no criterion is silently skipped."

That precedent's root cause differs from this item's: launchd-shell-sourcing
criteria 3/4 were blocked by a missing *tool capability* (no
`ArchiveBacklogItem` MCP RPC exposed to the session), not by two criteria in
the same item being logically contradictory as AC2/AC4 are here. I found no
other project_plans document (searched `requirements.md`, `implementation/
plan.md`, `implementation/validation.md` across all ~134 project directories
for `cannot be (met|satisfied)`, `unsatisfiable`, `formally (mark|fail)`,
`fail-N`, `AC cannot`, `contradicts`, `mutually exclusive.*criter`, `two
acceptance criteria`, `silently overrid`) describing a case of two
acceptance criteria within one backlog item being mutually exclusive as
written. This appears to be the first instance of that specific shape of
conflict in this repo's SDD history, though the general `/backlog/fail-N`
mechanism ("report a criterion that cannot be met as written, with
reasoning, rather than silently overriding it") is established precedent
and directly applicable here.

## Verdict

AC4 has no viable compliant alternative under the verified root causes: Bug
A's only real fix touches production code (`ReviewQueuePanel.tsx`) because
every test-only path either reproduces a state AC2 already calls legitimate,
requires new server-side seed machinery (itself production code), or leaves
the actual defect live for a second consumer spec file; Bug B's only route
to `0 failed` requires rewriting two assertions because their target
component is provably unreachable and no selector strategy can address code
that never renders, and additive-only test changes cannot make a
permanently-failing assertion pass. AC4 should be formally failed via
`report_progress`/`/backlog/fail-N` with this evidence attached — the
precedent at `project_plans/launchd-shell-sourcing/requirements.md:89-93`
establishes this is the pipeline's intended mechanism for exactly this
situation, not an irregular escape hatch.
