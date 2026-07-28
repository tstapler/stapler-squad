# Research: Pitfalls & Risks — `backlog-item-detail-ux`

Agent 4 (Pitfalls). Grounded in the actual `BacklogItemDetail.tsx` (1577 lines),
its sibling components, and the Go backend behind the "Sessions" section.

---

## 1. Refactoring a 1500+ line "kitchen sink" component into progressive disclosure

**State-reset-on-navigation is already a latent bug — don't make it worse.**
`BacklogItemDetail` is not remounted when `itemId` changes — there is no
`key={itemId}` at the call site (`BacklogItemPanel.tsx`). The component keys its
data refetch off `itemId` (`load` depends on it, `BacklogItemDetail.tsx:218`), but
none of its ~15 other `useState` calls (`showManualReview`, `manualReviewSummary`,
`editMode`, `showChangesModal`, `showFileBrowser`, `editingNotes`, …) are reset on
an `itemId` change. Today this is a narrow latent bug (e.g. the manual-review form
staying open if you click a different item while it's up). **Every new collapse/
expand `useState` you add for progressive disclosure inherits this same footgun**
— unless you either (a) explicitly reset all local UI state in a `useEffect` keyed
on `itemId`, or (b) key section-expanded-state by `itemId` (e.g.
`Record<itemId, Set<sectionKey>>` or reset via a ref-compare pattern), switching
items will silently carry over "which sections are expanded" from the previously
viewed item. Fix this class of bug once, for all local UI state, not just the new
fields — don't just patch it for the new accordion state and leave the old fields
broken.

**The manual-review verdict form (`BacklogItemDetail.tsx:1152`) is exactly the
kind of interactive element progressive disclosure regresses if handled naively.**
It's gated by two independent conditions — `showManualReview` (local toggle) AND
`item.status === "review"` (server truth) — and its visibility currently lives
inside the same flat JSX tree as everything else. If it moves under a collapsible
"Actions" section:
- Collapsing the Actions section while the form is open must not silently discard
  `manualReviewSummary`/`manualReviewOutcome` — either force the section open
  when `showManualReview` is true, or preserve the draft text across collapse.
- The `data-testid="manual-review-form"` / `"manual-review-outcome"` /
  `"manual-review-summary"` / `"manual-review-submit"` locators are used by the
  e2e conventions (`.claude/rules/e2e-test-conventions.md` mandates
  `data-testid`-only locators) — any DOM restructuring (wrapping in a new
  `<details>`/accordion component) must preserve these exact testids or the e2e
  suite silently stops finding the form rather than failing loudly, since
  Playwright's `getByTestId` just times out.
- Regression tests already exist for this file
  (`BacklogItemDetail.test.tsx`, `.regression.test.tsx`, `.shipPR.test.tsx`,
  `.markdown.test.tsx`) but there is **no e2e spec** that exercises the manual
  review form end-to-end (`grep` across `tests/e2e/*.spec.ts` found none) —
  RTL/Jest tests can pass while the real collapsed-by-default DOM hides the form
  behind a click a real user (or Playwright, if a spec existed) would need to
  perform. Treat "does the form still render when Actions is expanded" as an
  explicit new test to add, not an assumption the existing suite covers it.

**Actions panel regression risk is broader than the review form.** The Actions
block (`~1075-1230+`) branches on `item.status`, `actionLoading`,
`acAllComplete`, and per-button `disabled`/`aria-busy` state tied to
`actionLoading === "<action>"` string matching. If Actions becomes a collapsible
section, every action button's `data-testid` (`backlog-action-ship-pr`,
`backlog-action-override-done`, `backlog-action-re-review`,
`backlog-action-manual-review`, `backlog-action-restart-session`, etc.) and their
conditional-render logic must be preserved verbatim inside whatever wrapper is
introduced — a common refactor mistake is to "helpfully" extract the button list
into a new subcomponent and accidentally change prop plumbing for `actionLoading`
(a single shared string, not per-button state) in a way that breaks the
mutual-exclusion invariant (only one action `pending` at a time).

**Prop drilling risk if sections are extracted into subcomponents.** The
component currently closes over ~15 pieces of local state and several hook
results (`useBacklogService`, `useSessionService`, `useVcsStatus`,
`useBacklogItemShipStatus`, `useAnalytics`, `useNotifications`) in one function
body. Splitting into `<StatusSummary>`, `<ActionsSection>`, `<SessionsSection>`,
etc. will require either (a) drilling many props through each, or (b) a shared
context/reducer. Given this codebase's documented allergy to speculative
abstraction (`.claude/rules/interface-pollution-checklist.md`), prefer a single
local state object / `useReducer` colocated in `BacklogItemDetail.tsx` with
child components receiving narrow, explicit props — not a new context provider
for a single-consumer-tree feature (that would itself be an "unjustified
abstraction" by the same checklist's smell #1).

---

## 2. Progressive disclosure / accordion UX pitfalls

- **Collapsed-by-default hiding "why is this stuck" info.** The whole point of
  this project (per `requirements.md`) is that a user currently can't tell why an
  item is stuck without reading everything. If the redesign defaults to
  collapsing sections *without* first promoting the derived "what's this waiting
  on" signal into an always-visible summary, the redesign reproduces the exact
  problem it's meant to fix — just with an extra click required. The at-a-glance
  lifecycle summary (in scope) must ship *before or with* the collapse behavior,
  not as a stretch goal, or the interim state is a regression.
- **Collapse fatigue.** If sections default to collapsed indiscriminately, the
  user stops trusting the panel and starts expanding everything every time out of
  habit (defeating the purpose) or, worse, stops expanding at all and misses a
  section that only matters occasionally (e.g. `GateVerdictBox`, plan artifacts).
  Mitigate by having **state-dependent** default-expanded rules rather than a
  single global default — e.g. auto-expand the section relevant to the item's
  *current* status (Actions expanded while `status === "review"`, Sessions
  expanded while a session is actively running) and collapse sections that are
  inert for the current state (e.g. Plan Artifacts once already approved).
  `StuckItem.tsx`/`UnfinishedItem.tsx` (see below) already do something similar:
  `wasExpandedRef` + `isExpanded` prop pattern, not a single hardcoded default.
- **Inconsistent default-expanded state across sessions/reloads.** Decide
  explicitly whether "expanded" state should persist per-item across visits
  (`localStorage`, keyed by item ID or section ID — the codebase already has many
  `localStorage`-backed UI-state precedents:
  `BacklogItemPanel.tsx`, `TriageReviewPanel.tsx`, `SessionList.tsx`,
  `useHandedness.ts`, etc.) or reset every time the panel opens. Silently doing
  neither consistently (e.g. persisting Actions' state but not Sessions') creates
  an inconsistent, confusing experience — pick one policy and apply it uniformly
  across all collapsible sections, and document it in the plan.
- **Accessibility regressions specific to accordions.** `aria-expanded`,
  keyboard toggling (Enter/Space on the header), and focus management (does focus
  move into the revealed content, or stay put?) are easy to half-implement. This
  repo's CI runs Axe Core on PRs touching `web-app/src/` and **blocks on WCAG AA
  violations** (`CLAUDE.md`'s "E2E Tests" section) — an accordion header that's a
  `<div onClick>` instead of a real `<button aria-expanded>` will fail that gate,
  not just look sloppy.
- **Reference precedent already in-repo**: `StuckItem.tsx` +
  `StuckItemsSection.tsx` (backlog-stuck-item-visibility project, already shipped)
  implement exactly this pattern — `isExpanded: boolean` prop lifted to a parent
  `useState<Set<string>>` keyed by a stable string key, `aria-expanded`, a
  `wasExpandedRef` to detect collapse transitions, Escape-to-collapse, and a
  `cardExpanded` CSS variant class (not inline style, not height animation via
  custom properties) — see `web-app/src/components/backlog-stuck/StuckItem.tsx`
  and `.css.ts`. `UnfinishedItem.tsx` is a near-verbatim copy of the same shape.
  **Reuse this pattern rather than inventing a new accordion primitive** — two
  independent implementations of the same interaction already exist in this
  codebase; a third slightly-different one increases maintenance surface for no
  benefit.

---

## 3. Risks in the new read-only viewer for triage/headless-review sessions

**The two "inert `<span>`" cases at `BacklogItemDetail.tsx:1333` are not the same
kind of thing, and treating them identically will produce a broken or misleading
viewer for at least one of them:**

1. **`role === "triage"` sessions** (real triage, non-headless) — these likely
   have live/completed tmux sessions with real scrollback, fetchable the way
   work/review sessions already are.

2. **IDs prefixed `headless-triage-` / `headless-re-review-`**
   (`server/services/backlog_service_triage.go:204-205`,
   `session/backlog_lifecycle.go:1744`) — these are **not tmux sessions**. Headless
   triage/re-review runs as an in-process `claude -p` subprocess
   (`session/headless/runner.go`, `session/headless/caller.go`) whose output is
   streamed as in-memory `StreamChunk`s and only the **final parsed JSON result**
   is persisted, onto `ItemSession.triage_result`
   (`session/ent/schema/item_session.go`) — there is no raw transcript, no
   tmux scrollback, nothing in `session/scrollback/storage.go` for these IDs.
   **A "read-only session output viewer" that assumes scrollback exists for these
   IDs will find nothing to show.** The correct behavior (as
   `requirements.md`'s own Rabbit Holes section already suspected) is to render a
   *different* diagnostic — the structured `triage_result` JSON
   (title/summary/suggestions/tasks/AC) — not raw session output. Confirm this in
   the plan phase rather than building a generic "fetch scrollback by session ID"
   viewer that silently 404s or shows nothing useful for headless IDs.

3. **IDs prefixed `review-blocked-`** (`session/review_gate.go:229`) are even
   more distinct: these are synthetic `ItemSession` rows created **only** when
   `RunPreGateSecurityCheck` blocks a review because **secrets were detected in
   the diff** (`session/review_gate.go:220-236`). There is no session — headless
   or tmux — behind this ID at all; it's a pure DB marker recording a FAIL
   verdict with a summary string like `"Review blocked by security check: %v..."`.
   Two concrete risks:
   - **UX risk**: if the new viewer treats `review-blocked-*` like any other
     "session" and tries to fetch output, it will show an empty/broken state for
     exactly the case where the user most needs a clear diagnostic ("your diff
     had a detected secret — here's what triggered it").
   - **Security risk (see below)**: the `secErr` message embedded in the stored
     summary is a description of *what the security scanner matched* — depending
     on the scanner's error format, that description could itself echo a
     redacted-but-still-suggestive fragment of the matched pattern. Whoever
     designs this viewer should read exactly what `RunPreGateSecurityCheck`'s
     error strings contain before deciding they're safe to render verbatim in a
     "read-only viewer" that's explicitly being made *more discoverable/clickable*
     than before.

**General secret-exposure risk of making hidden-session output more discoverable.**
This tool drives Claude Code / AI agent sessions against real git worktrees. Two
concrete channels for sensitive content already exist in the schema/flow, independent
of the review-blocked case above:
- `ItemSession.verification_notes` — "freeform verification evidence reported via
  request_review (commands run, manual checks performed)" (schema comment,
  `session/ent/schema/item_session.go`). Agents self-report command output here;
  nothing constrains an agent from pasting an `env` dump, a credential it was
  debugging, or a curl command with a bearer token into this field. Today it's
  presumably only surfaced somewhere in the review UI already (worth confirming
  in Phase 3 exactly where) — but if the new viewer surfaces it *more
  prominently or for more session types* than today, that's an expansion of
  exposure, not neutral.
- Headless triage/re-review prompts are built from the backlog item's own title
  and description (`BuildHeadlessTriagePrompt`,
  `session/backlog_triage.go:40`+) plus repo content — if a backlog item's
  description itself contains a secret (pasted by mistake) or the LLM's
  `summary`/`suggestions` output happens to quote a secret it observed while
  reading the repo, that flows straight into `triage_result`, which the new
  viewer would render. This is a pre-existing exposure surface (triage_result is
  presumably already shown somewhere for triage results), but confirm whether
  today's display already gets any redaction/scrubbing before assuming the new
  viewer inherits safety "for free."
- **This is a single-user local tool (`localhost:8543`, "internal" security
  classification per requirements.md)**, which caps the blast radius — but "easily
  clickable" is explicitly the design goal here, and the risk isn't a new
  external audience, it's **discoverability creating a false sense that hidden
  session output is inert/safe to screen-share or paste into a bug report**,
  when in fact it can contain the exact kind of operational detail (tokens,
  paths, commands) a user wouldn't paste into a support channel if they thought
  about it. No action required beyond noting it in the plan and, if easy,
  reusing whatever ANSI-strip/truncation the existing `read_session_output` MCP
  tool already does (`maxOutputBytes = 10 * 1024`,
  `server/mcp/tools_terminal.go:20`) as a sanity ceiling for the new viewer too.

---

## 4. React + vanilla-extract stack risks for this UI

- **`.css.ts` is build-time-only; per-item collapse state is a runtime value.**
  Per `.claude/rules/css-architecture.md`, vanilla-extract cannot express
  "is this specific section, for this specific item, currently expanded" as a
  compile-time style. The rule's own "Dynamic Styles" section prescribes the
  fix (CSS custom property bridge via inline `style={{ '--x': ... }}` +
  `vars.xxx` fallback in the `.css.ts`), but **the codebase's actual existing
  precedent (`StuckItem.tsx`) doesn't use that bridge for expand/collapse at
  all** — it uses a plain **variant class swap**
  (`${styles.card} ${isExpanded ? styles.cardExpanded : ""}`) plus conditional
  JSX rendering (`{isExpanded && !justResolved && (...)}`), which is fully
  static from vanilla-extract's point of view (both classes are precompiled;
  React just picks which one to apply). **Prefer this simpler pattern over
  reaching for the CSS-custom-property bridge** — the bridge is for genuinely
  continuous/unbounded runtime values (an accent color, a computed width), not
  a boolean expanded/collapsed toggle, which is exactly what a build-time
  variant class is for. Using the custom-property bridge here would be the kind
  of unjustified complexity `.claude/rules/interface-pollution-checklist.md`
  warns against in the Go context, applied equally to CSS.
- **Animated height transitions and vanilla-extract don't mix well.** If the
  design calls for an animated expand/collapse (height 0 → auto), CSS alone
  can't animate to `auto` height without JS measurement (`scrollHeight`) or a
  grid-rows trick — vanilla-extract doesn't change this calculus, but it's worth
  deciding early (Phase 3 UX design) whether animation is even wanted; the
  existing precedent components (`StuckItem`, `UnfinishedItem`) appear to use
  instant show/hide via conditional render, not animated collapse — consistent
  with that precedent avoids a whole class of layout-thrash bugs for zero
  extra design cost.
- **Existing `.css.ts` file is large and already imported wholesale**
  (`BacklogItemDetail.css.ts`, imported as `* as styles`) — if the refactor
  splits `BacklogItemDetail.tsx` into subcomponents, decide whether each new
  subcomponent gets its own colocated `.css.ts` (matching the
  `Button/Button.css.ts` convention in the architecture doc) or continues
  sharing the parent's token file. Splitting styles without splitting the
  component (or vice versa) creates exactly the kind of "struct-wraps-struct"-
  style layering the interface-pollution checklist flags for Go, applied to
  React — a subcomponent that only exists to re-export styles from its parent
  adds a file without adding behavior.
- **Page-scroll convention must be re-verified after restructuring.** Per the
  same rules file, any scrollable panel needs **both** `height: "100%"` and
  `overflowY: "auto"` set on its root style, or content clips silently with no
  scrollbar. If the detail panel's root container changes (e.g. becomes a
  flex column of collapsible sections instead of one long scrolling block),
  re-check this invariant explicitly — it's a known "omit either property and
  it clips silently" failure mode, not something that fails loudly in dev.

---

## 5. Real-time streaming risk (WatchSessions et al.) vs. local collapse state

**Correction to the premise in the task brief: this component does not use
`WatchBacklogItems` at all — no such RPC exists.** A repo-wide search of
`proto/session/v1/*.proto` for `rpc Watch` finds `WatchSessions`,
`WatchReviewQueue`, `WatchInsights`, `WatchUserPRs`, `WatchUnfinishedWork` — but
**no `WatchBacklogItems`**. `BacklogItemDetail.tsx` instead **polls**: a
`setInterval(() => void load(), 5_000)` that's active only while
`triageStatus === "running"` OR (`status === "review"` AND no/PENDING
`gateVerdict`) OR `status === "pr_pending"`, and is explicitly suspended while
`editMode` is true so a background refresh can't clobber unsaved edits
(`BacklogItemDetail.tsx:243-249`). **This changes the shape of the risk**: it's
not "does a gRPC stream push mid-interaction," it's "does a 5-second poll's
`load()` call replace `item` with a new object and blow away in-progress local
UI state." Concretely:

- **Every 5s during polling windows, `item` is a freshly fetched object** (new
  reference, not a patch/merge). Any new collapse-state `useState` you add is
  safe from this *only if it's stored independently of `item`* (which is the
  natural way to write it, e.g. `const [expandedSections, setExpandedSections] =
  useState<Set<string>>(...)`) — React does not reset unrelated `useState` slots
  just because a sibling/prop value changed. The real risk isn't the poll
  resetting collapse state; it's the **existing editMode-suspend-polling pattern
  not being extended to new interactive elements**. E.g. if the manual review
  form or a new "expanded to inspect a session's output" state should also
  suspend polling (so a mid-typing verdict summary isn't visually disrupted by a
  `load()`-triggered re-render), that has to be added deliberately — copy the
  `editMode` guard's shape rather than assuming the existing guard covers it.
  Today `showManualReview` open does *not* suspend the poll — worth flagging as
  a pre-existing gap the redesign could either fix or knowingly inherit.
- **The one real streaming RPC in play, `WatchSessions`, is not currently wired
  into `BacklogItemDetail.tsx` at all** — no match for `WatchSessions` in that
  file. `SessionMonitor.tsx` (rendered inside the detail panel for the active
  session) manages its own terminal/output state independently. If the new
  read-only viewer for triage/headless-review sessions is implemented by reusing
  or adjacent to `SessionMonitor`/`WatchSessions`-based live-tailing, then the
  actual risk becomes: **a live-tailed session's output view auto-scrolling or
  re-rendering while the user has manually scrolled up to read something** — a
  classic streaming-UI pitfall, distinct from the collapse-state question, and
  worth explicit design attention (e.g. "pause autoscroll if user has scrolled up,
  resume on scroll-to-bottom" — check whether `TerminalOutput.tsx` or
  `SessionMonitor.tsx` already solves this, since the new read-only viewer should
  reuse that solution rather than re-deriving it).
- **Section-level auto-expand tied to live status is a double-edged design
  choice.** Section 2 above recommends auto-expanding sections relevant to
  current status (e.g. expand Actions while in `review`). Combined with polling:
  if a poll flips `item.status` while the user is mid-interaction with a
  *different* section, an auto-expand-on-status-change rule could suddenly
  change what's expanded/visible under the user's cursor without any click —
  disorienting, and worse than doing nothing. Recommend: auto-expand rules
  should only apply on **first render for a given itemId**, not on every
  subsequent poll-driven update while the panel is already open and the user may
  be mid-interaction. This needs an explicit "did the user manually toggle this
  section" override flag per section, so a poll-driven status change never
  overrides a user's explicit collapse/expand choice once made.

---

## Summary of Concrete Files/Lines to Anchor the Plan Phase Against

| Concern | File:Line |
|---|---|
| Manual review verdict form | `web-app/src/components/backlog/BacklogItemDetail.tsx:1152` (`showManualReview` block) |
| Inert triage/headless-review/review-blocked spans | `web-app/src/components/backlog/BacklogItemDetail.tsx:1333` |
| No itemId-based reset of local UI state | `web-app/src/components/backlog/BacklogItemDetail.tsx:126-220` (state decls), no `key={itemId}` in `BacklogItemPanel.tsx` |
| Polling (not WatchBacklogItems) | `web-app/src/components/backlog/BacklogItemDetail.tsx:243-249` |
| Headless triage/re-review = no scrollback | `server/services/backlog_service_triage.go:204-205`, `session/headless/runner.go`, `session/ent/schema/item_session.go` (`triage_result` field) |
| `review-blocked-*` = synthetic security-block marker, no session at all | `session/review_gate.go:220-236` |
| Existing accordion precedent to reuse | `web-app/src/components/backlog-stuck/StuckItem.tsx` + `.css.ts`, `web-app/src/components/unfinished/UnfinishedItem.tsx` |
| Existing tests to preserve/extend | `BacklogItemDetail.test.tsx`, `.regression.test.tsx`, `.shipPR.test.tsx`, `.markdown.test.tsx`; no e2e spec currently covers this component |
