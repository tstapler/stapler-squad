# UX / Accessibility Research: modal-focus-trap

## 1. WAI-ARIA APG "Dialog (Modal)" pattern — focus-management requirements

Source: well-established knowledge of the W3C WAI-ARIA Authoring Practices Guide dialog pattern (`https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/`). I did not fetch the live page for this research pass (no WebFetch/WebSearch call made) — the pattern has been stable for years, but if a citation-grade quote is needed for the plan/PR, verify against the current APG page directly.

The pattern's focus requirements, confidently recalled:

1. **On open**: focus moves to an element inside the dialog — either the first focusable element, a specific element chosen because it's the most likely/safest next action (e.g. not a destructive button), or the dialog container itself (`tabindex="-1"`) when no default focus target makes sense and the user should read content before acting.
2. **While open**: Tab/Shift+Tab must cycle only among the dialog's own focusable descendants. Focus must never reach browser chrome or background page content — this is the "focus trap" behavior itself.
3. **Escape**: closes the dialog, if the dialog is closable (some alertdialogs intentionally require an explicit action instead).
4. **On close**: focus returns to the element that triggered the dialog's opening, or a sensible alternative if that element no longer exists (e.g. was removed from the DOM by the same action that closed the dialog).
5. **Semantics**: the container carries `role="dialog"` (or `role="alertdialog"` for dialogs demanding immediate response) and `aria-modal="true"`, with an accessible name via `aria-labelledby` or `aria-label`.
6. **Background inertness**: content outside the dialog should be made inert to assistive tech while the dialog is open (via the `inert` attribute or `aria-hidden="true"` on siblings) — `aria-modal="true"` is a promise to AT that background content isn't reachable, and the DOM should back that promise up.

All six target modals implement #5 (`role="dialog"`/`alertdialog` + `aria-modal="true"`) but, per the requirements doc, several don't implement #2, and none of the six appear to implement #6 (see §5).

## 2. WCAG 2.1.2 vs 2.4.3 — which one is this bug actually about?

- **2.1.2 No Keyboard Trap** (Level A): "If keyboard focus can be moved to a component using a keyboard interface, then focus can be moved away from that component using only a keyboard interface." This is about a user getting **stuck** somewhere with no way out (classic case: an embedded widget that swallows Tab/Escape entirely). It does not require components to *contain* focus — it requires the opposite guarantee, that focus is never held hostage.
- **2.4.3 Focus Order** (Level A): Requires that when content can be navigated sequentially, focusable components receive focus in an order that preserves meaning and operability.

**The bug as described — Tab/Shift+Tab moving focus out of an open modal into the backgrounded page — is not a 2.1.2 violation.** Nothing traps the user in this scenario; quite the opposite, focus escapes too freely. Citing 2.1.2 in the backlog item title/description is a misnomer worth flagging in the plan or PR description.

The better-fitting citation is **2.4.3 Focus Order**: once a modal dialog is open, the "meaningful sequence" for a keyboard/screen-reader user is bounded by the dialog's content — background controls that are visually covered but still reachable via Tab produce a focus order that no longer corresponds to what's presented, which is exactly what 2.4.3 targets. There's also a secondary, weaker case for **4.1.2 Name, Role, Value**: `aria-modal="true"` asserts to assistive tech that content outside the dialog is inert, and if the DOM doesn't enforce that (no trap, no `aria-hidden`/`inert` on the rest of the page), the declared state contradicts actual behavior.

Net: recommend the plan/PR cite **2.4.3** as the primary WCAG success criterion this fixes, and either drop the 2.1.2 reference or explicitly note it as a correction from the original backlog item's phrasing. (Medium-high confidence — this is standard accessibility-audit framing, but I'd normally corroborate against the official WCAG "Understanding" pages for 2.1.2/2.4.3 via WebSearch/WebFetch before putting it in anything more formal than a research note.)

## 3. `useFocusTrap.ts` gap: zero-focusable-descendants case

Read: `web-app/src/lib/hooks/useFocusTrap.ts:22-63`.

Confirmed gap. The relevant lines:

```ts
const first = focusable[0];
const last = focusable[focusable.length - 1];
first?.focus();          // no-op when focusable.length === 0
```

If a dialog momentarily has **zero** focusable descendants (e.g. a pure-message confirmation dialog, or a loading state before buttons render), `first` is `undefined` and `first?.focus()` does nothing — focus is left wherever it was before the dialog opened, typically the trigger button in the background. Two consequences:

- A screen-reader user gets no announcement that a dialog opened (focus never moved into it), even though the Tab-key trap in `handleKeyDown` still fires and blocks Tab entirely (`if (focusable.length === 0) { e.preventDefault(); return; }`), so they're also stuck unable to reach anything with the keyboard at all.
- This is the common gap the task description anticipates: the hook should fall back to focusing **the container itself** when there are no focusable descendants, which requires the container to carry `tabindex="-1"` (the DOM only allows programmatic focus on non-interactive elements that have a `tabindex`).

Today, `ReviewChangesModal.tsx` and `BacklogFileBrowserModal.tsx` both already set `tabIndex={-1}` on their container **and** separately call `modalRef.current?.focus()` in their own `useEffect` (`ReviewChangesModal.tsx:70-72`, mirrored in `BacklogFileBrowserModal.tsx`) — i.e., they hand-rolled exactly the fallback `useFocusTrap` lacks, but unconditionally (not just as a zero-focusable fallback), so this existing container-focus call will race with `useFocusTrap`'s own `first?.focus()` once the hook is wired in. Recommend the plan either (a) push the "focus container if no focusables, else focus first" fallback logic into the hook itself and delete the two components' now-redundant manual container-focus effects, or (b) keep the container-focus effects but make sure hook-order guarantees the hook's `first.focus()` (when focusables exist) runs last and wins — option (a) is cleaner and matches the "reuse the shared hook" spirit of the ask.

## 4. Job-to-be-done (brief — this is a bug fix, not a new feature)

A screen-reader or keyboard-only user reviewing a backlog diff opens "View Changes," tabs through the diff/close controls, and — because nothing stops Tab at the dialog boundary — lands on background page controls (nav, other backlog cards) that are still visually hidden behind the modal. They lose their place, can't tell whether the dialog closed, and may act on a background control while believing they're still inside the review flow. The fix is entirely about restoring the expected containment; no new interaction model is being introduced.

## 5. Edge cases

**Focus restoration to the trigger element on close**: `useFocusTrap` already supports this via an optional `triggerRef` (`useFocusTrap.ts:57,61`: `triggerEl?.focus()` on cleanup). However, **none of the 5 existing consumers pass `triggerRef`** (`TagEditor.tsx`, `SessionActionsOverflow.tsx`, `WorkspaceSwitchModal.tsx`, `ResumeSessionModal.tsx`, `DebugMenu.tsx` all call `useFocusTrap(modalRef, true)` with no third argument). So today, closing any of those 5 modals does *not* return focus anywhere in particular — the user's next Tab starts from wherever the browser defaults to (often the top of the document). Users generally do expect focus to land back on the button/link that opened the dialog (this is APG requirement #4 above) — worth flagging as a **pre-existing gap wider than this ticket's scope**, but since `ReviewChangesModal`/`BacklogFileBrowserModal` are opened from clearly identifiable buttons (`BacklogItemDetail.tsx:1192` "View Changes", `:1227` "View Diff", `:1228` "Browse Files"), wiring `triggerRef` for at least the two newly-fixed modals is a low-cost, high-value addition worth including even though the requirements doc doesn't explicitly call it out — it's the same hook call and just needs a ref on the triggering button. Recommend flagging this as a suggested scope addition in the plan phase rather than assuming it's required.

**Double-trap risk from two modals open at once**: Checked `BacklogItemDetail.tsx:145-148,1129-1144,1192,1227-1228`. `showChangesModal` and `showFileBrowser` are independent `useState(false)` booleans, each set by a separate button (`onViewChanges`/`onViewDiff` → `showChangesModal`; `onBrowseFiles` → `showFileBrowser`), with no code path that clears one when the other opens. In principle both could be `true` simultaneously. In practice, `ReviewChangesModal` renders a full-screen backdrop (`backdrop` div with `onClick={onClose}`) that visually and pointer-wise covers the trigger buttons underneath, so a mouse user can't reach the second button while the first modal is open; a keyboard user is confined to the first modal once the trap lands (making the second button unreachable via Tab too). So this specific pair is **not practically reachable** as a double-trap today — no fix needed for these two. That said, there's no shared modal-stack/z-index registry in this codebase: if two `useFocusTrap` instances ever *were* active concurrently, both would attach independent `document`-level `keydown` listeners, and on a Tab keypress **both** handlers fire, each computing its own first/last from its own container and calling `preventDefault`/`focus` — the practical effect would be whichever handler is attached second "winning" the focus placement for that keypress, which is fragile. Worth a one-line note in the plan as a known architectural limitation, not something this narrowly-scoped ticket needs to solve.

## Summary of hand-rolled traps found (confirms requirements' audit list)

- `GateVerdictBox.tsx:216-243` — hand-rolled Tab cycling hardcoded to exactly two refs (`cancelRef`, `confirmRef`); doesn't re-query the DOM, so it silently misses any additional focusable elements (e.g. a textarea in the override form) that appear conditionally.
- `VaguenessPromptModal.tsx:32-45` — same pattern, hardcoded to two button refs.
- `CommitPushModal.tsx`, `WorktreeDiffModal.tsx` (both under `web-app/src/components/unfinished/`, not `components/backlog/` as the requirements doc's flat filenames might imply) — Escape-only; **no Tab trap at all** today.
- `BacklogQueueSection.tsx` (also under `components/unfinished/`) import dialog — no Tab trap; only `stopPropagation` on inner clicks.

All five are real gaps matching the requirements doc's audit list, confirming acceptance criterion 3's scope is accurate.
