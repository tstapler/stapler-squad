# UX Design: CI Status Badge in Diff Viewer + Block-on-Red-CI Rule

Scope: `project_plans/ci-status-diff-viewer/requirements.md`, grounded in
`research/ux.md` and the concrete components named in
`implementation/plan.md` (`CIStatusBadge.tsx` — Story 3.1.2; inline
block-explanation in `NotificationPanel.tsx` — Story 2.2.2; "Require CI
passing" checkbox in `RuleBuilderForm.tsx` — Story 1.1.4).

Sources read to ground this design against the actual code, not just the
plan's prose: `web-app/src/components/sessions/GitHubBadge.tsx`,
`web-app/src/components/sessions/StatusBadge.tsx`,
`web-app/src/components/ui/NotificationPanel.tsx:130-490`,
`web-app/src/components/rules/RuleBuilderForm.tsx:90-270`,
`server/services/approval_service.go:1-103`,
`web-app/src/components/sessions/DiffViewer.tsx`.

---

## Surface count and precise state resolution (Step 1)

Three user-facing surfaces, as scoped by the plan:

1. **Diff-viewer header CI badge** (`CIStatusBadge.tsx`)
2. **Blocked-Approve inline explanation** (`NotificationPanel.tsx`)
3. **"Require CI passing" checkbox** (`RuleBuilderForm.tsx`)

**Resolved: does "no-checks" render nothing, or a neutral badge?** These are
**two different cases that render two different things** — this is the crux
of AC7's "no error state" language and it is easy to conflate them:

| Case | `githubPrNumber` | `githubCheckConclusion` | Badge renders |
|---|---|---|---|
| No PR (one-off/directory session) | `0` | n/a | **Nothing** (`CIStatusBadge` returns `null` — Task 3.1.2a) |
| PR exists, no CI configured, OR data not yet polled, OR last poll errored | `>0` | `""` / `"neutral"` | **Neutral gray badge, text "No checks"** (`prBadgeUnknown` variant) |

Per the plan's own Pattern Decisions table ("Badge state vocabulary" row),
this is a **deliberate, documented 4-state design** — AC1's literal text
(passing/failing/pending/no-checks) is treated as authoritative over
`research/ux.md`'s elaborated 5-state ideal (which wanted a distinct
"fetch-error" state). The reason given: no backend signal exists today to
distinguish "last fetch failed" from "no checks configured are configured."

**Consequence for this design, not previously stated explicitly anywhere in
requirements/research/plan:** because "data hasn't arrived yet," "GitHub API
errored," and "PR genuinely has no CI configured" all collapse into the same
gray "No checks" badge, a reviewer cannot visually distinguish "this branch
truly has no CI" from "we don't know, ask GitHub." Given the feature's
stated purpose (research/ux.md §5, Functional JTBD: "Is this branch safe to
merge right now?"), this ambiguity is scoped-out by design, not a bug — but
it is the single largest residual risk in the visual design and is called
out explicitly in the UX Acceptance Criteria below (see "Known limitation"
item) rather than silently treated as equivalent to a true no-CI state.

---

## Surface 1: Diff-Viewer Header CI Badge

### Wireframe — all 5 render outcomes

```
┌─ Session Detail — Diff tab ──────────────────────────────────────────┐
│ Diff · feature/add-retry-logic                    +42 −13   [⟳]      │
│                                                                        │
│ (A) PR + CI passing:                                                  │
│   ┌──────────────┐                                                    │
│   │ ✅ Passing   │  ← <a role="status" aria-label="CI status: Passing"│
│   └──────────────┘     href=".../pull/42/checks" target=_blank>      │
│                                                                        │
│ (B) PR + CI failing:                                                  │
│   ┌──────────────┐                                                    │
│   │ ❌ Failing   │  ← same shape, class prBadgeBlocking (red)         │
│   └──────────────┘                                                    │
│                                                                        │
│ (C) PR + CI pending:                                                  │
│   ┌──────────────┐                                                    │
│   │ ⏳ Pending   │  ← class prBadgePending (amber)                    │
│   └──────────────┘                                                    │
│                                                                        │
│ (D) PR + no checks / not-yet-polled / last-poll-errored:              │
│   ┌──────────────┐                                                    │
│   │ ⬤ No checks │  ← class prBadgeUnknown (gray), low visual weight   │
│   └──────────────┘                                                    │
│                                                                        │
│ (E) No PR (one-off / directory session):                              │
│   (nothing rendered — header shows +42 −13 [⟳] only, no chip at all) │
├────────────────────────────────────────────────────────────────────────┤
│  @@ -12,7 +12,7 @@ func Foo() {                                       │
│  …diff content…                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

Icon+text+color triples (never color alone, per `StatusBadge.tsx`
precedent): ✅ Passing (green), ❌ Failing (red), ⏳ Pending (amber), ⬤/●
No checks (gray). Shape of the glyph, not hue, is the accessibility
guarantee (research/ux.md §3 — WCAG 1.4.1 / deuteranopia).

### Interaction flow

```
User opens a session's Diff tab
        │
        ▼
CIStatusBadge reads session.githubCheckConclusion / githubPrNumber / githubPrUrl
(already delivered via WatchSessions — AC3: no new fetch call)
        │
        ├─ githubPrNumber == 0 ──────────────────────────► render null
        │
        └─ githubPrNumber > 0
                │
                ▼
        map checkConclusion → {label, icon, class} (4-way switch)
                │
                ▼
        render <a role="status" aria-label="CI status: <label>"
                  title="CI: <label> · checked <Ns> ago" href="<prUrl>/checks">
                │
        ┌───────┴────────────────────────────────────────────┐
        │                                                      │
   User hovers/focuses                                   User clicks / presses
   (mouse or Tab key)                                     Enter/Space on badge
        │                                                      │
        ▼                                                      ▼
   Browser shows native tooltip                     New tab opens GitHub's
   from `title` attr: full state +                  PR Checks page
   staleness ("checked 2m ago")                      (prUrl + "/checks")
                                                       target=_blank,
                                                       rel=noopener noreferrer

Independently, in the background:
PRStatusPoller detects a checkConclusion change (even without a priority
boundary crossing — Story 3.2.1 fix) → publishes SessionUpdated event via
existing EventBus → WatchSessions delivers to this open tab → CIStatusBadge
re-renders with the new state, no user action, no page reload.
```

### Error / edge-case handling

| Case | What the user sees | Why |
|---|---|---|
| Session has no PR | Nothing — no chip, no placeholder, no skeleton | AC7 — explicitly "not an error state" |
| PR exists, CI genuinely has no workflow configured | Gray "No checks" badge | Informational, low visual weight, not alarming |
| PR exists, poller hasn't completed its first fetch yet | **Same gray "No checks" badge** — indistinguishable from the case above | Deliberate scope cut (plan's Pattern Decisions table); see "Known limitation" below |
| PR exists, last GitHub API poll failed/rate-limited | **Same gray "No checks" badge** — indistinguishable from a true no-CI PR | Same scope cut — no per-`Instance` last-fetch-error field exists to drive a 5th state |
| CI conclusion flips while the tab is open | Badge updates live, no refresh needed | Story 3.2.1 change-detection fix (AC4) |
| Badge staleness (data is old but not wrong) | Tooltip shows "checked Ns ago" via `formatRelativeTime(lastPrStatusCheck)` | Lets a suspicious reviewer judge freshness even though there's no explicit "stale" visual state |

**Known limitation (flagged, not silently accepted):** because "not yet
polled," "poll errored," and "genuinely no CI" all render identically, a
reviewer glancing at a gray "No checks" badge cannot tell "this branch has
no CI, approving is fine" from "GitHub is rate-limited right now and we
don't actually know." This is explicitly acknowledged and deferred in the
plan itself (Pattern Decisions table, Unresolved Questions §1) — this
design does not attempt to solve it, since no backend signal exists to
drive a 5th state, but it should not be treated as fully resolved either.
Recommended low-cost mitigation for a future iteration: differentiate the
`title` tooltip text using the existing `lastPrStatusCheck` timestamp —
"Not yet checked" (never polled) vs. "checked 2m ago" (polled recently,
genuinely no CI) vs. "last checked 45m ago" (suspiciously stale, might be
failing to poll) — all three are computable from data the plan already
threads into the badge (`lastChecked` prop, Task 3.1.2a), so this is a
tooltip-string change, not new plumbing. Not blocking for this ship.

---

## Surface 2: Blocked-Approve Inline Explanation (`NotificationPanel.tsx`)

### Wireframe

```
┌─ Notification Panel ──────────────────────────────────────────────┐
│ 🔒 Approval Pending                              [Permission]     │
│ my-feature-branch                                                  │
│ 🔧 Bash                                                             │
│ npm publish --tag latest                                           │
│ 📁 worktrees/my-feature-branch                                     │
│                                                                      │
│ (before clicking Approve — flag ON, CI failing, no pre-check)      │
│                              2m ago    [✓ Approve]  [✗ Deny]       │
│                                                                      │
│ (user clicks Approve → optimistic pending state)                   │
│                              2m ago    [ … ]         [✗ Deny]      │
│                                                                      │
│ (RPC rejects with CodeFailedPrecondition → blocked state)           │
│  ⚠️ Approval blocked: CI is failing on this branch — review        │
│     before approving. [View CI run ↗]                              │
│                              2m ago    [✓ Approve]  [✗ Deny]       │
│                                        (still clickable, not        │
│                                         disabled — see flow below)  │
│                                                                      │
│                                        ⚠ Approve anyway             │
│                                        ← own line, below the        │
│                                          primary row; small ghost/  │
│                                          warning-outline text       │
│                                          button, NOT a 3rd button   │
│                                          of equal weight (Story     │
│                                          2.2.4 / Task 2.2.4c)       │
│                                                                      │
│ (reviewer clicks "Approve anyway" → confirm dialog, see below)      │
│  ┌ Approve Despite Failing CI? ───────────────────────────────┐    │
│  │ CI is currently failing on this branch.                    │    │
│  │ ⚠ This bypasses the CI-red block for this approval only.   │    │
│  │   The override is recorded for other reviewers to see.     │    │
│  │                          [Approve Anyway]      [Cancel]     │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                      │
│ (override succeeds → resolved state)                                │
│                              2m ago    ⚠ Approved (CI override)     │
│                                        ← distinct from the plain    │
│                                          "✓ Approved" badge, same   │
│                                          resolvedBadge chrome, new  │
│                                          icon+text only (see below) │
└──────────────────────────────────────────────────────────────────┘
```

### Interaction flow

```
Reviewer clicks "✓ Approve" on an approval_needed notification
        │
        ▼
resolveApproval(approvalId, "allow", ids) fires
setPendingApprovals[approvalId] = true   → button shows "…", disabled
        │
        ▼
ApprovalService.ResolveApproval (server)
        │
        ├─ flag review:block-approval-on-ci-failure == false
        │  OR session has no PR (githubPrNumber == 0)
        │  OR githubCheckConclusion != "failure"
        │         │
        │         ▼
        │   approvalStore.Resolve() succeeds → 200 OK
        │         │
        │         ▼
        │   resolvedApprovals[approvalId] = "allow"
        │   UI renders "✓ Approved" badge (terminal state)
        │
        └─ flag == true AND githubPrNumber > 0 AND checkConclusion == "failure"
                  │
                  ▼
           connect.CodeFailedPrecondition returned,
           BEFORE approvalStore.Resolve() is called
           (approval remains PENDING in the store — not consumed)
                  │
                  ▼
           Frontend catch block (Task 2.2.2b — new logic required):
           inspect ConnectRPC error code; if FailedPrecondition,
           store message in new `blockedApprovals[approvalId]` state
           — NOT the existing generic "expired" branch
                  │
                  ▼
           setPendingApprovals[approvalId] = false → button re-enabled
           Inline warning renders above the action row:
           "⚠️ Approval blocked: CI is failing on this branch —
            review before approving. [View CI run]"
                  │
                  ▼
           Reviewer's available next actions:
             • Click "✗ Deny" — always works, ends the approval negatively
             • Click "✓ Approve" again — re-attempts, blocked again
               unless CI has since turned green or someone disabled
               the global flag (see Exit-Path Analysis below)
             • Click "View CI run" — opens the failing run in a new tab
               to investigate
             • Click "⚠ Approve anyway" — the scoped, per-approval
               override added by Story 2.2.4 (see dedicated subsection
               below for the full wireframe/interaction/badge spec)
```

### "Approve anyway" Override — Wireframe and Interaction Spec (Story 2.2.4 / Task 2.2.4c)

This subsection was added in this revision to close the gap left when ux.md's
original pass predated plan.md's adversarial-review addition of Story 2.2.4.
It covers the override button itself, its confirm step, and the post-override
badge — the three things Task 2.2.4c's literal scope (proto field, RPC call,
clear the block-error state) does not fully specify on its own.

**Button visual weight — why it must not read as a 3rd equal-weight button.**
This is an override of a safety gate, not a normal decision alternative like
Approve vs. Deny, so its visual weight must communicate "you are choosing to
bypass a warning," not "here is a third equally-valid option":

- Renders only when a stored block error exists for that `approvalId` — same
  condition Task 2.2.4c already specifies, so no new state is introduced to
  decide *when* it shows.
- Positioned on its **own line below** the Approve/Deny row, not inline
  beside them — this positional demotion is the primary weight signal.
- Styled as a small ghost/text button using `vars.color.warning` (not
  `vars.color.success` used by `approveButton` or `vars.color.error` used by
  `denyButton`), mirroring the existing outline-button recipe at
  `web-app/src/components/ui/NotificationPanel.css.ts:450-504`
  (`approveButton`/`denyButton`: 1px colored border, transparent background,
  colored text, fills to solid color + white text on hover) but at a smaller
  font-size (e.g. `0.72rem` vs. their `0.78rem`) and without the `font-weight:
  600` — a visibly quieter, third-tier action.
- Label: **"⚠ Approve anyway"** — reuses the same ⚠️ glyph already rendered
  in the inline block-explanation text immediately above it (Task 2.2.2b),
  visually tying the override to the specific warning it overrides (Gestalt
  proximity/similarity), consistent with this doc's existing "icon+text, not
  color alone" rule (AC14 below).
- Click target: a real `<button>` element, and — despite the smaller visual
  footprint — it must keep the same `44px` mobile min-height media query
  already applied to `approveButton`/`denyButton`
  (`NotificationPanel.css.ts:470-475`). Visual weight and touch-target size
  are independent concerns; shrinking one must not shrink the other.

**Confirm-on-click.** Given this bypasses a safety gate whose own design
goal (requirements.md Goal 2) is that it must remain overridable but not
trivially so, the override button opens one confirm step rather than firing
the RPC immediately. Rather than inventing a new confirm idiom, this reuses
the repo's existing consequential-action-confirm pattern verbatim: the
`confirmDialog`/`dialogContent`/`dialogActions`/`submitButton`/
`cancelButton`/`warningText` classes already defined in
`web-app/src/components/sessions/SessionCard.css.ts` and used for the
"Run Autonomously" confirmation in
`web-app/src/components/sessions/SessionActionsOverflow.tsx:401-430`
(`role="dialog"`, `aria-modal="true"`, portal-rendered to `document.body` per
`.claude/rules/css-architecture.md`'s createPortal-for-overlays rule). Dialog
content, matching that precedent's brevity (2 sentences + 2 buttons, no extra
fields):

- Title: "Approve Despite Failing CI?"
- One line of context: "CI is currently failing on this branch."
- One `warningText`-styled caveat line: "This bypasses the CI-red block for
  this approval only. The override is recorded for other reviewers to see."
- "Approve Anyway" (`submitButton`) — fires the exact call Task 2.2.4c
  already specifies, `resolveApproval(approvalId, "allow", group.allIds,
  true)`. The dialog is a pure client-side gate in front of that call; it
  adds no new RPC surface.
- "Cancel" (`cancelButton`) — closes the dialog, no RPC call, blocked state
  is unchanged (Deny / Approve / Approve-anyway all still available) — a
  true no-op, preserving "user control and freedom."

(`NotificationPanel.tsx` does not currently import `SessionCard.css.ts`;
whether the implementer cross-imports these classes or defines an equivalent
local recipe in `NotificationPanel.css.ts` is an implementation detail — the
interaction *structure* should match this precedent, not invent a new one.)

**Post-override visual indicator — "approved despite failing CI."** Verified
against Task 2.2.4c's literal text: on success it "clear[s] the stored
block-error state for that `approvalId` ... same success path Approve
already takes at `:166`" — i.e. `resolvedApprovals[approvalId]` is set to
plain `"allow"`, which renders the exact same `<span className={resolvedBadge}
data-decision="allow">✓ Approved</span>` as any ordinary approval
(`NotificationPanel.tsx:452-453`). **As specified, there is no way for a
later viewer to tell an override apart from a normal approval — this is
the gap this review exists to close**, and it is precisely what
`research/ux.md`'s Social JTBD ("a visible, checkable record") depends on.

Checked whether any surface beyond `NotificationPanel.tsx` could show this
more cheaply: `SessionCard.tsx`/`SessionRow.tsx` render no approval-decision
state at all today (grepped `approval_decision`/`ApprovalDecision`/
`resolvedBadge` across both — zero hits), so there is no existing
session-card surface to extend at low cost; building one from scratch would
be new UI beyond this gap's proportional scope. The one existing, low-cost
path is `NotificationPanel.tsx`'s own history view, which already has a
real, persisted (not just in-memory) signal to key off: `resolvedApprovals`
is seeded on load from `notification.metadata["approval_decision"]`
(`NotificationPanel.tsx:140-160`), a value the server stamps via
`as.notificationStore.SetMetadata(req.Msg.ApprovalId, "approval_decision",
req.Msg.Decision)` at `server/services/approval_service.go:81` — currently
always the literal decision string (`"allow"`/`"deny"`), with no override
distinction. Task 2.2.4b already computes the exact `blocked` boolean needed
("when `blocked && req.Msg.OverrideCiBlock`... log a distinct line") but
threads it no further than that log line.

**Minimal-cost recommendation (reuse existing plumbing, not new UI):** when
`blocked && req.Msg.OverrideCiBlock` is true — the same condition Task
2.2.4b already uses for its distinct log line — stamp `approval_decision` as
`"allow_override"` instead of plain `"allow"` at that one `SetMetadata` call
site (`approval_service.go:81`), a one-line change beyond Task 2.2.4b's
current scope. On the frontend, extend `resolvedApprovals`'s type from
`"allow" | "deny" | "expired"` to add `"allow_override"`
(`NotificationPanel.tsx:136`), add an `else if (decision === "allow_override")`
branch to the existing seeding effect (`:146-158`), and render a 4th
`resolvedBadge` branch: `data-decision="allow_override"`, text **"⚠ Approved
(CI override)"**. No new CSS is needed — `resolvedBadge` itself carries no
`data-decision`-keyed color today (verified: it's layout-only chrome; the
existing "✓ Approved"/"✗ Denied"/"Expired" badges are already
icon+text-differentiated, not color-differentiated), so the override badge
follows the exact same icon+text idiom already in place, consistent with
AC14 below. This is the cheapest option that closes the gap without
inventing a new UI element — it reuses a field, a seeding effect, and a
badge component that all already exist end-to-end.

---

### Error / edge-case handling

| Case | What the user sees | Exit offered |
|---|---|---|
| Block fires (flag on, PR CI failing) | Inline warning text above Approve/Deny, Approve button re-enabled (not disabled) after the failed attempt; "⚠ Approve anyway" override available below the primary row (Story 2.2.4, see subsection above) | Deny; retry Approve; "Approve anyway" (with confirm step); investigate via "View CI run" link |
| Session has no PR | Block never fires (short-circuits on `githubPrNumber == 0` — AC7) | Normal Approve flow, unaffected |
| Flag is off (default) | Block never fires | Normal Approve flow, unaffected |
| Approval already resolved elsewhere (existing "expired" case) | Existing generic "Expired" badge — **must remain a separate code path** from the new blocked-CI case per Task 2.2.2b, since both currently land in the same `catch` block today | "Expired" is terminal — no action possible, matches existing behavior (out of scope for this feature to change) |
| CI turns green while the blocked notification is still open | No push update to this specific inline warning — it only re-evaluates on the next Approve click | Reviewer must click Approve again to discover it now succeeds; **not live-updating**, unlike the diff-viewer badge (see Recommendation below) |

**Important distinction from the diff-viewer badge:** unlike Surface 1
(which updates live via `WatchSessions`), the blocked-approval message is
**not proactive** — nothing in the plan pre-checks CI status before the user
clicks Approve, and nothing re-evaluates the block after it's shown. The
reviewer only discovers the block by attempting the action and reading the
error. This is a real, if minor, friction point relative to Nielsen's
"visibility of system status" heuristic — a reviewer would ideally see the
warning *before* clicking Approve, not as a rejected-attempt message. Not
blocking for ship (the plan does not describe any pre-check data path into
`NotificationPanel.tsx` to drive this), but flagged as a follow-up
recommendation.

### Exit-Path / Human-Override Analysis (explicitly requested verification)

**Update to this revision: the override affordance is now designed, not
just referenced.** This section originally found a real gap — plan.md's
Story 2.2.2/2.2.3, as they stood when this doc was first written, shipped a
hard block with no scoped per-approval override. That finding held at the
time and drove a documented product/eng escalation (see the original
recommendation, preserved below). Since then, plan.md's adversarial review
added **Story 2.2.4** ("Override the AC5 block with an audited 'Approve
anyway'") to close exactly this gap. This revision closes the loop on the
UX side: the "Approve anyway" Override — Wireframe and Interaction Spec"
subsection above gives that mechanism a concrete button design, confirm
step, and post-override badge — the three things Story 2.2.4's own scope
(a proto field, a guard-clause change, and a bare RPC call) does not fully
specify by itself. **AC8 below now reads PASSES, not FAILS**, on that
basis — with one residual, explicitly named gap (the post-override badge
needs a small addition beyond Task 2.2.4b/2.2.4c's current written scope;
see the "Minimal-cost recommendation" above).

The original finding, for context on what changed: reading Story 2.2.2's
acceptance criteria and Task 2.2.2a/b line by line (as they stood before
Story 2.2.4 was added), the block returned a hard
`connect.CodeFailedPrecondition` error and nothing in the plan added an
"approve anyway" parameter, a second confirmation button, or any bypass
affordance scoped to a single approval. The only ways out of a blocked
state were:

1. **Deny** — always available, but this is not an override; it discards
   the approval rather than letting the reviewer approve despite red CI.
2. **Wait for CI to turn green**, then retry Approve — works, but is not
   useful when the reviewer has *judged* the failure irrelevant (flaky
   test, known-broken unrelated check, etc.) and wants to proceed now.
3. **Global flag toggle** — a user with Settings access can flip
   `review:block-approval-on-ci-failure` off in Settings → Feature Flags.
   This unblocks Approve for **every pending and future approval across
   every session**, not just the one in front of the reviewer, until
   someone remembers to turn it back on. This is the *rollback procedure*
   the plan documents (`implementation/plan.md`'s Risk Control section:
   "toggle the flag back off... zero blast radius to other rules") — but
   using a global kill switch as the day-to-day override mechanism for a
   single "I've reviewed this, approve it anyway" decision is a mismatch
   of scope: it is an admin action being used as a per-item workaround, and
   it silently removes the guard for every other reviewer/session in the
   interim.

This directly contradicted the feature's own stated framing.
`research/ux.md` §5 (Social JTBD) says the block "gives the reviewer a
system-level reason ('the tool wouldn't let me') rather than requiring them
to personally remember to check" — i.e., the rule is meant to be a
**speed bump with a documented override**, not an unconditional wall. As
planned at the time, a reviewer who had legitimately decided a red check was
safe to override had no in-context way to record that decision; they would
have had to either lie via Deny (not applicable — Deny doesn't approve),
wait, or ask an admin to globally disable the safety net for everyone. This
was a genuine UX gap, not a nitpick — it risked the block becoming exactly
the "hard, unbypassable gate" the requirements doc explicitly says it must
not be (requirements.md Goal 2: "not a hard-coded always-on gate").

**Resolution (current state, this revision):** Story 2.2.4 adds exactly the
single additional affordance this section originally recommended — a
secondary "Approve anyway" button that resends `ResolveApproval` with an
explicit `overrideCiBlock: true` field (Task 2.2.4a), server-side logs the
override distinctly (Task 2.2.4b, mirrors the existing
`log.Info("[ApprovalService] resolved approval"...)` pattern). The "Approve
anyway" Override — Wireframe and Interaction Spec" subsection above now
gives that affordance its visual weight (secondary/warning styling, own
line below the primary Approve/Deny row), its confirm step (reusing the
`SessionCard.css.ts` confirm-dialog pattern), and closes the one piece the
original recommendation left open — the resulting resolved state carrying
"a small badge distinguishing 'approved despite failing CI' for audit-trail
purposes" is now a concrete, minimal-cost design (see "Post-override visual
indicator" above), not just an "optionally." This is scoped, reviewable,
and preserves the "visible, checkable record" social value `research/ux.md`
describes, without requiring a global settings change per override. The one
remaining action item is the small addition to `approval_service.go:81`
and `NotificationPanel.tsx:136-158` needed to carry the override distinction
into the persisted `approval_decision` metadata — flagged for product/eng
to confirm is in scope for Task 2.2.4b/c, since it is not covered by either
task's current written text.

---

## Surface 3: "Require CI Passing" Checkbox (`RuleBuilderForm.tsx`)

### Wireframe

```
┌─ Rule Builder — structured mode ─────────────────────────────────┐
│ Tool:      ( ) Name  (•) Category  ( ) Pattern                    │
│            [ Bash                                    ]            │
│                                                                     │
│ Command pattern (regex): [ ^npm publish            ]              │
│                                                                     │
│ ☐ Safe Python imports only                                         │
│ ☑ Require CI passing on this branch                                │
│    ℹ CI must be green (not pending, failing, or unconfigured) for  │
│      this rule to auto-approve. Sessions with no associated PR     │
│      never match this condition.                                   │
│                                                                     │
│ Decision:  (•) Allow  ( ) Escalate  ( ) Deny                       │
│                                                                     │
│                                          [Cancel]  [Save Rule]     │
└────────────────────────────────────────────────────────────────────┘
```

Mirrors the existing `safePythonImportsOnly` checkbox exactly (same
component, same visual weight, same position in the structured-criteria
list) — per Task 1.1.4a's explicit instruction to copy that pattern at its
6 cited call sites, not invent a new form idiom.

### Interaction flow

```
Rule author opens RuleBuilderForm (new or edit)
        │
        ▼
Checks "Require CI passing on this branch"
        │
        ▼
onChange → setRequireCiPassing(true)   (new state, mirrors safePythonImportsOnly)
        │
        ▼
User clicks "Save Rule"
        │
        ▼
Submit payload includes requireCiPassing: true
        │
        ▼
Server persists RuleSpec.RequireCIPassing → auto_approve_rules.json
        │
        ▼
On next tool-use classification for that rule's matching command:
  ClassificationContext.CIStatus populated from the requesting session's
  GitHubCheckConclusion (only if session has a PR — else stays "")
        │
        ▼
matchesRule: existing conditions (regex, tool name, etc.) AND
             (RequireCIPassing == false OR CIStatus == "success")
        │
        ├─ all match → auto-approve fires
        └─ CIStatus != "success" (or no PR) → falls through to Escalate,
           same as any other non-matching rule (no special-cased UI
           needed here — it's the existing escalation path)
```

### Error / edge-case handling

| Case | What the user sees | Exit path |
|---|---|---|
| Rule saved with `requireCiPassing: true` but the session it would apply to has no PR | Rule silently never auto-matches for that session (falls to Escalate, same as any unmatched rule) — **no distinct error UI**, this is existing classifier behavior | Reviewer sees the normal manual-approval notification instead; no dead end, just no auto-approval |
| User edits an existing rule and the checkbox state doesn't match what's persisted | N/A — `editRule` seed effect (`RuleBuilderForm.tsx:206-235`) should read `editRule.requireCiPassing` on load, same as `safePythonImportsOnly` at line 226 | Standard form re-population; verify this is added to the seed effect, not just the save payload (a gap risk if only the submit path is wired and the edit-seed path is missed) |
| Mode switch (structured ↔ regex) | `applyModeSwitch("regex")` clears `safePythonImportsOnly` today (`RuleBuilderForm.tsx:269`) — `requireCiPassing` should be **decided deliberately**: does switching to regex mode also clear "require CI passing"? Not addressed in the plan. | Flag for implementer: if regex mode is meant to keep composability with `ci_passing` (AC6's own example combines a regex `CommandPattern` with `RequireCIPassing`), clearing it on mode switch would silently lose the CI requirement. Recommend NOT clearing it on mode switch, unlike `safePythonImportsOnly` (which is structured-mode-only by nature) |

No approval-blocking or destructive action lives on this surface — it's
pure rule authoring, so the "dead end" analysis that matters for Surfaces 1
and 2 doesn't apply here in the same way.

---

## UX Acceptance Criteria

### Task completion (clicks/steps)

1. A reviewer can determine a session's CI status from the diff viewer in
   **0 clicks** (glanceable at page load, no expansion needed) — badge is
   visible in the header the moment the Diff tab is open, per the
   "rollup-first" pattern (research/ux.md §1).
2. A reviewer can view the underlying GitHub Actions run/check detail in
   **1 click** (badge click → new tab opens `<prUrl>/checks`).
3. A rule author can add a CI-passing requirement to a new or existing rule
   in **1 click** (checkbox) **+ 1 click** (Save) = 2 total actions, no
   additional dialog or confirmation step.
4. A blocked reviewer can attempt to understand *why* Approve failed in
   **0 additional clicks** beyond the Approve click itself — the
   explanation renders inline immediately on the same click that triggered
   the block, not behind a separate "why?" affordance.

### Error states (message + offered action)

5. When CI is failing and the block flag is on, the Approve action fails
   with the message **"Approval blocked: CI is failing on this branch —
   review before approving."** and offers **"View CI run"** as a clickable
   inline link — not a silently disabled/grayed-out button (requirements
   AC5, verified against Task 2.2.2b).
6. When a session has no PR, the diff-viewer badge renders nothing (not a
   placeholder, not a spinner, not "Unknown") — verified this is
   distinguishable from the "No checks" case by PR presence elsewhere in
   `SessionDetailView` (the PR badge itself is also absent for no-PR
   sessions, giving a second, consistent signal).
7. **Known limitation, explicitly accepted, not silently dropped:** "data
   not yet fetched" and "last GitHub API poll failed" both render as the
   same neutral "No checks" badge as a genuinely-no-CI PR. No AC requires
   fixing this for this ship (see plan's Unresolved Questions §1); flagged
   here so it isn't rediscovered as a "bug" later without context.

### No dead ends (explicit override/exit-path verification — the task's primary ask)

8. **PASSES, with the design in this revision — previously FAILED.** The
   blocked-Approve state (Surface 2) has exit paths for *abandoning* the
   approval (Deny), *waiting* (CI turns green, retry), a *global, unscoped*
   admin workaround (disable the feature flag for all sessions), and now a
   **scoped, per-approval human override**: Story 2.2.4's "Approve anyway"
   button, designed above (own line below Approve/Deny, warning styling,
   confirm-on-click, `overrideCiBlock: true`, distinct server-side log
   line). This no longer risks becoming the "hard, unbypassable gate"
   requirements.md Goal 2 says the feature must not be. Two conditions
   remain before recommending the flag on-by-default: (a) product/eng
   confirms the button-weight/confirm-dialog/post-override-badge design
   above is in scope for Task 2.2.4c's implementation (its current written
   text covers only the RPC call and clearing the block-error state, not
   these three pieces); (b) the one-line `approval_decision` metadata
   addition (see "Post-override visual indicator" above) is accepted as
   in-scope for Task 2.2.4b, since without it AC16 below cannot pass.
9. The diff-viewer badge (Surface 1) has no dead end — it is read-only and
   non-blocking by construction (AC1–AC4, AC7); there is nothing to be
   "stuck" in.
10. The rule-builder checkbox (Surface 3) has no dead end — unchecking it
    or canceling the form fully reverses the action; a rule with
    `requireCiPassing: true` that never matches simply falls through to
    the existing Escalate path (manual review), which is itself a
    non-dead-end (human always sees it).

### Accessibility

11. **Keyboard navigation:** the CI badge (Surface 1) is a native `<a>`
    element — reachable via Tab, activatable via Enter/Space, with the
    browser's default focus ring (no custom `tabIndex`/`onKeyDown` needed
    because it's a real link, not a `<div>` pretending to be one). Approve/
    Deny buttons (Surface 2) and the checkbox (Surface 3) are native
    `<button>`/`<input type="checkbox">` elements — same guarantee.
12. **Screen-reader labels:** `role="status"` + `aria-label="CI status:
    <label>"` on the badge (Task 3.1.2a, mirrors `StatusBadge.tsx:94-104`
    and `GitHubBadge.tsx:127` verbatim) are **sufficient** — verified
    against the existing precedent components, which already ship this
    exact pattern in production. `role="status"` additionally makes the
    badge a live region, so a screen-reader user with the diff tab open
    when CI status changes (Story 3.2.1's live-update fix) will hear the
    change announced without needing to re-navigate to the badge — this is
    a meaningful accessibility benefit of the live-update fix beyond its
    stated AC4 purpose, worth calling out to whoever verifies this ship.
13. **Color contrast ≥ 4.5:1:** not re-litigated here — the plan
    deliberately reuses `GitHubBadge.css.ts`'s existing
    `prBadgeReady`/`prBadgeBlocking`/`prBadgePending`/`prBadgeUnknown`
    classes verbatim rather than introducing new colors (Pattern Decisions
    table), and those classes are already in production use elsewhere in
    the app. **Recommendation, not a blocker:** a reviewer should still
    do one visual spot-check of the "No checks" gray badge
    (`prBadgeUnknown`, `vars.color.surfaceSubtle`/`textSecondary`) once
    implemented, since gray-on-gray combinations are the most likely of
    the four variants to sit close to the 4.5:1 boundary even when reused
    from elsewhere — a quick contrast-checker pass during Story 3.1.3's
    unit tests or e2e visual review is cheap insurance.
14. **Color is never the sole signal:** every state pairs a distinct glyph
    (✅/❌/⏳/⬤) with a text label, per `StatusBadge.tsx`'s established
    pattern — verified this holds for all 4 non-null badge states plus the
    inline warning icon (⚠️) on the blocked-Approve message, and now the
    ⚠ glyph on the "Approve anyway" button and its post-override badge
    (AC15/AC16 below).

### "Approve anyway" override (added this revision)

15. **Override button — visibility, click target, weight:** the "⚠ Approve
    anyway" button (a) renders only when a stored block error exists for
    that `approvalId` (never alongside a normal, non-blocked Approve/Deny
    pair); (b) is a native `<button>`, own line below the Approve/Deny row,
    styled with `vars.color.warning` (not the `success`/`error` colors used
    by Approve/Deny) at a visibly smaller font-size/weight than those two,
    so it reads as secondary/warning rather than a third equal-weight
    choice; (c) keeps the same `44px` mobile min-height touch target as
    Approve/Deny despite its smaller visual footprint; (d) clicking it opens
    a confirm dialog ("Approve Despite Failing CI?" + one warning line +
    "Approve Anyway"/"Cancel") before the RPC fires — reusing
    `SessionCard.css.ts`'s existing `confirmDialog` pattern, not a new
    idiom; (e) "Cancel" is a true no-op — no RPC call, blocked state
    unchanged. All five are verifiable against the rendered component
    without needing to read source.
16. **Post-override indicator:** once an override succeeds, the resolved
    badge for that approval reads **"⚠ Approved (CI override)"**
    (`data-decision="allow_override"`), distinguishable by icon+text from
    the plain "✓ Approved" badge shown for a non-override approval — and
    this distinction **survives a page reload** (backed by the persisted
    `approval_decision` metadata value `"allow_override"`, not just
    in-memory component state). This AC requires the one-line addition to
    `approval_service.go:81` named above; it does not pass under Task
    2.2.4b/c's currently-written scope alone.

---

## Summary of design gaps to route back to product/eng

1. **Blocked-Approve override gap (Surface 2, AC 8 above) — CLOSED this
   revision, one follow-up remains.** Story 2.2.4 now adds the scoped
   per-approval override this item originally flagged as missing; the
   "Approve anyway" Override — Wireframe and Interaction Spec" subsection
   gives it a concrete button design, confirm step, and post-override
   badge. Remaining action item: confirm with product/eng that (a) the
   button-weight/confirm-dialog design and (b) the one-line
   `approval_service.go:81` + `NotificationPanel.tsx:136-158` addition
   needed for the post-override badge (AC16) are accepted as in-scope for
   Tasks 2.2.4b/2.2.4c — neither is covered by those tasks' current
   written text.
2. Badge ambiguity between "not yet polled," "poll errored," and
   "genuinely no CI" (Surface 1) — explicitly accepted scope cut in the
   plan; documented here so it isn't mistaken for an oversight later.
3. Blocked-Approve message is reactive (discovered only after clicking
   Approve), not proactive — minor Nielsen "visibility of system status"
   friction, not blocking.
4. `RuleBuilderForm`'s mode-switch clearing behavior for
   `requireCiPassing` is unspecified in the plan — needs an explicit
   decision (recommend: do not clear on structured↔regex switch, since
   AC6's own example composes a regex pattern with CI-passing).
