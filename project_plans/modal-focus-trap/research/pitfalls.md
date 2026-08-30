# Pitfalls Research: modal-focus-trap

Sources: `web-app/src/lib/hooks/useFocusTrap.ts` (read in full), the 7 target
modal components, existing adopters (`ResumeSessionModal.tsx`,
`WorkspaceSwitchModal.tsx`), and `tests/e2e/accessibility.spec.ts`.

## 1. Latent bugs in the current `useFocusTrap.ts`

File: `web-app/src/lib/hooks/useFocusTrap.ts:22-63`

- **Stale focusable-element snapshot (the bug AC #4 exists to fix).** `focusable`,
  `first`, `last` are computed once inside the effect body and closed over by
  `handleKeyDown` (lines 26-31). If the modal's content changes after
  activation — a tab switches, an accordion expands, an async section finishes
  loading and reveals new buttons — the trap keeps cycling between the
  *original* first/last elements. Newly-revealed focusable elements become
  reachable by mouse but are skipped by Tab, and elements removed from the DOM
  after activation leave `first`/`last` pointing at detached nodes (`.focus()`
  on a detached node is a silent no-op, so Tab does nothing at the trap edge).
  Fix must re-query `container.querySelectorAll(...)` inside `handleKeyDown`
  itself, not just in the effect setup.
- **Zero-focusable-element containers get no stable focus target.** When
  `focusable.length === 0`, `first`/`last` are `undefined` and `first?.focus()`
  no-ops — focus is left wherever it was before activation (typically still on
  the trigger button in the background). Tab is correctly blocked
  (`e.preventDefault()` at line 39), so focus can't escape further, but it was
  never actually moved *into* the dialog, which is itself a WCAG miss for a
  transient empty/loading state. `BacklogFileBrowserModal` (loading/error
  states before entries render) is the most likely of the 7 targets to hit
  this. Consider focusing the container itself (give it `tabIndex={-1}`) as a
  fallback when `focusable.length === 0`.
- **`aria-hidden` filter doesn't catch `display:none`/`visibility:hidden`.**
  Line 28 filters out elements inside `[aria-hidden="true"]` ancestors, but
  does nothing for elements that are simply hidden via CSS (a collapsed
  `display:none` panel, an off-screen clone). `querySelectorAll` matches DOM
  presence, not visibility, so such elements can end up in the tab cycle and
  swallow a Tab press without any visible focus change.
- **Unconditional `triggerEl?.focus()` on cleanup (line 61).** This runs on
  *every* cleanup — including a plain unmount for reasons other than "user
  closed the dialog" (e.g. React remounting the tree on a `key` change, or a
  parent re-render that briefly unmounts/remounts). It also runs when
  `isActive` flips `false` while the component stays mounted. If some other
  effect has already moved focus for a legitimate reason by the time cleanup
  runs, this silently steals it back. Not a new bug to introduce, but the 7
  target migrations must not paper over it by, e.g., wrapping modal content in
  a way that causes extra unmount/remount cycles (see React section below).
- **No dependency on DOM mutations, only `[isActive, ref, triggerRef]`
  (line 63).** Combined with the stale-snapshot bug above, this means the
  effect never re-runs just because modal content changed — only a genuine
  `isActive` flip re-arms it. Any fix that re-queries on each Tab keypress
  sidesteps this without needing a `MutationObserver`.

## 2. Hand-rolled focus-trap pitfalls, mapped to this repo's actual code

- **Double-handling Tab during migration — concretely reproducible in
  `GateVerdictBox.tsx`.** Its skip-confirm `alertdialog` (`GateVerdictBox.tsx:483-491`)
  already has a hand-rolled Tab trap in `handleSkipConfirmKeyDown`
  (`GateVerdictBox.tsx:215-240`), wired via React's synthetic `onKeyDown` on
  the dialog div. If `useFocusTrap` is added *alongside* this instead of
  *replacing* it, both handlers fire on every Tab press: the synthetic
  handler moves focus and calls `preventDefault()` first, then bubbles to
  `useFocusTrap`'s `document`-level listener, which computed its own
  first/last from a stale snapshot and may immediately move focus again based
  on an `activeElement` check that's now out of date because the first
  handler already changed it. Net effect: a Tab press that jumps focus twice,
  sometimes landing outside the intended cycle. **The
  `handleSkipConfirmKeyDown` Tab-handling branch (lines 221-239) must be
  deleted, not left in place, when this dialog is wired onto `useFocusTrap`**
  — only the `Escape` branch (lines 216-220) should remain.
- **Focus-restoration races when multiple effects call `.focus()` on
  activation.** `ResumeSessionModal.tsx` is the existing template for this
  exact hazard: it calls `useFocusTrap(modalRef, true)` (line 24) *and* has a
  separate effect that calls `titleInputRef.current?.focus()` (line 46).
  Because React runs effects in declaration order, the second effect wins and
  overrides whatever `useFocusTrap` focused first — currently harmless only
  because `titleInputRef` happens to be the first focusable element anyway.
  Any of the 7 target modals that already have their own "focus the first
  input on open" effect (check each for a `.focus()` call outside
  `useFocusTrap` before wiring it up) will have this same race; the two
  effects must agree on what should end up focused, or the manual one should
  be deleted in favor of `useFocusTrap`'s own initial-focus behavior.
- **Elements matching the selector but excluded by `aria-hidden` ancestor
  checks are a known gap in naive implementations** — this hook already
  handles the ancestor case (line 28) but not self-hidden or `display:none`
  elements (see section 1). Worth a regression test per target modal that has
  any conditionally-hidden content inside the dialog.

## 3. Why axe-core/CI accessibility scans cannot catch this bug, and what does

Static/DOM-snapshot scanners (axe-core, and this repo's Lighthouse/Axe CI gate
per `tests/e2e/accessibility.spec.ts`) inspect a single rendered DOM tree for
rule violations — missing labels, contrast ratios, ARIA misuse, missing
`role`s. **A keyboard focus trap is a behavioral property over a sequence of
key events, not a static DOM property.** `role="dialog" aria-modal="true"` is
present and structurally valid whether or not Tab is actually intercepted —
axe-core has no rule that presses Tab repeatedly and asserts focus stays
inside the subtree, because doing so requires simulating real input events and
observing `document.activeElement` across the interaction, which is out of
scope for a static ruleset. This is exactly why the bug shipped in the first
place: `ReviewChangesModal.tsx` and `BacklogFileBrowserModal.tsx` already
declare `role="dialog" aria-modal="true"` (correct ARIA) yet still leak focus
— an axe pass gives no signal either way.

The only technique that actually catches it is a **real, sequential
`page.keyboard.press('Tab')` loop in a Playwright e2e test**, asserting that
`document.activeElement` cycles within the dialog and never lands on a
background element. This repo already has the right *pattern* — just not yet
applied to a trap-loop assertion:

`tests/e2e/accessibility.spec.ts:259-274` (test: "Reload and dismiss controls
on the buffered-update banner are keyboard-reachable with a visible focus
indicator (UX AC #32)") drives focus with real `page.keyboard.press('Tab')`
calls in a loop, explicitly because `.focus()` calls don't trigger Chromium's
`:focus-visible` state the way a real keyboard Tab does (see the comment at
lines 259-264, citing `globals.css:194`'s
`:focus:not(:focus-visible) { outline: none }`). That test only checks
*reachability* (does N tabs eventually reach a target button) — it does not
assert *containment* (that continuing to Tab past the last element wraps back
to the first, or that the modal opener never regains focus while the dialog
is open). The new e2e tests for `ReviewChangesModal` and
`BacklogFileBrowserModal` need the containment assertion this existing test
doesn't make: Tab from the last focusable element and assert focus lands on
the first (not on a background element or `document.body`), and Shift+Tab
from the first lands on the last.

## 4. React-specific pitfalls

- **Effect cleanup ordering: unmount vs. `isActive` flipping false.** Both
  paths run the *same* cleanup function (`useFocusTrap.ts:58-62`), so there is
  no special-casing today — cleanup always removes the listener and always
  refocuses the trigger. That's consistent, but it means a component that
  conditionally renders the modal (`{isOpen && <Modal/>}`, causing full
  unmount) and one that renders it persistently with `isActive={isOpen}`
  (like `WorkspaceSwitchModal.tsx:89`, which passes a static `true` — meaning
  it must be the unmount-on-close style, not toggle-on-same-instance) behave
  identically from the hook's point of view, but only if each of the 7 target
  modals picks *one* of these two lifecycles consistently. Check each
  target's existing `{show && <Modal/>}` vs. `isActive` prop usage before
  wiring — mixing "modal stays mounted, `isActive` toggles" with a
  `key`-based remount elsewhere in the same tree would fire cleanup/refocus
  twice for one logical close.
- **`GateVerdictBox`'s two nested dialogs (skip-confirm alertdialog vs. the
  override form) toggle via sibling boolean state (`showSkipConfirm`,
  `showOverride`) in the same component**, not full unmount — this is the
  "isActive flips false while the component stays mounted" case, and the
  existing initial-focus effects (`GateVerdictBox.tsx:131-135` for
  `showSkipConfirm`) already race with whatever `useFocusTrap` would add, per
  section 2 above.
- **StrictMode double-invocation.** In dev, React 18 StrictMode mounts, cleans
  up, and remounts effects once, to surface cleanup bugs. For this hook, that
  means: effect runs → `first?.focus()` + listener attached → cleanup runs →
  listener removed + `triggerEl?.focus()` → effect runs again → `first?.focus()`
  + listener re-attached. Net behavior after the double-invocation settles is
  identical (listener count ends at 1, focus ends on `first`), so this hook is
  StrictMode-safe *as currently written* — the double-invocation is
  idempotent because cleanup fully reverses setup and setup doesn't depend on
  any external mutable state beyond what's recomputed fresh each run. The
  one thing to verify after the re-query-per-keypress fix (section 1) is that
  the new re-query logic is still purely a function of the current DOM at
  call time (no memoized/cached list that setup writes once) — if the fix
  introduces a `useRef` cache of the focusable list populated only in the
  effect body, StrictMode's double-run would populate it twice harmlessly,
  but a naive "populate only if empty" cache guard could get stuck with a
  stale list from the first (cleaned-up) invocation.

## 5. Portal-related pitfalls

All 7 target files already render via `createPortal(..., document.body)`,
confirmed by grep:

| File | createPortal | Existing Tab handling |
|---|---|---|
| `web-app/src/components/backlog/ReviewChangesModal.tsx:76` | yes | none (only `Escape`, `:60-67`) |
| `web-app/src/components/backlog/BacklogFileBrowserModal.tsx:67` | yes | none (only `Escape`, `:51-58`) |
| `web-app/src/components/backlog/VaguenessPromptModal.tsx:92` | yes | none |
| `web-app/src/components/backlog/GateVerdictBox.tsx` | **no** — this section renders inline, not portaled | hand-rolled Tab trap for the skip-confirm alertdialog (`:215-240`), see section 2 |
| `web-app/src/components/unfinished/CommitPushModal.tsx:117` | yes | none (only `Escape`, `:64-65`) |
| `web-app/src/components/unfinished/WorktreeDiffModal.tsx:93` | yes | none (only `Escape`, `:54`) |
| `web-app/src/components/unfinished/BacklogQueueSection.tsx:168` | yes | **none at all** — the import dialog has no ref, no Escape, no Tab handling today |

Implications:

- **`GateVerdictBox` is not portaled.** It's the one target that renders
  inline in the document flow, inside a `<section role="status">` wrapper
  (`GateVerdictBox.tsx:253-259`); the skip-confirm `alertdialog` is a plain
  DOM child of that section, not portaled. So there's no portal-timing
  concern for it, but it's also the one with real hand-rolled Tab logic to
  remove (section 2) — don't assume the portal caveats below apply to it, and
  don't assume the "just add `useFocusTrap`" recipe from the other 6 is a
  pure copy-paste for this one.
- **Ref-to-portaled-node timing is not actually a hazard here.** Because
  `createPortal` inserts its children into `document.body` synchronously
  during React's commit phase (same phase as ref assignment), a `ref` on the
  portaled dialog `div` is already non-null by the time `useEffect` runs,
  regardless of `document.body` being a different DOM subtree than the
  logical React parent. No extra `useLayoutEffect` or ref-ready check is
  needed for `useFocusTrap` to see a populated `ref.current` on activation.
- **`BacklogQueueSection`'s import dialog has no container ref today**
  (`BacklogQueueSection.tsx:170` — the dialog `div` has no `ref` prop at all).
  Wiring `useFocusTrap` here requires adding a new `useRef` and attaching it,
  not just calling the hook — a plain "add `useFocusTrap(existingRef, true)`"
  patch won't compile/won't work since there is no existing ref to reuse.
- **Scope creep risk on `BacklogQueueSection`.** Since this dialog currently
  has *no* Escape handling either, resist the temptation to add Escape support
  while wiring the focus trap — the requirements only ask for the trap fix
  (`useFocusTrap` reuse) and explicitly list "Escape unchanged" only for the
  two named modals; adding new Escape behavior elsewhere is an untested,
  unscoped change to this PR unless the plan phase explicitly decides to
  bundle it.
- **Multiple simultaneously-portaled dialogs share `document.body` as their
  DOM parent.** If two of these modals are ever open at once (unlikely by
  current UX flow, but not structurally prevented), `useFocusTrap`'s
  `document`-level `keydown` listener from the first-opened modal is still
  attached when the second opens, so both trap instances receive every Tab
  keypress and each independently tries to move focus based on its own
  (different) container's focusable list — this would need to be validated
  as out-of-scope-but-not-broken during implementation, since the requirements
  don't ask for a modal stack/z-index arbitration fix.
