# Stack Research: backlog-item-detail-ux

Agent: Stack (research question 1/N)
Scope: what's already installed vs. what needs adding for progressive disclosure UI, a read-only session/log viewer, and status/timeline summary patterns.

## 1. Package inventory (`web-app/package.json`)

Already installed, relevant to this feature:

| Package | Version | Relevance |
|---|---|---|
| `@radix-ui/react-dialog` | ^1.1.15 | Modal.tsx is built on this |
| `@radix-ui/react-tabs` | ^1.1.13 | Available, **but no component in `ui/` currently wraps it** — no existing `<Tabs>` primitive |
| `@radix-ui/react-tooltip` | ^1.2.8 | Tooltip.tsx wraps this |
| `@radix-ui/react-slot` | ^1.2.4 | Used for `asChild`-style composition (e.g. Button) |
| `react-virtuoso` | ^4.18.7 | Powers `VirtualLogList` — virtualized list for large log/output volumes |
| `@xterm/xterm` + addons (fit, search, serialize, web-links, webgl) | ^6.0.0 / addons | Full interactive terminal (`XtermTerminal.tsx`) — read-write, requires a live PTY via `StreamTerminal` |

**Not installed**: `@radix-ui/react-accordion`, `@radix-ui/react-collapsible`, any headless-ui package, framer-motion/motion. No accordion or collapsible primitive exists anywhere in the codebase today (confirmed via grep — the only `isOpen`/`isExpanded` hits are modal/dropdown open-state, not section collapse).

## 2. Progressive disclosure primitives — gap

`web-app/src/components/ui/` has no `Accordion`, `Collapsible`, `Disclosure`, or `Tabs` component. `BacklogItemDetail.tsx` (1577 lines) renders all 12+ sections as plain `{condition && <div>...}` blocks with no expand/collapse state at all — this matches the requirements' description exactly.

**Recommendation**: add `@radix-ui/react-accordion` (same vendor/version family already in use — `@radix-ui/react-dialog`/`tabs`/`tooltip` are all `^1.x`, keeps API idioms consistent) as the new dependency for collapsible sections. Radix accordion is unstyled/headless and composes cleanly with the existing vanilla-extract `.css.ts` pattern (see `Modal.css.ts` for the precedent of a Radix primitive wrapped in a `recipe()`).

`@radix-ui/react-tabs` is **already a dependency but unused** — worth checking with the product/design layer whether the redesign wants a tдок/tab-per-lifecycle-phase layout (e.g. "Overview / Planning / Review / History" tabs) instead of, or alongside, an accordion. Either primitive is a same-cost addition since tabs is already in `node_modules`; accordion is a new install.

For a plain native fallback (`<details>`/`<summary>`), the codebase has zero precedent and it doesn't give the animation/multi-open-mode control Radix accordion does — not recommended given Radix is already the established pattern for interactive disclosure (Dialog, Tabs, Tooltip all Radix).

## 3. Read-only log/output viewer — mostly reusable, with a real gap

### What exists and is directly reusable
`web-app/src/components/logs/` (and a near-duplicate copy under `web-app/src/components/shared/` — see "Duplication" below) is a **fully-built, virtualized, read-only log viewer**:
- `LogViewer.tsx` — top-level component, already parameterized by `source: "app" | "session"` + `sessionId`
- `VirtualLogList.tsx` — react-virtuoso-backed virtual list (handles large volumes without perf cost)
- `LogRow.tsx`, `ExpandedLogDetail.tsx` — row rendering + expand-in-place detail
- `LogViewerToolbar.tsx`, `LevelFilterChips.tsx`, `SearchWithHistory.tsx`, `TimeRangePicker.tsx`, `DensityToggle.tsx`, `ExportButton.tsx`, `JumpToLatestButton.tsx` — full toolbar: search, level filter, live-tail follow/pause, jump-to-latest, export
- `ShortcutHelpOverlay.tsx` — keyboard shortcuts (`/`, Esc, g/G, =, ?, Cmd+F)
- Backing hook: `web-app/src/lib/hooks/useLogViewer.ts`, calling `SessionService.getLogs` (proto `GetLogsRequest`/`GetLogsResponse`, `session_service.go:2591` → `utilitySvc.GetLogs`)
- Existing consumer: `web-app/src/components/sessions/SessionLogsTab.tsx` — `<LogViewer source="session" sessionId={sessionId} />`, used today as the "Logs" tab on a live session.

This is a strong, idiomatic starting point for the "read-only session viewer" requirement — it is already read-only, already virtualized, already has search/filter/export, and already vanilla-extract styled.

### The gap: `GetLogs` returns *structured application log lines tagged with a sessionId*, not raw terminal/PTY output
`GetLogs` queries the app's own structured log store filtered by `sessionId` (i.e., "log lines the Go app emitted about this session"), not the tmux/PTY scrollback a user would see in the terminal. The actual terminal transcript is served a different way:
- Interactive terminal: `XtermTerminal.tsx` + `useTerminalStream.ts` open a bidirectional `StreamTerminal` RPC (proto `session.proto:33`) and request historical content via `ScrollbackRequest`/`ScrollbackResponse` messages defined in `events.proto` (`ScrollbackRequest`/`ScrollbackResponse`/`ScrollbackChunk`), backed by `Instance.GetScrollbackHistory()` (`session/instance_tmux.go:513`) and `session/scrollback/manager.go` + `session/scrollback/buffer.go` (circular buffer).
- This scrollback path assumes a live PTY-backed `Instance` — appropriate for real work/review sessions, not for synthetic markers.

**Bigger gap — confirmed, not hypothetical**: `headless-review-*` and `review-blocked-*` prefixed "session IDs" (flagged as a rabbit hole in requirements.md) are **not real session.Instance objects**. Grep confirms:
- `session/review_gate.go:229` synthesizes `"review-blocked-" + uuid.New().String()` purely as an ID stamped onto a recorded verdict — no backing `Instance`, no tmux session, no scrollback buffer.
- `headless-review-*` only appears as a test fixture pattern (`session/backlog_lifecycle_test.go`), generated the same way — a synthetic UUID recorded against a `LinkedSession`, backed by a **headless LLM call** (`session/headless` package, `session/backlog_review.go` `BuildHeadlessReviewPrompt`), not a terminal session.
- Today's frontend (`BacklogItemDetail.tsx:1333-1336`) already special-cases these: it renders them as inert `<span>` text (`"headless review"` / `"review blocked"` labels) specifically *because* there's no session to open — confirming the UX bug in the problem statement is a direct consequence of this backend reality, not just a missing `<a>` tag.

**Implication for design**: a "read-only session viewer" for triage/headless-review needs a *different* backend data source than PTY scrollback — likely the persisted headless-call prompt/response or review verdict content (need to trace where that's stored — `session/review_gate.go`, `session/backlog_review.go`, and whatever persists `TriageResult`/verdict text against the `BacklogItem`/`LinkedSession` record). This is very likely the "new backend RPC surface" the requirements.md rabbit-hole section already anticipates. Recommend a fast-follow research pass (backend/data agent) to trace exactly what's persisted for these synthetic sessions so the new RPC can serve real content rather than "no output available."

For genuine triage sessions (`role === "triage"`, real `Instance`-backed, currently also rendered as inert text per the same conditional) and non-headless review sessions that *are* real sessions — the existing scrollback/terminal RPC path likely already works read-only simply by omitting the input-forwarding side of `XtermTerminal`, i.e., a `ReadOnlyTerminalViewer` variant that only consumes `ScrollbackResponse`/stream output and never sends `TerminalData` input frames. That's a much smaller lift than the headless case.

### Duplication flag (relevant to the "consolidate duplicative info" scope item, but also a code-hygiene note)
There are **two parallel copies** of the log viewer component family: `web-app/src/components/logs/*` and `web-app/src/components/shared/*` (`LogViewer.tsx`, `LogRow.tsx`, `VirtualLogList.tsx`, `ExpandedLogDetail.tsx`, `LogViewerToolbar.tsx` all exist in both directories). `SessionLogsTab.tsx` imports from `shared/`; `app/logs/page.tsx` imports from `logs/`. Worth a decision (probably in the planning phase, not this research) on which is canonical before building a third read-only viewer variant on top of either.

## 4. Status/timeline summary component patterns already in the codebase

- **`Badge` (`web-app/src/components/ui/Badge.tsx` + `.css.ts`)** — thin wrapper around a vanilla-extract `recipe()` with `intent`/`size` variants. This is the existing idiom for status pills; a new "lifecycle state" badge should extend this recipe's variants rather than inventing a new component.
- **`Skeleton.tsx`** — loading-state placeholder, reusable for any new collapsed/lazy section.
- **No existing Timeline/StatusHistory/Stepper component.** The "Workflow status-history timeline" and "Progress History" sections in `BacklogItemDetail.tsx` (section titles `Workflow` at line 1478, `Progress History` at line 1501) are custom-rendered inline, not backed by a shared timeline primitive. A new shared `Timeline`/`StatusHistory` component (vanilla-extract, no new library needed — this is straightforward flex/grid layout, not a case for a charting or animation library) would be a good consolidation target per the requirements' "reduce duplication" success metric, since Workflow status-history and Progress History likely show overlapping data today.
- `web-app/src/components/workflows/WorkflowsPanel.tsx` is a separate, unrelated feature (workflow *definitions*, not backlog item status) — not directly reusable but worth a glance for any existing status-list rendering idiom during planning.

## 5. Summary recommendations for planning phase

1. **Accordion/collapsible**: add `@radix-ui/react-accordion` (matches existing `^1.x` Radix family already used for Dialog/Tabs/Tooltip). Build `web-app/src/components/ui/Accordion.tsx` + `.css.ts` following the `Modal.tsx`/`Modal.css.ts` precedent for wrapping a Radix primitive in vanilla-extract.
2. **Tabs**: `@radix-ui/react-tabs` is already installed but has zero usage — decide during planning whether progressive disclosure is accordion-based (stacked, one panel at a time or multi-open) or tab-based (lifecycle phases as tabs); either is available at effectively the same integration cost.
3. **Read-only session viewer — two distinct cases, not one component**:
   - Real sessions (triage, non-headless review): build a `ReadOnlyTerminalViewer` — reuse `XtermTerminal`'s scrollback-consuming half of `useTerminalStream.ts`, strip the input-forwarding side.
   - Synthetic headless/blocked sessions (`headless-review-*`, `review-blocked-*`): **not a terminal problem at all** — needs a new read path into whatever persists the headless call's prompt/response (backend/data research agent should trace this before frontend work starts). Do not build one viewer component assuming both cases share a data source.
4. **Log viewer reuse**: the existing `logs/` or `shared/` `LogViewer` family (virtualized, search/filter/export already built) is a strong base to model the new read-only viewer's UX on (or literally reuse, if headless-call output can be shaped into the same `LogEntry` structure) — but resolve the `logs/` vs `shared/` duplication first.
5. **Status summary**: no new library needed; extend `Badge`'s vanilla-extract recipe for lifecycle-state variants, and build a new shared `Timeline`/`StatusHistory` component (plain flex/grid, vanilla-extract) to consolidate the Workflow/Progress-History duplication called out in the requirements.
