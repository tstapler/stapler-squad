# UX Research: context-compression

SDD Phase 2. Scope per `requirements.md` acceptance criterion "Compression
event shown in session detail UI" — this doc covers only the one surface
neither sibling project already designs: a **discrete "Stapler-Squad
compressed this session" event marker**, distinct from Claude Code's own
native compaction. It does not re-derive anything from
`project_plans/context-compaction-detection/research/ux.md` (live "⟳
Compacting" sub-status chip) or `project_plans/context-health-monitoring/research/ux.md`
(persistent context-quality trend badge) — both were read in full first and
are treated as settled precedent, referenced by name below rather than
repeated.

## 1. Would a second badge next to "⟳ Compacting" be confusing? Merge or keep distinct?

**Recommendation: do not add a second live badge. Keep one live signal, add
a separate past-tense event record.** The two things are not the same kind
of UI object and forcing them into the same visual slot (the session-card
badge row) is the actual confusion risk, not the wording.

Reasoning, grounded in what `requirements.md`'s own "Open architectural
question" (lines 66-80) leaves unresolved:

- `context-compaction-detection` badges a **live, transient sub-status**:
  Claude Code's CLI subprocess is, right now, auto-compacting its own
  context, self-clears in ~30-60s, sourced from `DetectedStatus`/`SubStatus`
  (terminal-pattern detection of the CLI's own behavior).
- This project's compression, per the architectural question, may turn out
  to be **the same underlying event** — if Phase 2/architecture research
  confirms the Go controller can't inject a synthetic user message into a
  CLI subprocess it only supervises via `--resume`/stdout (no raw
  chat-completions API to call `compress()` against), then "Stapler Squad
  compressed the session" *is* Claude Code's native auto-compaction, just
  observed from the outside. In that outcome there is only one phenomenon,
  and a second "context compressed" badge would be a literal duplicate of
  the compaction-detection badge — two pills, same underlying fact, gratuitous
  confusion. Do not build a second live badge in that case; this
  acceptance criterion is satisfied by pointing at the existing
  `⟳ Compacting` chip (already-shipped/in-flight from the sibling project).
- If architecture research instead confirms Stapler Squad implements its
  *own* independent mechanism (a genuinely separate synthetic-summary
  injection distinct from whatever Claude Code's CLI does internally), then
  two real, distinguishable events can occur on the same session over its
  lifetime, and the UI must not blur them — a user who sees a summary
  attributed to the wrong actor loses the ability to trust either signal
  (same "don't cry wolf, don't lie quiet" trust argument the health-badge
  research makes in its Jobs-to-be-Done section). In that case:
  - **Never use a bare, unattributed "Compressed"/"Compacted" label.** Every
    existing chip/badge in this codebase (`SubStatusChip`, `StatusBadge`,
    the health-badge research's own recommendation) pairs icon + explicit
    plain-language text; the actor must be named the same way. Use "Claude
    compacted" (native CLI auto-compaction, housekeeping, no content the
    user asked for) vs. "Stapler Squad summarized" (an external actor
    injected a synthetic turn — a content change worth being able to
    inspect, since it can affect what the agent "remembers").
  - The two must differ in more than the two words in the label —
    following the health-badge research's shape-not-just-color principle
    (§3 of that doc), use **distinct icons**, not the same glyph recolored:
    e.g. keep `⟳` for Claude's own live compaction (already assigned by the
    sibling project) and use a different, non-cyclical glyph for a
    Stapler-Squad-authored summary event (something that doesn't read as
    "spinning/in-progress," since — see §2 below — this is a discrete past
    event, not a live status).
- Either way, **this project's UI surface is not a second card-row badge**.
  Card-row badge space is scarce and reserved for live/ambient state (per
  the health-monitoring research's own §2 on badge-row placement fights);
  a compression event is a point-in-time occurrence a user reviews when
  checking why a session's behavior changed, which argues for a session
  **detail**-scoped record, not a session **card**-scoped chip. This also
  matches the requirements doc's own phrasing — "a badge on the timeline,"
  i.e. inside session detail, not the card.

## 2. Where does a discrete compression EVENT belong in the session detail UI?

**Finding: there is no existing generic session-event-timeline component to
reuse.** Searched `web-app/src/components/sessions/` for `timeline`,
`EventLog`, `ActivityFeed`, and equivalents; the only matches repo-wide are:

- `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx` — but
  this is scoped to **backlog items**, not sessions (renders
  `BacklogItem.statusEvents`, a status-transition audit trail). It is not
  directly reusable, but it is the closest **structural pattern** in the
  codebase for "a capped, collapsible list of discrete past events with a
  timestamp and a short description," and should be imitated rather than
  invented from scratch:
  - `role="list"` / `role="listitem"` on the container/rows (not
    `role="status"` — that ARIA role is for live-announced regions, which a
    historical log is not; see §3).
  - Wrapped in `CollapsibleSection`, collapsed by default.
  - Capped rendering via the existing `useShowMore` hook (cap of 8), not an
    unbounded list.
  - Each row: a short structured summary line (here: `from → to` transition)
    + a meta line (`formatDate(...)` + actor) + optional freeform note.
  - Always renders, even with zero events (explicit "No status history
    recorded" text) rather than hiding the section — the doc comment
    explains this was a deliberate fix (issue #198) because hiding it made
    the feature look broken for older items with no events yet. The same
    logic applies here: a session with zero compression events (the common
    case, since compression should be rare) should show an explicit
    "No compression events" state, not omit the section.

- `web-app/src/components/sessions/CheckpointList.tsx` — a **session**-scoped
  list of discrete named events (checkpoints) with `formatRelativeTime`,
  a label, and metadata pills (git SHA, conversation UUID), `role="list"`
  with `<li>` items, "Show all (N)" beyond `MAX_VISIBLE = 10`. This is the
  closer sibling by domain (sessions, not backlog) and its per-item shape
  (label + relative time + metadata pill) is a good template for a
  compression-event row (e.g. "Context compressed" + relative time + a pill
  showing tokens-before→after or turns-summarized count). **Caveat, verified
  by grep**: `CheckpointList` is not currently rendered anywhere in the app
  (`grep -rl "CheckpointList" web-app/src --include="*.tsx"` finds only its
  own file and no import site; `CheckpointButton` is similarly unreferenced
  outside its own file). Treat it as a well-shaped but currently-orphaned
  component — useful as a design reference, but confirm with whoever owns
  checkpoints before assuming it's live, and don't assume its existence
  means there's a working "session events" surface today.

- `SessionDetailView.tsx`'s **Info tab** (`activeTab === "info"`) is a flat
  key/value grid (Instance ID, Status, Session Type, Created, Updated,
  Branch, Working Directory, ...) — not a list/log construct, but it is
  the tab already scoped to "facts about this session" and is open
  regardless of session state (unlike **Summary**, which is `disabled`
  until `isSessionTerminal(session.status)` — i.e. only enabled after the
  session ends, making it the wrong home for an event a user wants to see
  *during* a still-running long session).

- `SessionLogsTab.tsx` (via the shared `LogViewer`) shows raw backend log
  lines — the technically-complete but wrong home: it's an unfiltered,
  monospace debug stream, not a curated user-facing event list. A user
  checking "did this session get summarized and is that why its behavior
  changed" should not have to grep raw logs to find out.

- `DetectionEventsPanel.tsx` is explicitly `?debug=1`-gated developer
  tooling (its own doc comment says so) — not a candidate for a
  user-facing feature.

**Recommendation for Phase 3 planning**: add a small, capped, collapsible
list to the **Info tab** (or a new lightweight section directly below it in
the same tab, not a new top-level tab — a handful of expected events over a
session's life does not justify a 9th tab alongside Terminal/Diff/VCS/
Files/Logs/Info/Browser/Artifacts/Summary), built on the
`WorkflowHistorySection` pattern (`role="list"`, `CollapsibleSection`,
capped + show-more, explicit empty state) with `CheckpointList`'s per-item
visual shape (label, relative timestamp, metadata pill). Do not build a new
generic "timeline" abstraction for this alone — one event type does not
justify a reusable framework; if `context-health-monitoring` or other
projects later need the same shape, extract it then.

## 3. Accessibility — non-color signal, matching `SubStatusChip` conventions

- **Icon is always decorative, text label always present** — the same rule
  both sibling research docs already state and every existing chip
  (`SubStatusChip.tsx`) follows: `aria-hidden="true"` on the glyph, a real
  text label alongside it, never color or icon alone as the signal. Apply
  identically here: whatever glyph is chosen for "Stapler Squad summarized"
  (see §1) needs `aria-hidden="true"` plus a text label ("Context
  compressed" / "Stapler Squad summarized"), not a bare icon.
- **Role differs from the live-badge family, and that's correct, not an
  inconsistency.** `SubStatusChip`/`StatusBadge` use `role="status"`
  (implicit `aria-live="polite"`) because they announce a *current* state
  change a screen-reader user should be told about as it happens. A
  compression event row in a historical list is not live-announced content
  — it already happened and is being displayed as part of a static list a
  user opens deliberately. Use `role="listitem"` inside a `role="list"`
  container (matching `WorkflowHistorySection`/`CheckpointList`), not
  `role="status"`. Applying `role="status"` to a list of past events would
  cause screen readers to announce every row on mount/update, which is
  exactly the "chatty" anti-pattern the compaction-detection research
  explicitly warns against (§3 of that doc: "announce without being
  chatty").
- **`title` tooltip for the "why," consistent with every chip in the
  codebase**: e.g. `title="Stapler Squad summarized 42 earlier turns to
  free up context space"` — matches the convention both sibling docs
  document (`SubStatusChip` and the recommended `ContextHealthBadge` both
  carry a `title` explaining the state in plain language, not just a short
  label).
- **Shape, not just color, if two distinct actors are ever shown side by
  side** (per §1's "if research finds two real mechanisms" branch): reuse
  the health-monitoring research's §3 finding almost verbatim — a
  colorblind user must be able to tell "Claude compacted" from "Stapler
  Squad summarized" from icon shape alone, not from hue. Don't ship two
  events distinguished only by a blue vs. purple dot.
- **CSS architecture constraint** (`.claude/rules/css-architecture.md`,
  already flagged by the health-monitoring research and equally applicable
  here): any new component/row styling is vanilla-extract (`.css.ts`),
  token-based colors from `theme.css.ts`, not a `.module.css` file and not
  hardcoded hex.

## Summary of concrete recommendations for Phase 3 planning

1. **Don't add a second card-row badge.** This feature's UI surface is a
   session-detail event record, not a `SessionCard` badge — card badge
   space is reserved for live/ambient state per the two sibling projects'
   own placement rules.
2. **Resolve the architecture question first** (already flagged in
   `requirements.md`): if Stapler Squad's compression turns out to just be
   an outside view of Claude Code's own native auto-compaction, this
   acceptance criterion is satisfied by the sibling project's existing
   `⟳ Compacting` chip plus a historical log entry — do not build a
   redundant second live indicator. If it's a genuinely separate mechanism,
   every event must be explicitly actor-attributed ("Claude compacted" vs.
   "Stapler Squad summarized"), never a bare "Compacted."
3. **No reusable session-timeline component exists yet.** Build a small,
   capped (~8-10), collapsible list in the Info tab (not a new tab),
   modeled structurally on `WorkflowHistorySection.tsx` (list/listitem
   roles, `CollapsibleSection`, `useShowMore`, explicit zero-state) with
   `CheckpointList.tsx`'s per-row visual shape (label + relative time +
   metadata pill) — noting `CheckpointList` itself is currently unreferenced
   in the app, so treat it as a design template, not working code to import.
4. **Accessibility**: `role="listitem"`/`role="list"` (not `role="status"`
   — this is historical, not live, content), `aria-hidden` icon + visible
   text label + `title` tooltip, distinct icon shapes (not just color) if
   two actor-attributed event kinds coexist, vanilla-extract styling with
   theme tokens.
