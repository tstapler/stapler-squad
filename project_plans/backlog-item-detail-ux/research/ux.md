# UX Research: backlog-item-detail-ux

**Agent**: 5 (UX Research)
**Date**: 2026-07-21
**Input**: `project_plans/backlog-item-detail-ux/requirements.md`

Scope note: this document covers UX patterns, mental models, accessibility requirements, and
error/edge-case UX for the detail-panel redesign. It does not decide the backend RPC surface for
the read-only session viewer (Open Question in requirements.md, flagged as a Phase 2/technical
research item) — but section 4 states what the *viewer's* UX must accommodate regardless of how
the data gets there.

---

## 0. Existing codebase conventions (read first, reuse — don't reinvent)

Before proposing new patterns, here's what already exists and should be extended rather than
duplicated:

| Need | Existing component/util | Notes |
|---|---|---|
| Status label text | `web-app/src/lib/backlog/status.ts` (`STATUS_LABELS`, `getStatusLabel`) | Canonical vocabulary: idea, refining, ready, queued, in_progress, review, done, archived. Reuse — do not invent new label strings. |
| Status → color class | `BacklogItemDetail.tsx`'s `STATUS_CLASS` map and `BacklogItemBadge.tsx`'s identical copy | **Currently duplicated in two files.** Consolidation target: extract to a shared `getStatusClass`/`.css.ts` variant map (own file, e.g. `web-app/src/lib/backlog/statusStyles.css.ts`), consumed by both the badge and the new detail-view summary, per "no new `.module.css`, reuse tokens" rule. |
| Board/list card status chip | `BacklogItemBadge.tsx` | Small chip + AC-done fraction + truncated title. This is the pattern to extend for card/detail consistency (success metric #3 in requirements.md) — don't invent a second chip visual language. |
| "Stuck" reason chips (icon + text label, never color-only) | `web-app/src/components/backlog-stuck/stuckReason.ts` + `stuckReason.css.ts` | This is the **direct precedent** for a derived-state chip system: `StuckReason` enum → label/icon/class via three parallel `Record<StuckReason, T>` maps (compile-error-if-a-case-is-missed pattern), `formatStuckDuration`, `formatAgo`, `formatSinceUTC` for time display. The new "blocked/waiting on X" indicator (section 5) should follow this exact shape, not a new one. |
| Session-attention badges (icon + text + tooltip, `role="status"`) | `web-app/src/components/sessions/StatusBadge.tsx` | Shows the `role="status"` + icon + text + `title` tooltip pattern already used for live session state (Approval Pending, Input Required, Error, Idle, etc.) — reuse this shape for any per-session status chip inside the redesigned Sessions section. |
| Existing collapse/expand precedent | `web-app/src/components/sessions/RecentFilesSection.tsx` | `<button aria-expanded={!collapsed}>` + chevron glyph (▸/▾) + `localStorage` persistence keyed per-section (`"filesTab.recentCollapsed"`). **This is the only existing accordion-adjacent implementation in the codebase** — no generic `<Accordion>`/`<Disclosure>` component exists yet. The redesign should either promote this into a shared primitive (`components/ui/Collapsible.tsx` + `.css.ts`) consumed by every panel section, or accept per-section duplication if a shared primitive is out of scope — but should not invent a third expand/collapse idiom (e.g. avoid mixing `<details>/<summary>` with button+aria-expanded elsewhere in the same view). |
| Streaming/live section update convention | `BacklogItemDetail.tsx` Planning section (`aria-live="polite"` on `styles.section`, line ~787) | Establishes that live-updating sections already use `aria-live="polite"` at the section-container level, not per-row. Follow this for any newly-live-updating summary. |
| Read-only, currently inert session rows | `BacklogItemDetail.tsx` Sessions section, ~line 1333 | Triage sessions (`role === "triage"`) and synthetic headless/blocked sessions (`sessionId.startsWith("headless-")` / `"review-blocked-"`) render as a plain `<span>` instead of the `<a href="/?session=...">` used for real sessions — this is the literal "inert unclickable text" named in the task. The fix target is a third rendering branch: real/interactive sessions → link into the terminal view (unchanged); headless/triage/synthetic sessions → open the new read-only viewer (not the terminal view, not a dead span). |
| Portal/z-index rules for any new overlay (e.g. viewer opened as a modal/sheet) | `.claude/rules/css-architecture.md` | If the read-only viewer is presented as an overlay rather than inline/in-panel, it **must** use `createPortal(..., document.body)` and a named `zIndex` slot — never bare `position: fixed`. |

**Design implication**: the redesign is not greenfield — it's a consolidation exercise. The two
existing parallel systems (`BacklogItemBadge`'s plain status chip, `stuckReason.ts`'s richer
derived-state chip) should converge into one status vocabulary consumed by card, detail-panel
summary, and any future surface, per requirements.md's success metric "reduced duplication."

---

## 1. Comparable UX patterns: status/lifecycle at a glance + progressive disclosure

### CI/CD pipeline UIs (GitHub Actions, GitLab)

- **GitHub Actions run view**: the run page leads with a single-line status glyph + label
  (queued/in-progress/success/failure) at the top, then a job graph (parallel/sequential stages
  as nodes), then step-level logs collapsed by default with the *currently running* or
  *first-failed* step auto-expanded. The key move: **the system chooses what to expand for you**
  based on state (running/failed), not a static "everything collapsed" or "everything expanded"
  default. Streaming logs use a `role="log"`-equivalent scrolling console with backscroll (last
  ~1000 lines fetched immediately on open, not built up from empty) — directly relevant to the
  new read-only session viewer (section 4).
- **GitLab pipeline mini-graph**: stages render as a compact horizontal chain of stage-icons
  (pass/fail/running/**blocked**/manual). Blocked is a first-class, distinctly-colored state
  separate from "running" — GitLab's own postmortems note that conflating "blocked" with
  "pending/running" is a recurring source of user confusion ("tests passed = I assumed the
  pipeline succeeded" when it was actually blocked waiting on a manual gate). Applicable directly:
  this app's "stuck" derived state is exactly this "blocked-but-looks-like-it-might-still-be-
  progressing" trap — the lifecycle summary must visually distinguish *actively working* from
  *blocked/waiting on you* from *blocked/waiting on something else*, not just show a spinner for
  all three.
- **Real-time updates without losing place**: GitLab's mini-graph historically updated only on
  page refresh/poll, which they identified as a UX gap and fixed via live push updates — the
  fix was explicitly to update *in place* (same DOM node re-renders new state) rather than
  re-render the whole graph, which is the same constraint this project has for live streaming
  into an already-open detail panel (see section 4's "preserve expand state" requirement).

### Issue trackers (Linear, GitHub Issues sidebar)

- Common shape: a persistent **compact identity strip** (status pill + assignee + priority +
  labels) stays visible regardless of scroll position or which section is expanded — it's the
  "spine" the rest of the page hangs off. Everything else (activity history, sub-issues, linked
  PRs, comments) is either below the fold or in a collapsible sidebar module. The status pill
  itself is always a single word + color, never a paragraph — detail lives one click away, never
  in the pill itself.
- GitHub Issues sidebar groups metadata into small, independently-collapsible modules
  (Assignees, Labels, Projects, Milestone, Linked PRs) rather than one giant form — each module
  answers exactly one question and can be ignored if irrelevant to the current task. This maps
  directly onto turning this app's 12 flat sections into named, independently-collapsible modules
  rather than a single monolithic expand/collapse toggle for the whole panel.

### Job/task monitoring dashboards (general pattern, confirmed via NN/g + GitLab sources)

- The three-layer model that recurs across dashboard-style UIs: **(1) at-a-glance KPI/status,
  (2) one-click detail, (3) deliberate-intent configuration/rare actions.** This maps cleanly
  onto: (1) lifecycle summary always visible, (2) the 12 sections collapsed-but-one-click-away,
  (3) destructive/rare actions (archive, override-to-done, delete session) tucked further —
  already partially true here (Actions section exists) but worth auditing whether truly rare
  actions are visually weighted the same as common ones today.
- Documented caveat (NN/g): progressive disclosure is the wrong call "when users are doing
  operational monitoring where information density is the whole point." This is a real tension
  for this feature — Tyler's job here *is* operational monitoring. The resolution used by all
  three comparables above is not to hide information but to **sequence** it: the always-visible
  summary carries the operationally-critical signal (state + blocker), and progressive disclosure
  only applies to the *justification*/detail behind that signal (full timeline, full session
  list, full plan text) — never to the signal itself. Don't collapse the thing that answers "is
  it stuck," only the evidence for the answer.

**Why these work**: all three domains solve the same problem — a human periodically checks on an
asynchronous, possibly-long-running process they are not actively driving — by putting a single
unambiguous state indicator at a fixed location, distinguishing "actively progressing" from
"blocked" as different visual states (not different opacities of the same spinner), and letting
the detail underneath be opt-in without ever requiring opt-in for the headline state.

---

## 2. User mental models

**Job to be done**: functional — "verify this autonomous item isn't stuck and doesn't need me."
Emotional — "trust that if something silently broke, I'd know without having to comb through
JSON-shaped text." (Solo tool: no social JTBD.)

Given that framing, the check-in scan order a developer performs (confirmed by the CI/issue-
tracker precedents above, which are the same "periodic check on background async work" pattern)
is:

1. **Status** (first, always, at fixed position): What state is it in right now — one of the
   real backend statuses (idea/refining/ready/queued/in_progress/review/done/archived) — the
   *ground truth* the UI must never contradict, per requirements.md's explicit constraint against
   inventing a UI-only status model.
2. **Blocker / next action** (second, immediately adjacent to status, no click required): Is
   there something *actively running* right now (in which case: leave it alone, nothing to do),
   or is it *waiting on something* (in which case: what, and can I unblock it)? This is where the
   derived "stuck" state and existing `stuckReason.ts` reasons plug in — a `queued`/`in_progress`
   item can still be "waiting on the WIP cap" or "waiting on rework," which is functionally a
   blocker even though the raw status field says "in_progress." The summary must show the
   *derived* blocker even when it disagrees with the naive read of the raw status.
3. **Drill-down detail** (third, opt-in, click/tap to expand): full description, AC criteria,
   plan artifacts, full session list and their transcripts, full status-event timeline, full
   progress history. This is "I want to understand *why*," not "I want to know *what*" — the
   distinction that justifies collapsing it by default without harming the functional JTBD.

A secondary mental-model point specific to this app: because sessions are *autonomous agents*,
"is it stuck" is not equivalent to "is the status field static." An item can sit in `review` for
a legitimate long time (agent actively reviewing) or be silently dead in `review` (session
crashed, nothing polling it). The emotional JTBD ("trust nothing is silently broken") is *only*
satisfiable if the at-a-glance summary encodes recency/liveness (e.g. "last activity 2m ago" vs
"last activity 3d ago," reusing `formatAgo`/`formatStuckDuration` from `stuckReason.ts`), not just
the categorical status. A status-only summary (no liveness signal) would look identical for
"healthy and slow" and "dead," which fails the actual job to be done.

---

## 3. Accessibility (WCAG / ARIA)

### Collapsible/accordion sections

Per WAI-ARIA Authoring Practices (APG) accordion pattern, confirmed via search:

- Each section header must be a real `<button>` (not a `<div onClick>`) so it's natively
  keyboard-operable (Enter/Space) and reachable via Tab — matches the existing
  `RecentFilesSection.tsx` precedent exactly.
- `aria-expanded="true"|"false"` on the header button is **required** — without it, a screen
  reader has no way to convey open/closed state. Set it dynamically, not just visually
  (chevron-only state is a WCAG 1.3.1 / 4.1.2 failure — state must be programmatically
  determinable, not conveyed by icon shape alone).
- If the header controls a distinct panel `id`, add `aria-controls="<panel-id>"` on the button
  pointing to the content region — not currently present in `RecentFilesSection.tsx`, worth
  adding when this becomes a shared primitive.
- This app's accordion does **not** need to enforce "only one open at a time" (that's an
  implementation choice, not an APG requirement) — multiple independently-open sections is fine
  and arguably better here, since the JTBD is scanning multiple signals, not a strict wizard flow.
- Up/Down arrow key navigation between headers is explicitly *optional* per APG — not required
  for compliance, nice-to-have if a shared `Collapsible` primitive is built.
- Tab order: all interactive elements inside an expanded panel remain in the normal page Tab
  sequence — nothing extra to implement here beyond standard DOM order, but it does mean
  **collapsing a section should remove its interactive contents from the tab sequence** (i.e.
  actually unmount/hide with `hidden` or conditional render, not just `display:none` a container
  while leaving inputs focusable — check whichever collapse mechanism is chosen for this).
- Focus management: expanding a section should not steal focus away from the header button that
  was just activated (focus stays on the header, which is standard toggle-button behavior — no
  special handling needed, but explicitly *do not* auto-focus into the newly-revealed content,
  which would be surprising for a scanning/monitoring use case).

### Read-only log/output viewer (section 4's new triage/headless-review viewer)

- MDN/ARIA confirm a dedicated `role="log"` exists for exactly this shape: "sequentially ordered
  content where new information is added to the end" — chat logs, error logs, activity feeds.
  `role="log"` has an **implicit `aria-live="polite"`**, so screen-reader users are told about new
  output without needing an explicit `aria-live` attribute layered on top (don't double up
  `role="log" aria-live="assertive"` unless a genuinely urgent event needs to interrupt — normal
  streaming output should stay polite).
- Default `aria-atomic="false"` is correct here — only the newly-appended text should be
  announced on update, not the entire scrollback re-read on every poll tick. This matches how
  `SessionMonitor.tsx`'s polling model already works (fetch + replace) — the new viewer's polling
  should append/diff rather than force a full re-announce.
- This app already has one precedent for `role="status"` (`StatusBadge.tsx`, session-level
  attention state) — that's a different ARIA role from `role="log"` and both are legitimate:
  `role="status"` for a single current-state summary line, `role="log"` for the actual scrolling
  transcript. Don't conflate the two — the new viewer likely wants both: a `role="status"` line
  ("Session ended — read-only" / "Live") plus a `role="log"` region for the transcript body.
  Consistent with the existing Planning section's `aria-live="polite"` convention at the
  section-container level (section 0 above).
- Because this viewer is explicitly **read-only** (no `handleSend`/`writeToSession`, no
  `QUICK_ACTIONS` input row from `SessionMonitor.tsx`), it should be built as a stripped variant
  or shared sub-component of `SessionMonitor.tsx` minus the input form — not a full reimplementation of terminal rendering. Visually and semantically it should read like a `<pre>`
  (monospace, preserved whitespace) inside the `role="log"` container, matching this app's
  existing terminal-output styling conventions in `TerminalOutput.tsx`.
- Keyboard/focus: a log region needs no special keyboard handling beyond being scrollable and,
  if it contains no interactive children, does not need to be in the Tab order itself (only its
  scroll container needs `tabindex="0"` if it's the only way to scroll it via keyboard on a
  non-focusable container — verify with the eventual implementation whether native scroll
  affordance suffices).

### General

- Per `stuckReason.ts`'s own established convention: **never color-only** for any status/blocker
  indicator — icon or text label must always accompany color, which the existing
  `STUCK_REASON_ICONS`/`STUCK_REASON_LABELS` pairing already enforces and any new lifecycle
  summary chip must match (WCAG 1.4.1, Use of Color).
- Mobile: collapsible headers and the read-only viewer's scroll region both need ≥44×44px touch
  targets per the existing project-wide mobile+desktop UX requirement (`feedback_mobile_desktop_ux`
  memory) — the current `RecentFilesSection.tsx` chevron+button pattern already satisfies this if
  reused as-is (full-width button, not just the chevron glyph, is clickable).

---

## 4. Error / edge-case UX

### Zero sessions yet

Today the Sessions section is conditionally rendered only when sessions exist elsewhere in the
component (see the `item.linkedSessions.length` heading), but the redesign should show an
explicit **empty state**, not omit the section entirely — omission reads as "did this load
correctly?" in a monitoring tool. Recommended: keep the section visible (collapsed is fine) with
a one-line "No sessions yet — triage hasn't started" (or status-appropriate variant, e.g. for
`idea`/`refining` vs `ready`/`queued`), reusing the same `getStatusLabel`-driven phrasing already
used elsewhere, so it reads as *expected* rather than *broken*. This directly serves the
emotional JTBD (trust nothing is silently wrong) — an empty section with no explanation is
indistinguishable from a fetch failure to a scanning user.

### Synthetic/headless review session with no real output

Per requirements.md's own flagged Rabbit Hole: `headless-`/`review-blocked-` prefixed session IDs
may not correspond to a session that ever actually started. The viewer must handle three distinct
sub-states, not one:

1. **Real output exists** (session ran, headless) → open the read-only viewer with the transcript,
   as designed.
2. **Session record exists but never produced real output** (e.g. blocked before starting) → the
   viewer should say so explicitly ("This review was blocked before a session could start" or
   similar, sourced from whatever the backend records as the block reason — likely overlapping
   with `StuckReason.REVIEW_BLOCKED`-shaped data) rather than showing an empty scrollback that
   looks like a loading/broken state.
3. **No underlying session record at all, purely synthetic placeholder ID** → do not present a
   "view session" affordance at all for this row; instead surface whatever diagnostic *is*
   available inline (e.g. the reason text already computed for `stuckReason.ts`'s
   `ORPHANED_TRIAGE`/similar reasons) so the row still answers "what happened here" without
   implying there's a transcript to open.

This is a case where the UI must **not** uniformly treat all three as "clickable → opens viewer"
— doing so would put an affordance in front of the user that leads to a dead/empty screen,
which is worse for trust than an inert but honest label. The correct fix for the "inert
unclickable text" complaint in requirements.md is *not* "make everything clickable" — it's "make
the clickability match what's actually inspectable, and give the non-inspectable ones an inline
reason instead of dead text." Confirming which of these three states is which is explicitly
called out as needing Phase 2 backend research (requirements.md Rabbit Holes) — this UX doc
specifies the three presentations that research needs to be able to select between per row, not
which one applies to which existing prefix.

### Very long-running item with dozens of progress-history entries

- Follow the GitHub Actions precedent (backscroll fetched on-demand, not everything rendered
  greedily): the Progress History and Workflow status-history timeline should each cap the
  default-rendered count (e.g. most recent N entries) with a "Show earlier" affordance, rather
  than rendering an unbounded list inside an already-collapsed section — collapsing the *section*
  doesn't help perf/scroll-length once it's expanded if the full history still renders in one
  shot.
- Because both Progress History and Workflow timeline currently show overlapping information
  (per requirements.md's flagged duplication concern), the long-running-item case is a good forcing
  function to resolve that overlap: a single merged, capped, reverse-chronological timeline is
  likely correct here rather than two separately-capped lists showing similar events twice.

### Collapse/expand state vs. live data updates (streaming UI)

This is the sharpest UX risk in the whole redesign and needs an explicit rule, not an implicit
default:

- **User-initiated expand/collapse choices must persist across live data updates within the same
  viewing session.** If Tyler expands "Sessions" to watch a running session, a poll tick that
  refreshes `item.linkedSessions` must not silently re-collapse it — this is precisely the failure
  mode GitLab identified and fixed for their pipeline mini-graph (updating in place vs.
  re-rendering the whole graph). Concretely: expand/collapse state must live in component state
  keyed by section identity (or `localStorage`, per the `RecentFilesSection.tsx` precedent) —
  never derived from or reset by the incoming data payload.
- Corollary: a poll/stream update that changes *which* sections are relevant (e.g. item moves
  from `review` to `done`, so the "Reviewing" section's live content disappears) is a legitimate
  reason for a section to change appearance (e.g. show a "session ended" state), but should still
  not forcibly collapse a section the user explicitly opened — show the end-state content inside
  the section they left open, don't yank it shut on them.
- The one case where auto-expand-on-update *is* appropriate (matching the GitHub Actions
  "auto-expand the running/failed step" pattern): a **first-time** transition into a state that
  needs attention (e.g. item newly enters a "stuck" derived state, or a session newly needs
  approval) should be allowed to auto-expand the relevant section *once*, on the state
  transition — but if the user then manually collapses it again, that manual choice should win
  over any subsequent re-render, i.e. auto-expand is a one-shot nudge, not a sticky override.
- This also applies to the read-only log viewer itself while open: if it's polling
  (`SessionMonitor.tsx`'s `POLL_INTERVAL_MS` pattern), new content must append below the
  current scroll position without forcing scroll-to-bottom if the user has scrolled up to read
  earlier output — `SessionMonitor.tsx`'s current unconditional
  `outputRef.current.scrollTop = outputRef.current.scrollHeight` auto-scroll-to-bottom on every
  update (line ~72) is the wrong default for a review-focused read-only viewer if the user has
  scrolled away from the bottom; the "jump to latest" affordance should be an explicit control
  (a small "scroll to bottom" pill, common in chat/log UIs) rather than forced on every poll.

---

## 5. Status/lifecycle visual language recommendation

### Backend statuses (ground truth, must not be reinvented)

`idea → refining → ready → queued → in_progress → review → done` with `archived` as a terminal
side-state reachable from most points, per `STATUS_LABELS` in `web-app/src/lib/backlog/status.ts`
and the WIP-cap/queued-status work already shipped (`44f77e0b`, per repo git log).

### Recommended pattern: compact stage tracker + separate blocker indicator

A single horizontal **stage tracker** (breadcrumb/stepper style, e.g.
`Idea → Ready → In Progress → Review → Done`, with `Refining`/`Queued` shown as sub-labels or a
brief detour rather than doubling the main track's width) as the *always-visible* header element,
plus a **separate, visually distinct "blocked/waiting on" chip** next to it — not merged into the
same visual element. Reasons to keep them separate, grounded in the CI-pipeline research above:

1. GitLab's own postmortem shows conflating "current stage" with "blocked" in one indicator
   causes users to misread blocked-and-stalled as still-progressing. Two indicators avoid that:
   the stepper always shows the *real* backend status (never lies), and the blocker chip is the
   *derived* layer (reusing `stuckReason.ts`'s existing `StuckReason` enum/labels/icons) that can
   be present or absent independent of which stage the tracker shows.
2. It reconciles cleanly with backend truth (requirements.md's explicit constraint against
   inventing a UI-only status model) — the tracker's active step is always exactly
   `item.status`, no interpretation. The blocker chip is where all the *derived* logic
   (`useStuckBacklogItems.ts`, `isPrStatusUnknown`, staleness thresholds) lives, already built and
   tested — this recommendation is "surface it here too," not "build new derivation logic."
3. It matches the "distinguish actively-working from blocked" requirement from section 1 without
   needing a third pseudo-status: `in_progress` + no blocker chip = "working, leave it alone";
   `in_progress` + a `StuckReason.STALE_WORK` blocker chip = "looks like it stopped."

Concrete shape:

```
[Idea] → [Ready] → [● In Progress] → [Review] → [Done]      🟠 Stuck 4h — Stale work session
         ↑ current step highlighted                          ↑ stuckReason chip (icon+label+duration,
                                                                 only rendered when useStuckBacklogItems
                                                                 flags this item — otherwise omitted,
                                                                 not shown as a neutral/green "OK" state
                                                                 to avoid a fourth always-on element)
```

- `queued` renders as a modifier/badge on the `In Progress` step (e.g. "In Progress · Queued —
  waiting for WIP slot") rather than its own stepper node, since it's a sub-state of the working
  phase from the user's mental-model perspective (item hasn't reached review yet) even though
  it's a distinct backend status — this keeps the stepper's node count stable and matches how
  "queued" was introduced as a WIP-cap mechanism rather than a new lifecycle phase (per
  `44f77e0b`'s "configurable WIP cap + queued status" framing).
- `archived` is not a stepper node — it's better shown as an overlay/ribbon state on top of
  whatever step the item was at when archived ("Archived from Review"), since archiving can
  happen from multiple points and forcing it into the linear track would misrepresent it as a
  terminal "success" state alongside `done`.
- Liveness/recency (section 2's "healthy and slow" vs "silently dead" distinction) belongs as a
  small secondary line under the tracker — "Last activity 2m ago" via the existing `formatAgo`
  helper — not baked into the stepper or the blocker chip, since it applies even when there is no
  blocker.
- For the board/list card (requirements.md's card-consistency success metric), this is a
  compressed version of the same two-part model: keep `BacklogItemBadge`'s existing single status
  chip (full stepper doesn't fit a card), but add the same derived blocker chip
  (icon+label, from `stuckReason.ts`) next to it when present — this is the minimum change that
  satisfies "same status vocabulary, same 'what's it waiting on' signal" without a card layout
  redesign, per the explicit out-of-scope note in requirements.md.

---

## Summary of concrete recommendations for Phase 3 (planning)

1. Consolidate the two duplicated `STATUS_CLASS` maps (`BacklogItemDetail.tsx`,
   `BacklogItemBadge.tsx`) into one shared source, following `stuckReason.ts`'s
   `Record<Enum, T>`-per-concern shape.
2. Promote `RecentFilesSection.tsx`'s button+`aria-expanded`+chevron+localStorage pattern into a
   shared `Collapsible` primitive; add `aria-controls`; ensure collapsed content is actually
   removed from the tab sequence.
3. Header/summary block: stage tracker (backend status, ground truth) + separate blocker chip
   (reusing `stuckReason.ts` verbatim) + a liveness/recency line — always visible, never
   collapsed.
4. New read-only session viewer: strip `SessionMonitor.tsx` down to output-only (no
   `writeToSession`/`QUICK_ACTIONS`), wrap in `role="log"` (implicit `aria-live="polite"`) +
   `role="status"` line for viewer state, replace forced scroll-to-bottom with an explicit
   "jump to latest" control, and design three distinct presentations for
   has-output / blocked-before-start / no-underlying-session (don't make every row clickable).
5. Cap Progress History / Workflow timeline render length by default; consider merging them per
   the duplication note in requirements.md.
6. Expand/collapse state must be independent of poll/stream data — keyed by section identity in
   local/component state, never reset by an incoming payload; allow one-shot auto-expand only on
   a genuine state transition into a needs-attention condition.
7. Card-level consistency: add the same blocker chip next to `BacklogItemBadge`'s existing status
   chip; don't redesign the card layout.

## Sources consulted

- [WAI-ARIA Authoring Practices — Accordion pattern](https://wai-aria-practices.netlify.app/aria-practices/examples/accordion/accordion)
- [Accessible Accordion — Aditus](https://www.aditus.io/patterns/accordion/)
- [MDN — ARIA log role](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Roles/log_role)
- [What Are ARIA Live Regions? — BOIA](https://www.boia.org/blog/what-are-aria-live-regions)
- [GitHub Actions: UI Improvements (backscroll, streaming logs) — GitHub Changelog](https://github.blog/changelog/2024-04-30-github-actions-ui-improvements/)
- [Progressive Disclosure — UXPin](https://www.uxpin.com/studio/blog/what-is-progressive-disclosure/)
- [Progressive Disclosure (video) — NN/G](https://www.nngroup.com/videos/progressive-disclosure/)
- [Progressive disclosure — Wikipedia](https://en.wikipedia.org/wiki/Progressive_disclosure)
- [GitLab Pipeline Mini Graph — GitLab Handbook](https://handbook.gitlab.com/handbook/engineering/architecture/design-documents/pipeline_mini_graph/)
- [GitLab — Add real-time pipeline stage status updates via GraphQL subscription](https://gitlab.com/gitlab-org/gitlab/-/issues/591338)
- [GitLab CI/CD — "manual" job status always "blocked" (user confusion precedent)](https://forum.gitlab.com/t/specifying-a-pipeline-step-as-manual-causes-its-status-to-always-be-blocked/58333)
