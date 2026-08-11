# Build vs. Buy — Collapsible Sections, Lifecycle Tracker, Read-Only Session Log Viewer

Agent 6 research for `backlog-item-detail-ux`. Scope: three UI pieces for the redesigned
backlog item detail panel — (a) collapsible/accordion progressive-disclosure sections,
(b) a compact lifecycle/stage tracker, (c) a read-only session-output/log viewer.
Constraint: all new CSS must be vanilla-extract `.css.ts`; no CSS-in-JS runtime libs.

## Method

Checked `web-app/package.json` dependencies, then searched the codebase for existing
`<details>`/accordion usage (`Grep` for `accordion|collapsible`, `<details`) and existing
terminal/log rendering (`@xterm/xterm` importers, `ansi-to-html` importers). Read the
matching component files at targeted line ranges.

---

## Piece (a): Collapsible / Accordion Sections

### Option 1 — Existing OSS library already a dependency

`package.json` has `@radix-ui/react-dialog`, `@radix-ui/react-tabs`, `@radix-ui/react-tooltip`,
`@radix-ui/react-slot` — **no `@radix-ui/react-accordion` or `@radix-ui/react-collapsible`**.
Radix is already the house pattern for unstyled/composable primitives elsewhere in this repo
(Dialog, Tabs, Tooltip all render via `Slot`/data-attribute styling hooks that vanilla-extract
`selectors` target), so adding `@radix-ui/react-accordion` (or the lighter
`@radix-ui/react-collapsible` for single-panel disclosure) is a natural, low-risk extension of
an already-adopted family rather than a net-new dependency category.

- **Pros**: correct ARIA (`aria-expanded`, `aria-controls`, `role`), keyboard nav (Home/End/Arrow
  for accordion groups) handled for free; animatable via CSS custom properties
  (`--radix-accordion-content-height`) which composes cleanly with vanilla-extract; same vendor
  family as existing Dialog/Tabs/Tooltip so no new update/security-review surface; unstyled by
  design, so no CSS-in-JS conflict.
- **Cons**: one more package to pin/update; slightly more indirection than native `<details>`
  for the simplest single-section case; multi-section "type=multiple" accordion API has a
  learning curve if unfamiliar.
- **Verdict: Recommended** for any section that needs grouped/coordinated expand-collapse
  behavior (e.g. multiple named sections in the detail panel where only some stay open) or
  where nested interactive content (buttons, forms) lives inside the collapsible body — native
  `<details>` has known focus/interaction quirks with nested interactive elements in some
  browsers. Use `@radix-ui/react-collapsible` for a single independent panel, `@radix-ui/react-accordion`
  for a multi-section group.

### Option 2 — SaaS/managed API

N/A — pure client-side UI behavior, no external service involved.

### Option 3 — LLM-generated hand-rolled implementation vs. battle-tested primitive

Hand-rolling means owning: `aria-expanded` sync, `id`/`aria-controls` pairing, focus retention
across expand/collapse, `prefers-reduced-motion` handling for open/close animation, and (for
accordion groups) roving tabindex/arrow-key navigation between headers. These are exactly the
class of "easy to get subtly wrong" details called out in the task — and this repo's own
`WorkflowsPanel.tsx` `RecentRuns` component (see Piece (d) below) demonstrates the failure mode
concretely: it's a `<button>` + conditional `<div>` toggle with **no `aria-expanded`,
`aria-controls`, or `id` pairing at all** — keyboard/tab semantics work only because it happens
to use a real `<button>`, but a screen reader gets no signal that the button controls a region
or what its current state is.

- **Custom is worth it when**: the "collapsible" is trivial and single-purpose — e.g. a single
  `<details>`/`<summary>` disclosure with no nested interactive controls and no group
  coordination (this repo already does this well in several places, see Piece (d)).
- **Custom is not worth it when**: there's more than one section that needs group semantics,
  the panel body contains interactive elements (buttons/inputs), or animation is desired —
  the ARIA/keyboard surface area grows past what's reasonable to hand-verify, even for a
  personal tool. Given the detail panel redesign explicitly calls for *multiple* progressive-
  disclosure sections, this tips toward Radix.
- **Personal-tool framing**: this is a solo-maintained tool, not a component library shipped to
  third parties, which lowers the bar somewhat (no cross-browser support matrix to maintain,
  no external a11y audit). But the repo's own CI already runs Axe Core UX-analysis blocking on
  WCAG AA violations for any PR touching `web-app/src/` (per `CLAUDE.md`) — so "personal tool"
  doesn't remove the ARIA correctness requirement, it just removes the "why bother, nobody
  else uses it" argument. The CI gate makes the hand-rolled ARIA risk concrete, not
  hypothetical.
- **Verdict: Recommended** to adopt `@radix-ui/react-accordion`/`-collapsible` for the new
  multi-section detail panel; native `<details>` remains fine for simple, single, non-nested
  disclosures elsewhere.

### Option 4 — Fork or adapt existing in-codebase pattern

Two independent patterns already exist, neither used consistently:

1. **Native `<details>`/`<summary>`**, styled via vanilla-extract, used in:
   - `web-app/src/components/sessions/GoalPanel.tsx:120` (`<details className={panelContainer}>`)
   - `web-app/src/components/ui/ErrorState.tsx:68` (`<details className={details}>`)
   - `web-app/src/components/sessions/SuggestedRuleCard.tsx:201` (`<details className={sourceCommandsDetails}>`)
   - `web-app/src/components/rules/RuleBuilderForm.tsx:565` and
     `web-app/src/components/sessions/ApprovalRulesPanel.tsx:664` (inline `style={}`, not
     vanilla-extract — pre-existing CSS-architecture drift worth flagging but out of scope here)

2. **Fully hand-rolled `useState` accordion**, in
   `web-app/src/components/workflows/WorkflowsPanel.tsx` (`RecentRuns`, ~lines 34–86): a
   `<button>` toggle + conditionally-rendered `<div>`, styled via `WorkflowsPanel.css.ts`
   (`runsAccordion`, `runsToggle`, `runsList` classes) — no `aria-expanded`/`aria-controls`, as
   noted above.

- **Pros of adapting**: `GoalPanel.tsx`'s `<details>` pattern is the closest existing match in
  both intent (compact summary + expandable body inside a session-related panel) and styling
  approach (vanilla-extract, `panelContainer`/`summary`/`body` class shape) — a real template to
  copy for a single-section disclosure.
- **Cons**: neither existing pattern solves *grouped* accordion behavior (multiple sections,
  only-one-open coordination) that the redesigned detail panel likely wants; extracting a shared
  component from `GoalPanel.tsx` now versus adopting Radix largely converges on rebuilding what
  Radix already provides, just without keyboard/focus-group semantics.
- **Verdict: Viable** for a single always-independent section (skip Radix, copy the
  `GoalPanel.tsx` `<details>` + vanilla-extract pattern) — but **not recommended** as the
  sole approach for the full progressive-disclosure section set, given the ARIA gap already
  visible in `WorkflowsPanel.tsx`.

**Recommendation for (a)**: Add `@radix-ui/react-accordion` (covers the multi-section case) and
reuse it for single sections too, for consistency — one dependency instead of three divergent
patterns (Radix, `<details>`, hand-rolled `useState`). Style via vanilla-extract `selectors`
targeting Radix's `data-state="open"/"closed"` attributes, matching the existing Dialog/Tabs
integration pattern already in this codebase.

---

## Piece (b): Lifecycle / Stage Tracker (Pipeline Summary)

- Searched for `stepper|pipeline.*tracker|stage.*tracker|lifecycle.*(bar|track)|progress.*stage`
  across `web-app/src` — **no existing component found**. `recharts` is a dependency but is a
  charting library, not a stepper/pipeline primitive, and would be overkill for a compact
  status strip.
- **Radix/Headless UI do not offer a stepper primitive** — this is a narrow, presentation-heavy
  widget (a row of stage chips/dots with connecting lines and a current-stage highlight) with
  no complex interaction or focus-management surface (it's typically non-interactive or, at
  most, a single "jump to stage" click). This is exactly the kind of small, project-specific,
  purely-visual component where a hand-rolled build is the calibrated choice.
- **Verdict: Build from scratch**, styled purely with vanilla-extract (recipe/variants per
  stage-state: `pending`/`active`/`done`/`blocked`, matching the status-chip variant pattern
  already used in `GoalPanel.css.ts`'s `statusChipVariants`). No new dependency needed. If any
  interactivity is added later (e.g., clickable stages), reuse Radix's `Tabs` semantics rather
  than hand-rolling roving tabindex.

---

## Piece (c): Read-Only Session-Output / Log Viewer

Two credible reuse candidates already exist in the codebase — this is the piece with the most
build-vs-buy leverage.

### Candidate 1 — `@xterm/xterm` in read-only mode

`XtermTerminal.tsx` (`web-app/src/components/sessions/XtermTerminal.tsx`, 989 lines) is the
existing interactive terminal, using `@xterm/xterm` + `addon-fit`, `addon-web-links`,
`addon-search`, `addon-serialize`, `addon-webgl`, plus custom mouse-tracking
(`src/lib/terminal/mouseTracking.ts`), gesture handling
(`src/lib/hooks/useTerminalGestures.ts`, `useMobileTerminalGestures.ts`), and a context menu
(`TerminalContextMenu`). xterm does support a read-only mode (`terminal.options.disableStdin =
true`, omit the `onData` handler), so reuse is technically possible.

- **Pros**: pixel-perfect terminal rendering including ANSI colors, cursor positioning, box-
  drawing characters — a true "terminal-like scrollback" for triage sessions where raw fidelity
  matters; WebGL-accelerated rendering already proven at scale in this codebase (benchmarks
  exist: `tests/e2e/benchmarks/terminal-throughput.spec.ts`).
  Reusing it avoids re-solving ANSI/cursor-sequence parsing.
- **Cons**: heavy for a "no interactivity" requirement — the 989-line component carries mouse
  tracking, gesture handling, context menu, and WebGL setup that would all need to be
  conditionally stripped or bypassed for a lightweight read-only viewer; pulls in the full
  `@xterm/xterm` + addon bundle size for something the requirement explicitly says needs no
  interactivity; canvas/WebGL rendering is arguably the wrong performance profile for a static
  scrollback dump (WebGL setup/teardown cost per open, versus plain DOM/virtualized text).
- **Verdict: Not recommended** as-is; only reconsider if faithful ANSI/cursor-sequence replay
  (not just color) turns out to be a hard requirement for triage review.

### Candidate 2 — Existing `logs/` component suite (recommended)

A full read-only-oriented log viewer stack already exists and is a much closer match:

- `web-app/src/lib/logs/logParser.ts` — ANSI-to-HTML pipeline (`ansi-to-html` +
  `DOMPurify` sanitization already wired in), log-level detection (`ERROR`/`WARN`/`INFO`/
  `DEBUG`/`TRACE`), and search-term segmentation for highlighting.
- `web-app/src/components/logs/VirtualLogList.tsx` — `react-virtuoso`-backed virtualized list
  (already a dependency: `@tanstack/react-virtual` *and* `react-virtuoso` are both present) with
  live-tail "follow mode," throttled `aria-live` announcements, and **row-level accordion
  expansion** (`ExpandedLogDetail.tsx`) — i.e., piece (a)'s disclosure pattern already appears
  inside this component for per-row detail.
  - `web-app/src/components/logs/LogRow.tsx`, `LogViewerToolbar.tsx`, and the
    `useLogViewer.ts` hook round out the suite (search, follow/pause, level filter).
  - A near-duplicate copy also exists under `web-app/src/components/shared/` (`ExpandedLogDetail.tsx`,
    `LogViewerToolbar.tsx`) — worth reconciling which is canonical before extending, to avoid
    a third divergent copy.
- **Pros**: purpose-built for exactly "read-only, terminal-like scrollback text, no
  interactivity" — it's a *log* viewer, not an interactive terminal, so there's no stdin/mouse-
  tracking/context-menu surface to strip; virtualization already solves the "long scrollback"
  performance problem without WebGL; ANSI color rendering already implemented and sanitized;
  directly reusable or lightly adapted (e.g., feeding it session output text instead of
  structured log entries) rather than built new.
- **Cons**: was built for a structured/leveled log format (`LogEntry` with detected level,
  timestamp, etc.) — raw session/tmux scrollback may not cleanly decompose into that shape and
  might need a thin adapter (e.g., treat each line as an `UNKNOWN`-level entry, or skip level
  detection entirely for this use case); the `logs/` vs `shared/` duplication should be resolved
  first so the new viewer builds on one canonical copy, not both.
- **Verdict: Recommended.** Adapt `VirtualLogList` + `logParser.ts` for the new session-output
  viewer rather than building a new component or repurposing `XtermTerminal`. This satisfies
  the "read-only... no interactivity" requirement more precisely than xterm, avoids a new
  dependency, and reuses vanilla-extract-styled components that already exist
  (`VirtualLogList.css.ts`, `LogViewerToolbar.css.ts`).

---

## Summary Table

| Piece | Build | Buy/Reuse-dependency | Fork/Adapt-in-repo | Verdict |
|---|---|---|---|---|
| (a) Collapsible sections | Viable for 1-off simple case | `@radix-ui/react-accordion`/`-collapsible` (new dep, same family as existing Radix usage) | `<details>` pattern in `GoalPanel.tsx`; hand-rolled pattern in `WorkflowsPanel.tsx` (has ARIA gap) | **Recommended: add Radix Accordion/Collapsible**; keep native `<details>` for trivial single disclosures |
| (b) Lifecycle/stage tracker | Yes — small, presentational, no suitable primitive exists | N/A — no library fits | No existing component | **Recommended: build from scratch**, vanilla-extract recipe/variants |
| (c) Read-only session log viewer | Not recommended from scratch | `@xterm/xterm` reuse in read-only mode — viable but heavy/wrong-shaped | `VirtualLogList` + `logParser.ts` + `ExpandedLogDetail` (`logs/` suite) | **Recommended: adapt existing `logs/` component suite**, not xterm, not new-from-scratch |

## Open Follow-ups for Later Phases

- Reconcile `web-app/src/components/logs/` vs `web-app/src/components/shared/` duplicate
  `ExpandedLogDetail.tsx`/`LogViewerToolbar.tsx` before building on either.
- Confirm whether triage/headless-review session output needs literal ANSI cursor-repositioning
  fidelity (would push back toward xterm) or is line-oriented append-only text (fits
  `VirtualLogList` cleanly as-is).
- If Radix Accordion is adopted, add a shared vanilla-extract style module (e.g.
  `Accordion.css.ts`) so the detail panel and any future accordion consumers share one styled
  wrapper instead of re-deriving `data-state` selectors per usage site.
