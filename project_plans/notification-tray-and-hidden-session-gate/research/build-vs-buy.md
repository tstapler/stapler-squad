# Research: Build vs. Buy — notification-tray-and-hidden-session-gate

**Date**: 2026-10-07. Labels: VERIFIED = command run / file opened this session; INFERRED = reasoning, not checked against the primary source.

Prior art: `project_plans/notification-revamp/research/build-vs-buy.md` verdict ("extend what exists, reuse in-repo components, no SaaS, reuse the battle-tested path rather than bespoke logic") carries over unchanged in direction.

## Verified facts

- Already in `web-app/package.json` (VERIFIED): `@radix-ui/react-dialog ^1.1.15`, `react-accordion`, `react-tabs`, `react-tooltip`, `react-slot`, `@dnd-kit/core ^6.3.1`, `@tanstack/react-virtual ^3.13.25`, `react-virtuoso ^4.18.7`, `react ^19`, `next 15.3.2`, vanilla-extract. Not present: sonner, vaul, react-hot-toast, react-toastify, @use-gesture, react-swipeable, motion/framer-motion.
- `Modal.tsx` already wraps `@radix-ui/react-dialog` (VERIFIED, `web-app/src/components/ui/Modal.tsx:2`).
- In-repo touch handling exists: `web-app/src/lib/window/useWindowSwipe.ts` (horizontal pane swipe, 30px edge-avoidance, 60px/300ms thresholds, axis classification) and `web-app/src/lib/terminal/gestureMachine.ts` / `useTerminalGestures.ts` (VERIFIED by file listing and header read of useWindowSwipe).
- Existing UI: `NotificationToast.tsx` (249 lines), `NotificationPanel.tsx` (286 lines), `NotificationContext.tsx` (532 lines) (VERIFIED `wc -l`).
- npm registry (VERIFIED via `npm view`, 2026-10-07):

| Package | Version | License | Last modified | React 19 peer | Unpacked size |
|---|---|---|---|---|---|
| sonner | 2.0.8 | MIT | 2026-08-09 | yes | 174 KB |
| react-hot-toast | 2.6.1 | MIT | 2026-09-16 | `>=16` | 199 KB |
| react-toastify | 11.1.0 | MIT | 2026-04-19 | yes | 565 KB |
| @radix-ui/react-toast | 1.2.24 | MIT | 2026-10-08 | yes | 188 KB |
| vaul | 1.1.2 | MIT | 2024-12-14 | yes | 184 KB |
| react-modal-sheet | 5.6.0 | MIT | 2026-03-27 | yes, but peer `motion >=11` | 503 KB |
| @use-gesture/react | 10.3.1 | MIT | 2024-03-21 | `>=16.8` | 37 KB |
| react-swipeable | 7.0.2 | MIT | 2026-07-20 | yes | 87 KB |

  Unpacked size is not gzipped bundle size; treat as an upper bound only. Bundle-size and "maintained" judgments beyond last-publish date are INFERRED. I did not open the libraries' repos/issue trackers.

## 1. OSS libraries

### Toast stacking

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **sonner** | Built-in stacked/collapsed toasts with visible-count cap (`visibleToasts`), swipe-to-dismiss, pause on hover, React 19 peer OK, actively published (INFERRED from feature set; props not re-verified) | Owns its own toast queue and DOM, so it would duplicate `NotificationContext` state (the history/read/dismissed source of truth) and need a bridge; styling is its own CSS (vanilla-extract overrides are awkward); no concept of "pinned approval toast", "+N more chip opens the tray", or "never auto-clear decision items" — all required here; renders toasts in a portal with its own swipe handlers that could fight terminal touch handling | Viable, not recommended |
| react-hot-toast | Small, headless-capable | No stacking/collapse/cap semantics we need; same dual-state problem | Not recommended |
| react-toastify | Mature, rich | Largest (565 KB unpacked), own queue, `limit` option only caps; heavy for this | Not recommended |
| @radix-ui/react-toast | Same Radix family already in use; accessible live regions, swipe-to-dismiss, F8 hotkey region | Radix Toast has no stack/collapse/cap; we would still own queue logic; adds a package for ~what we have | Viable, marginal |
| **Evolve in-repo `NotificationToast` + `NotificationContext`** | One source of truth already exists and drives toasts, history, cross-tab sync; the new requirements (cap 3, "+N more", dismiss all via existing `clearAll()`, pinned decision toasts, demote routine to tray) are *policy over the existing list*, which no library models | We own the animation/timer code (small) | **Recommended** |

Rationale: the hard parts of this feature are policy (cap, pin, demote, never bulk-clear decisions, mobile offset above soft keyboard/bottom nav), not rendering. Libraries solve rendering and force a state bridge. (INFERRED from requirements.md L99-106 versus library feature sets.)

### Drawer / sheet / tray

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **`@radix-ui/react-dialog` (already a dependency)** | Focus trap, Escape, `aria-modal`, scroll lock, portal, labelled dialog — exactly the "focus-managed dialog" NFR; already wrapped by `Modal.tsx`; side-anchored via CSS | Modal mode steals focus and marks the rest inert, including the xterm terminal, and locks scroll; for a tray that must not disturb the terminal use `modal={false}` plus `onOpenAutoFocus`/`onInteractOutside` handling, or keep it a non-modal `role="complementary"` landmark | **Recommended** as the focus/ARIA primitive, configured non-modal (verify with a spike: terminal keeps focus and DOM identity) |
| vaul | Good bottom-sheet drag UX | Bottom-sheet oriented (we want a side tray), last published 2024-12 (verified date; maintenance status otherwise INFERRED), built on Radix Dialog already, adds a layer | Not recommended |
| react-modal-sheet | Bottom sheet with snap points | Requires adding `motion` (new heavy dep), bottom-sheet only, 503 KB unpacked | Not recommended |
| Pure custom CSS tray with no primitive | Zero deps | Reimplements focus management/ARIA | Not recommended for the dialog semantics; fine for layout |

### Gesture handling

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **Custom pointer-event swipe hook, modeled on `useWindowSwipe.ts`** | Repo already has tested axis classification, edge avoidance and thresholds for exactly the conflict named in Rabbit Holes (pane swipe vs. terminal touch); swipe-to-dismiss on a single row is one axis, ~40-80 lines; reuse `windowGestureConstants` | We own edge cases (see section 3) | **Recommended**, scoped to the toast/tray row elements only (never attached to the terminal) |
| @use-gesture/react | Best-in-class drag with velocity/axis lock/`touch-action` handling; tiny (37 KB) | Last release 2024-03 (verified); `useDrag` attaches its own listeners and `touch-action` conventions that must be reconciled with the existing terminal gesture machine | Viable (fallback if the custom hook proves flaky) |
| react-swipeable | Simple swipe callbacks, published 2026-07 | Swipe *detection* only (no drag-follow animation); overlaps `useWindowSwipe` | Viable, redundant |
| @dnd-kit (already present) | Already a dependency | Reorder/DnD semantics, wrong tool for dismiss | Not recommended |

## 2. SaaS notification inbox (Knock, Novu, Courier)

**Not recommended.** Same reasoning as the prior build-vs-buy §2 (INFERRED; vendor docs not re-read):
- Single operator, self-hosted tool over Tailscale/LAN; those products solve multi-user routing, preferences, and templating.
- The bug is in-process delivery gating (hidden-session policy) plus client UX; none of it is fixed by an external inbox. Hidden-session events would still need the same gate before being sent out.
- Adds an external network dependency, API keys and a data-egress path for internal session data; conflicts with JSON-backed history and live-settable local flags.

## 3. LLM-generated bespoke vs. battle-tested library

Rule of thumb applied: go custom only when (a) the logic is policy specific to this app, (b) the repo already has tested equivalents, or (c) the library would own state we must own. Go library when the logic is a well-known tricky primitive with many browser quirks.

| Concern | Choice | Why |
|---|---|---|
| Focus trap / dialog ARIA / Escape / inert | **Library** (existing Radix Dialog) | Classic quirk-heavy primitive; already a dependency; hand-rolled traps routinely break with portals, iOS, and screen readers. For a non-modal tray, a trap is not wanted anyway — verify what Radix non-modal mode does to xterm focus |
| Swipe-to-dismiss | **Custom, extending `useWindowSwipe` patterns** (fallback `@use-gesture/react`) | Single axis, single row; repo precedent and thresholds exist; key risks are `touch-action: pan-y`, pointercancel, scroll-vs-swipe disambiguation, and not capturing events on the terminal. Needs real-device or Playwright touch-emulation tests, not just unit tests |
| Toast queue/timer logic (cap, pause on hover/focus, pinned vs. auto-expire) | **Custom, in `NotificationContext`** | Policy-heavy and must share state with history/read/dismissed and cross-tab sync. Pure reducer functions are cheap to unit-test deterministically (fake timers). Pitfalls to test: timer pause/resume accounting, dedup re-trigger, StrictMode double effects |
| Virtualized tray list | **Library** (existing `@tanstack/react-virtual` / `react-virtuoso`) | Already present; hundreds of rows per NFR |

LLM-authored code is acceptable for the custom parts only with deterministic tests and a touch-emulation e2e; do not hand-roll the dialog/focus primitive.

## 4. Fork/adapt vs. new

### Frontend

**Recommended: evolve `NotificationPanel` / `NotificationToast` / `NotificationContext`** (matches requirements Alternatives Considered). Add: visible-cap and demotion policy in the context; "+N more" chip and Dismiss-all in the toast stack (wire existing `clearAll()`); edge handle + non-modal overlay for the panel; grouping via existing `Collapsible.tsx` (`CollapsibleGroup`/`CollapsibleSection`, from prior research). Caution: these files total ~1,070 lines already (VERIFIED wc), so extract the stack policy into a pure module rather than growing `NotificationContext.tsx` further. A new component only for the edge handle/tray shell if `NotificationPanel` is tied to the header-bell sidebar layout (not verified — read its layout during planning).

### Backend gate

**Recommended: adapt the existing pattern, but consolidate into one shared function, not per-path copies.**
- Existing hidden policy exists in two places (VERIFIED): `server/review_queue_manager.go:436-449` (`suppressForHidden`: hidden AND reason in TaskComplete/Idle/Stale, so ErrorState/TestsFailing still publish) and `server/services/notification_service.go:132-146` (hidden AND priority LOW). Hidden is read via `inst.Snapshot().Hidden` there (line 106), consistent with the Snapshot rule.
- `server/push` has no Hidden check (VERIFIED: grep of `Hidden` in `server/push/subscriber.go` returned nothing), so this is the bug.
- The two existing predicates are different (reason-based vs. priority-based), which is evidence the per-path approach already diverges. A new small package (e.g. a `HiddenGate` with `Allow(event) (bool, reason)` plus slog + counter) that both existing sites and the EventBus-level subscribers call would implement the "single choke point" requirement. A new *package* is justified only for the shared predicate and metrics; reuse the existing semantics (failure/needs-human pass) as its rules rather than inventing new ones. Choke point placement (EventBus subscriber wrapper vs. producer-side) is a planning decision that needs the producer inventory (Open Question 1), which this research did not do.
- Read-only hidden-session view: reuse `ListSessions`' `IncludeHidden` (exists, `server/services/session_service_crud.go:61` per requirements) rather than a new RPC; add a server-side input guard (not client-only), per the MCP-surface lesson.

## Summary

| Decision | Verdict |
|---|---|
| Toast library (sonner et al.) | Viable (sonner), **not recommended** — dual state, no pin/demote policy |
| Tray primitive | **Recommended: existing `@radix-ui/react-dialog`**, non-modal, spike for xterm focus |
| vaul / react-modal-sheet | Not recommended |
| Gestures | **Recommended: custom hook modeled on `useWindowSwipe`**; `@use-gesture/react` as fallback |
| SaaS inbox | Not recommended |
| Frontend fork/new | **Evolve existing**; extract stack policy to a pure module |
| Backend gate | **Adapt existing predicates into one shared gate function** with metrics; not per-channel copies |

New dependencies recommended: **zero**.

## Gaps

- Did not open library repos/issues; maintenance and feature claims for sonner/vaul/@use-gesture are INFERRED from npm metadata and prior knowledge.
- Radix Dialog `modal={false}` behavior with xterm focus is unverified; needs a spike.
- Producer inventory for the gate choke point not done here.
