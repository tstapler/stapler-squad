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
```

### Error / edge-case handling

| Case | What the user sees | Exit offered |
|---|---|---|
| Block fires (flag on, PR CI failing) | Inline warning text above Approve/Deny, Approve button re-enabled (not disabled) after the failed attempt | Deny; retry Approve; investigate via "View CI run" link |
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

**Finding: there is no scoped, per-approval human override.** Reading
Story 2.2.2's acceptance criteria and Task 2.2.2a/b line by line: the block
returns a hard `connect.CodeFailedPrecondition` error and nothing in the
plan adds an "approve anyway" parameter, a second confirmation button, or
any bypass affordance scoped to a single approval. The only ways out of a
blocked state are:

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

This directly contradicts the feature's own stated framing.
`research/ux.md` §5 (Social JTBD) says the block "gives the reviewer a
system-level reason ('the tool wouldn't let me') rather than requiring them
to personally remember to check" — i.e., the rule is meant to be a
**speed bump with a documented override**, not an unconditional wall. As
currently planned, a reviewer who has legitimately decided a red check is
safe to override has no in-context way to record that decision; they must
either lie via Deny (not applicable — Deny doesn't approve), wait, or ask
an admin to globally disable the safety net for everyone. **This is a
genuine UX gap, not a nitpick** — it risks the block becoming exactly the
"hard, unbypassable gate" the requirements doc explicitly says it must not
be (requirements.md Goal 2: "not a hard-coded always-on gate").

Recommended fix (design-level, not mandating an implementation): add a
single additional affordance to the blocked state — e.g. a secondary
"Approve anyway" button that resends `ResolveApproval` with an explicit
`overrideCiBlock: true` field, server-side logs the override (mirrors the
existing `log.Info("[ApprovalService] resolved approval"...)` pattern,
Observability Plan), and the resulting "✓ Approved" state could optionally
carry a small badge distinguishing "approved despite failing CI" for
audit-trail purposes. This is scoped, reviewable, and preserves the
"visible, checkable record" social value `research/ux.md` describes,
without requiring a global settings change per override. This is flagged
here for product/eng sign-off before ship — it is out of scope for this
UX-design pass to unilaterally add to the implementation plan.

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

8. **FAILS, as currently planned.** The blocked-Approve state (Surface 2)
   has exit paths for *abandoning* the approval (Deny) or *waiting*
   (CI turns green, retry), and a *global, unscoped* admin workaround
   (disable the feature flag for all sessions), but **no scoped, per-approval
   human override** ("I've reviewed this, approve it anyway despite red
   CI"). This risks becoming the "hard, unbypassable gate" requirements.md
   Goal 2 explicitly says the feature must not be. See the "Exit-Path /
   Human-Override Analysis" section above for the finding and a recommended
   scoped fix (`overrideCiBlock` flag + audit log entry). **This should be
   resolved — either by adding the override, or by an explicit, written
   product decision that the global flag toggle is the intended and
   accepted override mechanism — before this ships as an on-by-default or
   widely-recommended setting.** Since the flag defaults off, this doesn't
   block shipping the feature *disabled*; it blocks recommending it be
   turned *on* without this gap being closed or explicitly accepted.
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
    inline warning icon (⚠️) on the blocked-Approve message.

---

## Summary of design gaps to route back to product/eng

1. **Blocked-Approve override gap (Surface 2, AC 8 above)** — no scoped
   per-approval override exists; only Deny, wait, or a global flag toggle.
   Needs an explicit product decision before the flag is recommended
   on-by-default.
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
