# Feature Landscape Research: modal-focus-trap

Agent 2 (Features) — repo `web-app/src` paths below are relative to repo root.

## 1. `useFocusTrap` hook — current implementation

`web-app/src/lib/hooks/useFocusTrap.ts` (64 lines):

```ts
export function useFocusTrap(
  ref: AnyElementRef,
  isActive: boolean,
  triggerRef?: AnyElementRef
)
```

Behavior, exact:
- Single `useEffect` keyed on `[isActive, ref, triggerRef]`.
- Bails out (`return`) if `!isActive || !ref.current`.
- **Computes the focusable-element snapshot exactly once per activation**, outside the
  `handleKeyDown` closure (lines 26-31): `Array.from(container.querySelectorAll(FOCUSABLE_SELECTORS)).filter(el => !el.closest("[aria-hidden='true']"))`.
  `first`/`last` are captured once and reused for the lifetime of the effect. This is the
  exact bug named in requirement #4 — if the container's focusable content changes after
  mount (e.g. an async list renders new buttons), `first`/`last` go stale and Tab-wrapping
  breaks (wraps to an element that's no longer first/last, or misses newly-added focusable
  elements entirely).
- `FOCUSABLE_SELECTORS` = `'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'`.
- On activation, calls `first?.focus()` — moves focus into the container immediately (no
  "focus first *interactive*, else the container itself" fallback; if `focusable.length === 0`
  nothing gets focus and `first` is `undefined`).
- `handleKeyDown` listens on `document` (not the container) for `Tab`/`Shift+Tab` only —
  does not handle Escape (each modal keeps its own Escape handler separately).
  - Zero-focusable-elements case: `e.preventDefault()` unconditionally on every Tab — locks
    Tab entirely rather than leaving the container (matches "trap" semantics but means a
    modal with e.g. only disabled buttons cannot be Tabbed out of at all).
- Cleanup: removes the listener and calls `triggerEl?.focus()` — this is the **focus
  restoration to the triggering element on close** feature already built into the hook.
  Restoration is opt-in via the optional third `triggerRef` param; most call sites (below)
  don't pass one, so focus is not restored to the invoking element for those.

## 2. Existing adoption pattern (5 call sites)

Established shape, consistent across all 5:

```tsx
const modalRef = useRef<HTMLDivElement>(null);
useFocusTrap(modalRef, /* isActive */ true | isOpenBoolean, /* optional */ triggerRef);
// ...
<div ref={modalRef} role="dialog" aria-modal="true" ...>
```

| Component | `isActive` arg | `triggerRef` passed? |
|---|---|---|
| `ResumeSessionModal.tsx:24` | `true` (component only mounts while modal is open — conditional render by parent) | no |
| `WorkspaceSwitchModal.tsx:89` | `true` (same pattern) | no |
| `TagEditor.tsx:17` | `true` | no |
| `DebugMenu.tsx:74` | `isOpen` boolean prop (component stays mounted, hook itself gates on the flag) | no |
| `SessionActionsOverflow.tsx:153-160` | 8 separate `useFocusTrap` calls, one per nested dialog/menu, each keyed to its own `isXOpen` boolean, each with its own `dialogRef` | 3 of 8 pass a `triggerRef` (`restartDialogRef`→`restartTriggerRef`, `checkpointDialogRef`→`checkpointTriggerRef`, `clearConversationDialogRef`→`clearConversationTriggerRef`); the other 5 (overflow menu itself, delete/autonomous/steer/program-restart confirms) don't restore focus |

`SessionActionsOverflow.tsx` is the reference implementation for "many independent
dialogs in one component, each with its own activation flag" — directly relevant since
`GateVerdictBox.tsx` has the same shape (override form, reopen form, skip-confirm
alertdialog all coexist in one component).

Two adoption styles emerge:
- **Conditional-mount modals** (`Review­ChangesModal`, `BacklogFileBrowserModal`,
  `CommitPushModal`, `WorktreeDiffModal`, `VaguenessPromptModal`, the `BacklogQueueSection`
  import dialog): parent only renders the component while open → pass `true` for `isActive`.
- **Always-mounted, prop-gated modals** (`DebugMenu`, and the in-place alertdialog inside
  `GateVerdictBox`): component stays mounted, boolean prop/state gates visibility → pass
  the boolean through.

`GateVerdictBox`'s skip-confirm alertdialog is the always-mounted style: `GateVerdictBox`
itself never unmounts (it's the persistent gate-verdict section of the item detail view),
so `showSkipConfirm` must be threaded into `useFocusTrap` as `isActive`, matching the
`DebugMenu` pattern, not the "pass `true`" pattern.

## 3. Current state of the 7 target modals

### 3a. `web-app/src/components/backlog/ReviewChangesModal.tsx` (120 lines)
- `modalRef = useRef<HTMLDivElement>(null)` (line 30), attached to the outer dialog div.
- Mount-focus effect (lines 70-72): `useEffect(() => { modalRef.current?.focus(); }, [])` —
  focuses the **container** (`tabIndex={-1}` on the div, line 85), not the first focusable
  child. Runs once on mount regardless of async diff-fetch state.
- Escape handler (lines 59-68): separate `useEffect` adding a `window` (not `document`)
  listener with `{ capture: true }`, calls `e.stopPropagation(); onClose();`. This is
  intentionally decoupled from any Tab handling.
- **No Tab handling at all** — Tab/Shift+Tab currently escape into the backgrounded page.
- Portal-rendered via `createPortal(..., document.body)`, guarded by
  `typeof document === "undefined"` SSR check.
- Focusable content is dynamic: `DiffRenderer` (line 107) renders differently for
  `loading` / `fetchError` / loaded-diff states, and has its own "Retry"/refresh affordance
  (`onRefresh={fetchDiff}`) — the set of focusable elements inside the dialog can change
  after the initial `fetchDiff()` promise resolves (loading → error-with-retry-button, or
  loading → loaded). This is a direct instance of the "focusable content changes after
  mount" edge case requirement #4 exists to fix.
- No `triggerRef` — nothing in the calling code passes a reference to whatever button
  opened this modal, so today there is no restore-focus-on-close behavior to preserve or
  break.

### 3b. `web-app/src/components/backlog/BacklogFileBrowserModal.tsx` (113 lines)
- Structurally identical to `ReviewChangesModal.tsx`: same `modalRef` container-focus
  pattern (lines 61-63), same `window`+`capture` Escape handler (lines 50-59), same portal
  + SSR guard, no Tab handling.
- Focusable content is *more* volatile here: `FileTree` (async-loaded tree, files become
  focusable as they render) and `FileContentViewer` (content pane whose interactive
  elements depend on `selectedPath`, which changes via `onFileSelect={setSelectedPath}`
  every time the user clicks a different file in the tree). Every file click potentially
  changes what's focusable in the content pane — the single most dynamic modal in this set.
- Also fetches its own VCS status (`useSessionVcs`) independently, which populates
  `gitStatusMap` async and could change `FileTree`'s rendered item set/icons after mount.

### 3c. `web-app/src/components/backlog/VaguenessPromptModal.tsx` (93 lines)
- Already hand-rolls a **correct, working 2-element focus trap** (lines 24-45): two
  `useRef<HTMLButtonElement>` refs (`refineButtonRef`, `proceedButtonRef`), a
  `handleKeyDown` on the dialog div (React synthetic `onKeyDown`, not global listener) that
  Tab-wraps between exactly those two buttons.
- Explicitly **no Escape handling by design** — code comment: "No escape-key dismissal:
  the user must choose one of the two explicit options." Migrating to `useFocusTrap` must
  preserve this (the hook itself doesn't touch Escape, so this is safe by default — just
  don't add an Escape handler while doing the swap).
- Mount-focus effect (lines 28-30) focuses `refineButtonRef` (the *primary* button), not a
  generic "first focusable" — matches what `useFocusTrap`'s `first?.focus()` would do here
  since `refineButtonRef`'s button is the first focusable element in DOM order, so behavior
  is preserved by a straight swap.
- Static content — exactly 2 buttons, no async data, no edge case around changing
  focusable-element count. Simplest of the 7 to migrate.
- Portal-rendered, SSR-guarded, same as the others.

### 3d. `web-app/src/components/backlog/GateVerdictBox.tsx` (526 lines) — skip-confirm alertdialog
- **Not portal-rendered** — this is the one target in the set that's an **inline**
  `role="alertdialog"` (lines 484-514) rendered directly inside the persistent
  `<section>` that is `GateVerdictBox` itself, no `createPortal`, no full-page backdrop/
  overlay div. It's modal only in ARIA semantics (`aria-modal="true"`), not visually —
  the rest of the gate-verdict section content remains in normal document flow directly
  above/below it.
- Two refs already scoped to exactly this alertdialog's buttons: `cancelRef`,
  `confirmRef` (lines 107-108, focus targets at 500/507). No ref currently exists for the
  alertdialog *container div itself* — one must be added (e.g. `skipConfirmDialogRef`) for
  `useFocusTrap`'s first argument, since the hook needs a container ref to query
  `querySelectorAll` against.
- Mount-focus effect (lines 131-135): focuses `cancelRef` (not "first focusable" — an
  explicit "default to Cancel, not the destructive Confirm" UX safeguard). `useFocusTrap`'s
  `first?.focus()` would coincidentally match here too, since Cancel (line 500) is before
  Confirm (line 507) in DOM order — but this is fragile: if a future edit reorders the
  buttons or the destructive button moves first, the swap to `useFocusTrap` would silently
  auto-focus the destructive action. Flag for the plan phase: verify DOM order is preserved,
  or keep an explicit `cancelRef.current?.focus()` alongside adopting `useFocusTrap` only
  for the Tab-wrap portion.
- Hand-rolled Tab-wrap logic in `handleSkipConfirmKeyDown` (lines 221-239) is bytewise
  identical in structure to `useFocusTrap`'s own Tab-wrap logic (same `focusables` array,
  same first/last, same shift-vs-not branch) — this is the closest existing hand-roll to
  what the hook already does, confirming the hook is a drop-in replacement for this case.
- Escape handling (lines 215-220) closes the alertdialog and refocuses `skipLinkRef` (the
  button that opened it) — this **is** an existing correct implementation of
  "restore focus to the trigger on close," achieved manually rather than via
  `useFocusTrap`'s `triggerRef` param. When migrating, `skipLinkRef` should be passed as
  `useFocusTrap`'s `triggerRef` argument so the hook's own cleanup-time
  `triggerEl?.focus()` subsumes this — but the existing `handleSkipConfirmKeyDown`'s
  Escape branch will still be needed standalone (the hook doesn't manage Escape), and care
  must be taken not to double-fire the refocus (once from the Escape handler's own
  `skipLinkRef.current?.focus()`, once again from the hook's effect cleanup running
  because `isActive` flips to `false`) — should be idempotent (focusing an already-focused
  element is a no-op) but worth a test.
- `GateVerdictBox` also has a second hand-rolled trap-*like* handler,
  `handleOverrideFormKeyDown` (lines 242-248) — Escape-only, no Tab-wrap, for the override
  reason form. This one is **not** in the requirements' explicit list of targets (only the
  skip-confirm alertdialog is named) — flag as a possible scope question: the override
  form has `role` unclear (need to check — not confirmed `aria-modal` bearing) and its own
  mount-focus effect (lines 111-118) focusing the `#override-reason` textarea, but no
  Tab-wrap at all currently. Confirmed via earlier grep it does **not** carry
  `aria-modal="true"` (only the skip-confirm alertdialog and the outer section's
  `role="status"` do) — so per the requirements' own scoping rule ("audit every other
  backlog-scoped modal with `aria-modal=\"true\"`"), the override form is out of scope
  since it isn't `aria-modal` at all. Confirmed not a gap versus requirements — just noting
  it exists nearby and reviewers may ask why it wasn't touched too.

### 3e. `web-app/src/components/unfinished/CommitPushModal.tsx` (118 lines)
- `textareaRef` mount-focus effect (lines 28-30) focuses the commit-message textarea
  directly (not the container) — matches "focus first focusable element," since the
  textarea is the first focusable element in the dialog.
- Escape + submit shortcut combined in one `handleKeyDown` (lines 64-67), attached as
  React `onKeyDown` on the outer overlay div (not a global listener) — only fires while
  focus is somewhere inside the dialog (relies on synthetic event bubbling up from
  descendants). **No Tab handling** — this is a real gap, Tab currently escapes the modal.
- Overlay click-outside-to-close (line 72): `onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}`.
- Static content (textarea + Cancel + Commit&Push buttons) — no async-loaded focusable
  elements, but the submit button's `disabled` state changes with `!message.trim() || loading`
  (line 107), meaning it toggles in and out of `FOCUSABLE_SELECTORS`'s
  `button:not([disabled])` match as the user types — another concrete instance of
  "focusable set changes after mount," this time driven by form validity rather than async
  data. Confirms requirement #4 isn't just an async-fetch concern.
- No container ref exists yet — one must be added for `useFocusTrap`'s first argument
  (currently only `textareaRef` exists, scoped to the textarea, not the dialog).
- Portal-rendered, no SSR guard visible (`createPortal` called unconditionally at line 117,
  unlike the other 6 which all guard with `typeof document === "undefined"`) — worth a note
  since it means this component would throw in a non-browser render context; out of scope
  for this bug fix but flag as an inconsistency the "audit" ask surfaced.

### 3f. `web-app/src/components/unfinished/WorktreeDiffModal.tsx` (94 lines)
- **No focus management of any kind** — no ref, no mount-focus effect, no `tabIndex`,
  nothing. The only keyboard-adjacent behavior is a Escape-to-close `document` listener
  (lines 53-57). This is the least-managed modal in the set — currently, opening this
  modal does not move focus into it at all (focus stays wherever it was on the
  page/whatever triggered the diff view).
- Async content: `fetchDiff` (lines 26-49) populates `content`/`added`/`removed` from an
  RPC call; `DiffRenderer` again renders loading/error/loaded states differently
  (same `onRefresh={fetchDiff}` retry affordance as `ReviewChangesModal`) — same
  "focusable set changes after async fetch" edge case.
- No container ref, no `tabIndex={-1}` on the dialog div — both need to be added.
- Click-outside-to-close on the overlay, same pattern as `CommitPushModal`.
- Portal-rendered, no SSR guard (same gap noted for `CommitPushModal`).

### 3g. `web-app/src/components/unfinished/BacklogQueueSection.tsx` — import dialog (lines 166-178)
- The overall component is a collapsible "Up Next" section (not itself a modal); the
  **import dialog** is a small portal (`role="dialog" aria-modal="true"`, line 170)
  rendered only while `showImport` is true, wrapping `<GitHubIssuePicker>`.
- **No focus management on the dialog wrapper at all** — no ref, no mount-focus effect, no
  Escape handler, no Tab handling declared in this file. All keyboard behavior currently
  comes entirely from the child `GitHubIssuePicker` component (see §4 below) — the wrapper
  itself is a bare `<div>` with no `tabIndex`.
- This is the one target where **the fix must be careful not to fight the child's own
  focus/keyboard management** — see §4.
- `handleKeyDown` on the section's collapsible header (lines 81-86) is unrelated Enter/Space
  toggle logic for the disclosure triangle, not dialog-scoped.
- Cancel path: `GitHubIssuePicker`'s `onCancel={() => setShowImport(false)}` (line 174) —
  no `triggerRef` currently exists to restore focus to the "+ Import GitHub Issue" button
  (line 132-143) on close; that button has no ref today.

## 4. `GitHubIssuePicker` — the nested focus-owning child (relevant to 3g)

`web-app/src/components/backlog/GitHubIssuePicker.tsx` is not itself in the requirements'
target list, but it's the sole interactive content of the `BacklogQueueSection` import
dialog, and it already does substantial focus management of its own:
- `searchRef` autoFocus-on-mount effect (lines 55-59).
- **Two-level Escape handling** (comment at line 62: "Two-level Escape: issue → repo →
  onCancel") — Escape first collapses a selected issue back to the repo list, then repo
  list back to search, only calling the dialog's `onCancel` on a third Escape. This is
  materially different from the other 6 modals' flat one-level Escape.
- A second, list-scoped `handleKeyDown` (line 133) on a search/list input plus `autoFocus`
  on list items (lines 166, 286) — this strongly suggests **arrow-key list navigation**
  inside the picker (issue/repo lists), which is a distinct keyboard-interaction layer
  from Tab-based focus trapping.
- Implication for the fix: wrapping the `BacklogQueueSection` import dialog in
  `useFocusTrap` is safe for Tab/Shift+Tab (arrow-key nav and Tab are different keys, no
  conflict), but the dialog's focusable-element set is **highly dynamic** — issues/repos
  load asynchronously and change the DOM repeatedly while the picker is open — making this
  the second-clearest instance (after `BacklogFileBrowserModal`) of why requirement #4
  (re-query focusable elements per Tab keypress, not once on activation) is load-bearing
  here, not just a theoretical nicety.
- No `triggerRef` wiring exists between `BacklogQueueSection` and `GitHubIssuePicker`
  today for restoring focus to the "+ Import GitHub Issue" button on cancel/complete.

## 5. Edge cases identified (cross-cutting)

1. **Focusable content changes after mount** — confirmed concrete instances in 4 of the 7
   targets, not hypothetical:
   - `ReviewChangesModal` / `WorktreeDiffModal`: `DiffRenderer` loading → error(+retry
     button) → loaded transitions.
   - `BacklogFileBrowserModal`: file-tree async load + `selectedPath` changes altering
     `FileContentViewer`'s interactive content on every click.
   - `CommitPushModal`: submit button's `disabled` attribute toggles with textarea
     validity, moving it in/out of the `:not([disabled])` focusable selector.
   - `BacklogQueueSection` import dialog: `GitHubIssuePicker`'s async issue/repo lists.
   This is exactly the hook bug requirement #4 asks to fix — a single-snapshot `first`/
   `last` will silently stop matching reality in all four cases above.

2. **Zero focusable elements** — no target modal currently has a genuinely empty state
   (all render at least a Cancel/Close button), but `DiffRenderer`'s loading state
   renders before any retry button exists — worth a unit test asserting Tab is a no-op
   (not a crash) when `focusable.length === 0`, since the hook's current behavior
   (`e.preventDefault()` unconditionally) has never been exercised by any of the 5
   existing call sites' tests.

3. **Nested/simultaneous modals** — `SessionActionsOverflow.tsx` already proves the hook
   supports multiple independent `useFocusTrap` instances active in the same component
   tree (8 calls, each keyed to its own boolean). None of the 7 target modals in this
   project can currently be open simultaneously with another target modal (each is gated
   by its own parent state and typically full-screen/exclusive), **except** the
   `GateVerdictBox` skip-confirm alertdialog, which lives inside a `GateVerdictBox` that
   could in principle be rendered alongside other page content — but no evidence found of
   two target modals stacking. Not a blocking edge case for this project, but the
   `SessionActionsOverflow` pattern is the template if it ever needs to be.

4. **Portal vs. inline** — 6 of 7 targets are portal-rendered to `document.body`
   (`ReviewChangesModal`, `BacklogFileBrowserModal`, `VaguenessPromptModal`,
   `CommitPushModal`, `WorktreeDiffModal`, `BacklogQueueSection` import dialog). The
   7th — `GateVerdictBox`'s skip-confirm alertdialog — is **inline**, not portaled, no
   backdrop. `useFocusTrap` itself is agnostic to portal-vs-inline (it only needs a DOM
   ref, which is valid either way), so no hook change is needed, but it's a fact worth
   stating plainly for the plan/implementation phase so nobody assumes a backdrop needs
   to be added as part of "fixing" the alertdialog (out of scope per requirements: "Making
   the rest of the page inert/aria-hidden" is explicitly excluded).

5. **SSR guard inconsistency** — `CommitPushModal` and `WorktreeDiffModal` call
   `createPortal` unconditionally with no `typeof document === "undefined"` guard, unlike
   the other 5 targets/call-sites. Not caused by and not required to be fixed by this
   project, but flag it since touching these two files for the focus-trap fix is an
   opportunity a reviewer may ask about; recommend leaving it unless the plan phase wants
   to fold it in as a one-line drive-by fix.

## 6. Unstated user needs / gaps versus what `useFocusTrap` already offers

- **Focus restoration to the triggering element on close**: the hook already supports
  this via the optional `triggerRef` param (confirmed working, cleanup-time
  `triggerEl?.focus()`), but **none of the 7 target modals currently pass a triggerRef
  today** (all either do nothing on close or, in `GateVerdictBox`'s case, hand-roll it via
  an explicit `skipLinkRef.current?.focus()` in the Escape handler). The requirements
  don't explicitly ask for restore-on-close as an acceptance criterion, but 3 of the 5
  *existing* `useFocusTrap` adopters (`SessionActionsOverflow`'s restart/checkpoint/
  clear-conversation dialogs) do use it, establishing it as a codebase norm for
  confirmation-style dialogs. Recommend the plan phase decide explicitly whether to wire
  `triggerRef` for all 7 (most impactful for `GateVerdictBox`'s skip-confirm, where the
  behavior already exists by hand) or leave it out-of-scope-but-noted, since silently
  adding it changes visible behavior (whatever currently retains browser focus after
  these modals close, e.g. `document.body` or the last-focused-before-open element by
  default, would change to a specific button) beyond "trap Tab, don't change anything
  else" — worth flagging as a scope decision rather than assuming.
- **Screen-reader announcement**: none of the 7 modals use `aria-live` for open/close
  announcements (aside from `GateVerdictBox`'s unrelated `role="status" aria-live="polite"`
  on the whole persistent section, not the alertdialog). `role="dialog"`/`"alertdialog"` +
  `aria-modal="true"` + `aria-labelledby`/`aria-label` (all 7 have one of the latter two)
  is the existing, sufixxent baseline pattern across the codebase — no target modal is
  missing labelling, so no additional screen-reader work appears needed beyond the focus
  trap itself.
- **Background scroll/interaction locking**: none of the 7 modals (nor the 5 existing
  `useFocusTrap` adopters) lock `document.body` scroll while open (no `overflow: hidden`
  toggle found). This matches the requirements' explicit "out of scope" note ("Making the
  rest of the page inert/aria-hidden") — confirmed as a non-goal, not an oversight to flag.
