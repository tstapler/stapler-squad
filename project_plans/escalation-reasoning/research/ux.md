# UX Research: Escalation Reasoning on Review Queue Items

Phase 2 (research) input for `project_plans/escalation-reasoning/`. Grounds
recommendations in the actual renderable primitives in
`web-app/src/components/sessions/ReviewQueuePanel.tsx`,
`web-app/src/components/ui/Button.tsx`/`Button.css.ts`, and
`ApprovalAnalyticsPanel.tsx` — no new dependency, no new component library,
per AC6/AC7 in `requirements.md`.

## 0. What's actually renderable today (grounding)

- **`itemContext`** (`ReviewQueuePanel.css.ts:202-207`) — a plain `<p>`,
  `italic`, `textSecondary` color, no background/border. Currently rendered
  only when `queueItem.context && !queueItem.metadata?.["pending_approval_id"]`
  (`ReviewQueuePanel.tsx:718-720`) — i.e. it is *suppressed* for exactly the
  items this feature targets. AC6 requires a new render branch inside the
  `pending_approval_id` block (`:726-743`), not touching that suppression.
- **Card structure for a pending-approval item** (`:704-769`): title + compact
  badge → full `ReviewQueueBadge` (priority + `AttentionReason`, itself
  emoji+text via `StatusBadge.tsx`'s `getAttentionReasonInfo`) → (today)
  suppressed context → pattern name → `commandPreview` (`<pre>`, monospace
  box) → cwd → orphaned badge → session details (program/branch/path/tags) →
  footer (last-activity, diff stats). The escalation reason has no existing
  slot; it must be inserted into this sequence.
- **`Button`** (`Button.tsx` + `.css.ts`): 4 intents, visually distinct by
  design — `primary` (solid `vars.color.primary` bg), `danger` (solid
  `vars.color.error` bg), `secondary` (`hoverBackground` fill +
  `borderColor` border — visible border/fill but not solid-color), `ghost`
  (transparent, no border, text-only until hover). No focus-only or
  data-attribute-only "intent" — the change from `ghost` → `secondary` for
  "Create Rule" (AC7, `:820`) is real, visible chrome (a border and fill
  appear) without touching color semantics reserved for `primary`/`danger`.
  `disabled` state exists (`opacity 0.5`, `pointer-events: none`,
  `cursor: not-allowed`) — relevant for a "reason not yet enriched" state if
  the button needs to be inert rather than hidden.
- **Existing category→icon/label lookup pattern**: `StatusBadge.tsx`'s
  `getAttentionReasonInfo(reason): {label, icon, variant}` is the established
  codebase idiom for "render a category as emoji + text, keep semantics in a
  `variant` string, put `aria-hidden` on the emoji and full text in
  `aria-label`" (see `ReviewQueueBadge.tsx:93-95,101-106`: emoji wrapped in
  `<span aria-hidden="true">`, the parent carries `aria-label`/`title` with
  the human sentence). **This is the pattern to reuse for the 5 escalation
  categories** (no-match / explicit-rule / domain-age / secret-scan /
  unclassifiable) — a plain object/function map, not a new component, not a
  new dependency. Emoji-as-icon is already load-bearing UI in this codebase
  (`ReviewQueueBadge`, priority dots), so using e.g. a distinct emoji per
  escalation category inside the existing `itemContext` `<p>` is consistent
  with the existing visual language, satisfies "no new UI library," and
  gives non-color visual differentiation for free (color-only differentiation
  would fail WCAG 1.4.1 "use of color" if it were the *only* signal, so text
  + emoji, not color-only, is also the accessible choice here).
- **`SuggestedRuleCard` modal**: already wired via `createPortal` to
  `document.body` (`:1345-1424`), `role="dialog"` `aria-modal="true"`
  `aria-label="Create Auto-Approval Rule"`. AC3's gap is *visibility/emphasis
  conditioning* on escalation-reason data, not new plumbing — the modal and
  `commandSample` sourcing already exist.
- **`ApprovalAnalyticsPanel`'s closest existing analog for a category
  breakdown** is the coverage-gap badge (`:340-349`, styles
  `gapBadgeHigh/Med/Low/Desc` in `ApprovalAnalyticsPanel.css.ts:364-381`): a
  small pill (`fontSize:12, fontWeight:600, padding:"2px 8px",
  borderRadius:10`, colored border+bg from semantic `vars.color.warning*`/
  `success*` tokens) plus a plain-text description span next to a section
  title, with counts computed from `summary.coverageGapCount`/`total`. The
  existing `windowSelector`/`windowBtn`/`windowBtnActive` group
  (`:104,134-140`, `WINDOW_OPTIONS` = 7/14/30/90 days) is the selectable
  time-window control to reuse for AC4's breakdown, and `UnifiedActivityTable`
  (`:351-361`) plus the `row`/`td`/`tdRight`/`tdBar`+`Bar` idiom used for
  `topPythonImports` (`:320-328`) is the existing table+bar-chart pattern —
  a 5-row reason-category table with the same `Bar` component (value/max
  props, no new charting library) is the natural fit for AC4, not a new
  chart type.

## 1. Comparable UX patterns: how similar tools explain "why review is needed"

Researched: human-in-the-loop AI approval/moderation systems, GitHub required
status checks, AWS IAM policy simulator ("why denied"), plus general
knowledge of PR-bot and agent-orchestration conventions (Dependabot/Renovate,
Discord AutoMod, Stripe Radar, Azure "Check Access").

- **Reason codes over free text, sourced from the actual decision, not a
  generic fallback.** Every mature moderation/approval system surfaces the
  *specific rule or signal* that fired — AWS's policy simulator explicitly
  shows "which policy statement is denying access" via a drill-down link,
  not just "denied by policy"; Stripe Radar shows the named rule that
  flagged a payment; Discord AutoMod attributes an action to the specific
  rule name. This validates the requirement's core design constraint
  (`requirements.md` "Backend plumbing gap"): the reason string must be
  sourced from the real `RuleID`/category, never a generic
  "needs review" placeholder — a generic string is the exact anti-pattern
  these tools avoid.
- **Confidence/threshold framing for the "nothing matched" case.** HITL
  literature frames "no rule matched" / "AI couldn't decide confidently" as
  its own first-class reason category, distinct from "a rule explicitly
  flagged this" — matching this feature's no-match vs. explicit-rule split.
  The practical UX implication: no-match should read as *"nothing told us
  this was safe"* (absence of signal), while explicit-rule should read as
  *"something told us to stop and check"* (presence of signal) — these are
  different mental models and the copy should not conflate them (see §4).
- **Drill-down/link-out from the reason, not inline dump of raw rule
  internals.** IAM's pattern is a short result + a "show details/statement"
  affordance, not printing the full policy JSON next to the decision. This
  is the precedent for AC3: the reason line stays a short plain-text
  sentence in `itemContext`, and the *rule-authoring detail* lives one click
  away in the existing `SuggestedRuleCard` modal — the feature under review
  already matches this pattern structurally; the research confirms it's the
  right shape rather than inlining rule-suggestion UI into the card itself.
- **Explainability text is written for the decision-maker, not the
  system's internal vocabulary** — reviewer guidance literature repeatedly
  stresses that the review UI must show reasoning "reviewers" (non-experts
  in the underlying model/ruleset) can act on in the time budget they have
  (commonly cited: under ~60 seconds per item). This directly informs the
  plain-language copy requirement in §4 below — internal terms like
  `RuleID`, `"shell-expansion-program"`, or `Decision == Escalate` must never
  reach the reviewer.
- **Best-practice placement: short reason line directly adjacent to the
  action, not buried in an expandable/collapsed section.** None of the
  researched systems hide the "why" behind a click for the primary
  reviewer flow — IAM's summary decision+reason is inline in the results
  table row; AWS's "show statement" drill-down is an *additional* detail
  layer on top of an already-visible summary reason, not a replacement for
  one. This rules out an accordion/expand-to-see-reason pattern for the
  primary line (matches AC6's plain-`itemContext`-line requirement) — an
  expandable *detail* could still exist as an enhancement, but the plain
  sentence must be visible without interaction.

Sources:
- [Human-in-the-Loop Escalation Pattern: Triggers, Reviewer Workflow, Feedback Loop](https://appscale.blog/en/blog/microservices-pattern-human-in-the-loop-escalation-2026)
- [Human-in-the-Loop Workflows for AI - Velt](https://velt.dev/blog/designing-human-in-the-loop-workflows-ai-products)
- [What is Human-in-the-Loop (HITL)? - Databricks Blog](https://www.databricks.com/blog/human-in-the-loop)
- [AI Agent Approval Workflows: Human Oversight That Scales](https://waxell.ai/blog/ai-agent-approval-workflows)
- [IAM policy testing with the IAM policy simulator - AWS docs](https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies_testing-policies.html)
- [Troubleshoot access denied error messages - AWS docs](https://docs.aws.amazon.com/IAM/latest/UserGuide/troubleshoot_access-denied.html)
- [IAM Policy Simulator - Medium](https://medium.com/@richardsantiago_38987/iam-policy-simulator-fdf0b24d5864)

## 2. User mental models

- **What a reviewer expects first**: the *what* (what tool/command is being
  requested — already the `commandPreview` `<pre>` block) and the *why*
  (why it escalated) are both needed before a decide-or-not-decide judgment,
  but they answer different questions — "what am I approving" vs. "should I
  trust this enough to approve fast." Given the existing card layout puts
  the badge (priority + attention-reason label) first, then context, the
  reason line should sit **directly below the badge / above the command
  preview** — i.e. in the position `itemContext` already occupies in the
  non-approval branch (`:718-720`), immediately preceding
  `commandPreview`/`cwd` (`:726-732`). This reads as "here's why you're
  looking at this" → "here's exactly what it wants to do" → "here's the
  environment it's in" — a natural escalating-detail order, and it doesn't
  require restructuring the existing DOM order, just adding a sibling
  branch at the top of the existing `pending_approval_id` block.
- **Above or below the request detail**: **above** (i.e., reason precedes
  the command preview), for the reason above — it primes the reviewer's
  read of the command ("this is here because no rule matched" changes how
  a reviewer reads an otherwise-ordinary `rm` command vs. "this is here
  because it touches a newly-registered domain"). Below would force the
  reviewer to read the command cold, form a judgment, then get context that
  might override it — worse ordering for the "fast, confident decision"
  job.
- **Should categories look visually distinct even with plain text/no new
  component?** Yes, and it's achievable with existing primitives: reuse the
  `{label, icon, variant}` map idiom from `getAttentionReasonInfo`
  (`StatusBadge.tsx:15-60`) — a plain leading emoji per category (e.g. ❓
  no-match, 🛑 explicit-rule, 🌐 domain-age) prefixed to the `itemContext`
  text satisfies "distinct without a new component/library." This is
  visual differentiation via **glyph**, not new CSS classes or color — it
  works inside the single existing `itemContext` class, requires no new
  `.css.ts` variants, and (per §1) avoids relying on color as the sole
  differentiator. Color-coding via `itemContext`'s existing `textSecondary`
  token could still shift per-category using inline
  `style={{ color: vars.color.X }}` if the plan phase wants it, but the
  emoji prefix alone already clears the bar the constraint sets (AC6: no new
  styling system) and the accessibility bar (§3: not color-only).

## 3. Accessibility

- **Association with the card**: the reason `<p>` should carry a stable
  `id` (e.g. `escalation-reason-${queueItem.sessionId}`) and the outer
  clickable card wrapper (`itemClickable` div, `:690-703`, already
  `role="button" tabIndex={0}`) should reference it via
  `aria-describedby`. This lets screen-reader users landing on the card's
  button role get the reason announced as supplementary description, not
  just visually adjacent text a screen-reader user could miss if they
  navigate by heading/button instead of linear reading. Precedent in this
  file: `ReviewQueueBadge`'s compact variant already pairs a `title` +
  `aria-label` on the badge span (`:88-95`) — same instinct, applied here
  via `aria-describedby` because the reason is prose attached to the whole
  card's action, not a label on one small element.
- **Button intent change (ghost → secondary) needs a non-visual signal
  too**: intent is a purely visual/CSS change (`Button.css.ts` variants) —
  screen readers don't announce background-color/border differences. The
  `aria-label="Create Rule"` (`:833`) already exists and is intent-neutral,
  which is correct — the *visual* emphasis change doesn't need an ARIA
  counterpart because the button's function and label don't change, only
  its prominence. What *does* need an accessible signal is the
  **conditional presence** of the button — it's currently gated on
  `queueItem.metadata?.["tool_input_command"]` (`:818`) only; if the plan
  adds gating on "is this a no-match escalation," a reviewer who tabs
  through the card should not be surprised by a button that silently
  disappears based on data they can't see. Recommendation: if the button is
  conditionally hidden rather than disabled for non-no-match reasons, ensure
  the reason text itself (which is always visible per AC6) already tells
  the reviewer why — e.g. an explicit-rule reason's copy should make clear
  there's no "create rule" action available for it (a rule already exists),
  so the missing button isn't unexplained.
- **Keyboard-nav impact of a new visible line**: the reason `<p>` is
  non-interactive (no tabindex, no href/button), so it does not add a new
  stop to the tab sequence — it only extends the reading/description
  content of the existing `role="button"` card, which is the cheap,
  correct choice per WAI-ARIA APG guidance (don't make static text
  focusable). The `itemActions` button row (`:783`) is already outside the
  clickable card `div` (sibling, not descendant — note `itemClickable` div
  closes at `:782`, before `itemActions` at `:783`), so adding a reason line
  inside `itemBody`/`itemClickable` doesn't change the tab order among
  Approve/Deny/Create Rule buttons — it only changes what's announced when
  the card itself receives focus.
- **Color-only differentiation risk**: flagged in §2 — if a future iteration
  wants per-category color (not required by AC6), it must not be the sole
  signal (WCAG 1.4.1); pair with the emoji/text approach already planned.

## 4. Error/edge-case UX

- **Transiently missing (poller hasn't enriched yet) vs. permanently
  absent (pre-feature orphaned approval)**: these need different copy
  because they imply different reviewer actions. A transient gap will
  self-resolve (poll again / refresh); a permanent gap never will (approve
  or deny on other evidence). Recommendation, keeping to plain
  `itemContext`-class text (no spinner component, no new dependency):
  - **Transient** (metadata key present but reason not yet computed — if
    the plan's enrichment is async relative to `ReviewItem` creation):
    render nothing extra rather than a misleading placeholder, or a
    neutral line like *"Reason loading…"* only if there's a real
    intermediate state where `pending_approval_id` exists but the reason
    key doesn't yet — confirm in the plan phase whether this window can
    actually occur (per requirements.md, the reason is meant to be set
    synchronously in the same handler path that creates the
    `PendingApproval`, so this state may not exist in practice; don't build
    UI for a state the backend never produces).
  - **Permanently absent** (orphaned approval predating this feature,
    loaded from disk via `loadFromDisk` per requirements.md AC2, with no
    `escalation_reason` metadata key because it was persisted before this
    feature shipped): render a distinct, honest fallback line, e.g.
    *"Reason not recorded — this request predates escalation-reason
    tracking."* — never silently omit the line for pending-approval items
    (that would be an unexplained gap next to *other* cards that do show a
    reason, which reads as broken UI) and never fabricate a category by
    guessing.
- **Plain-language copy per category** (non-technical reviewer, tells them
  what to *do*, not just what happened):
  | Category | Internal signal | Reviewer-facing sentence (draft) |
  |---|---|---|
  | no-match | `RuleID==""`, `Decision==Escalate` | "❓ No auto-approval rule covers this command — review it, then optionally create a rule so similar requests don't need you next time." |
  | explicit-rule | non-empty `RuleID`, `Decision==Escalate` | "🛑 A rule flagged this specifically for review: *[rule name/description]*." |
  | domain-age | `RuleID=="new-domain-check"` | "🌐 This request targets a domain that was registered very recently — a common signal for risky or unfamiliar destinations." |
  | secret-scan | `RuleID=="secret-scan"` | *(analytics-only per AC1 scope note — no queue item exists; not rendered in `ReviewQueuePanel`, only counted in `ApprovalAnalyticsPanel`)* |
  | unclassifiable | `RuleID=="shell-expansion-program"` | "⚙️ This command couldn't be automatically classified (shell expansion/variable substitution) — needs a human read." |

  Each sentence: (a) never mentions `RuleID`, `Decision`, or enum names,
  (b) states the *implication* not just the mechanism, (c) for no-match
  specifically, explicitly foreshadows the Create Rule button (see §5) so
  the two pieces of UI reinforce each other instead of being two unrelated
  facts on the same card.
- **Explicit-rule reason needs the rule's own name/description, not just
  "a rule matched"** — otherwise it's no more informative than the
  no-match case in the reviewer's eyes. This requires the escalation-reason
  value threaded through to carry a human-readable rule label (the plan
  phase should confirm whether existing `ApprovalRule` records have a
  `description`/`name` field to surface here, or whether `RuleID` alone
  must be formatted).

## 5. Jobs-to-be-done

- **Functional** (decide fast): served by placing the reason above the
  command detail (§2) and keeping it a single scannable sentence (§4) —
  reviewer reads reason → reads command → decides, in one visual pass, no
  extra clicks for the common case.
- **Emotional** (confidence they're not missing something dangerous):
  served by category-specific, mechanism-naming copy rather than a generic
  "flagged for review" string — a reviewer who sees "targets a
  newly-registered domain" has a concrete risk model to check against;
  one who sees only "needs review" has to reconstruct that risk model
  themselves or trust the system blindly. This is the strongest argument in
  the requirements for *not* accepting a placeholder/generic string even
  temporarily (AC1's "real, per-path" requirement) — a generic string
  actively undermines the emotional job by feeling like a black box.
- **Social/organizational** (feed the rule-suggestion loop that reduces
  future interruptions): **the connection should be made explicit in copy,
  not left implicit**, specifically for no-match — this is the one category
  where the reviewer's action (approve command) and the system's ask
  (create a rule) are causally linked, and the research in §1 (reason codes
  → drill-down action, not two disconnected UI elements) supports pairing
  them textually. Concretely: the no-match reason sentence should reference
  the Create Rule button in words (draft above: "...so similar requests
  don't need you next time"), so a reviewer who sees the same no-match
  reason repeatedly gets a repeated, reinforcing nudge toward the button
  that's already right there in `itemActions` (`:818-838`) — rather than
  relying on the reviewer independently noticing the pattern across
  multiple queue visits. This also gives product a natural analytics
  signal already covered by AC4: a `no-match` category count that stays
  high over successive time windows is itself evidence the nudge isn't
  working / rules aren't being created, closing the loop back into
  `ApprovalAnalyticsPanel`.
- **Explicit-rule and domain-age have no equivalent loop** — there's
  already a rule (explicit-rule) or the "rule" is an inherent property of
  the request (domain age can't be pre-approved away the same way a
  command pattern can) — so the Create Rule button correctly stays
  no-match-only (per AC3/existing gating), and the copy for those two
  categories should *not* imply a similar "you can prevent this next time"
  framing, since it would be misleading for domain-age (new domains will
  always look new) and redundant for explicit-rule (a rule already exists
  and chose to escalate on purpose — the fix, if any, is editing that rule
  in `ApprovalRulesPanel`, which is explicitly out of scope per
  requirements.md "Out of scope").

## Summary of concrete recommendations for the plan phase

1. Reason line: new `<p className={itemContext}>` branch at the top of the
   `pending_approval_id` block (before `commandPreview`), `id`'d and wired
   via `aria-describedby` from the card's `role="button"` wrapper.
2. Category→copy: a plain `{icon, label, sentence}` lookup keyed by
   escalation category, mirroring `getAttentionReasonInfo`'s shape — no new
   component, colocate near `ReviewQueuePanel.tsx` or a small shared helper.
3. Emoji-prefixed text, not color-only, for category differentiation —
   consistent with existing `ReviewQueueBadge` idiom and WCAG 1.4.1.
4. Create Rule button: keep existing gating, add `intent="secondary"`
   (AC7), no ARIA change needed for the intent swap itself; ensure it's only
   shown for no-match (verify against current `tool_input_command`-only
   gate at `:818` in the plan phase).
5. Missing-reason fallback: distinct, honest copy for "orphaned/predates
   feature" (permanent) vs. treat "loading" as likely a non-existent state
   pending plan-phase confirmation that reason is set synchronously.
6. AC4 breakdown: reuse the `gapBadge*`-style pill + `windowSelector` +
   `Bar`/table idiom already in `ApprovalAnalyticsPanel.tsx`/`.css.ts` — a 5
   (or 4, excluding secret-scan's already-covered analytics bucket if
   redundant) row table, not a new chart type.
