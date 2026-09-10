# ADR-001: Re-query focusable elements on each Tab keypress in `useFocusTrap`

**Status**: Accepted
**Date**: 2026-08-29
**Context project**: modal-focus-trap

## Context

`web-app/src/lib/hooks/useFocusTrap.ts` computes its `focusable`/`first`/`last`
element list once, inside the effect body, on activation
([useFocusTrap.ts:26-31](../../../web-app/src/lib/hooks/useFocusTrap.ts)). The
effect's dependency array is `[isActive, ref, triggerRef]`
([useFocusTrap.ts:63](../../../web-app/src/lib/hooks/useFocusTrap.ts)), so the
snapshot is never recomputed for the lifetime of a single "open" session.

Three of the seven components this project wires onto the hook render
additional focusable controls *after* mount, without unmounting/remounting
(`isActive` stays `true` across the transition):

- `ReviewChangesModal.tsx` — diff fetch resolves after mount; `DiffRenderer`
  renders a retry button on error or refresh/view-mode buttons on success.
- `WorktreeDiffModal.tsx` — same `DiffRenderer` async-load shape.
- `BacklogFileBrowserModal.tsx` — `FileTree`/`FileContentViewer` populate
  further focusable rows once their own fetches resolve.

Against these three, the trap would keep cycling Tab among the stale
first/last captured at mount (typically just the close button and the
"Open in Terminal" link while `loading === true`), silently excluding the
later-rendered controls from the Tab cycle. This does not let focus escape
the modal (the WCAG 2.4.3 ask is still met), but it falls short of full APG
dialog-pattern compliance and would ship the exact same partial-trap gap into
three additional call sites.

`GateVerdictBox.tsx`'s existing hand-rolled trap
(`handleSkipConfirmKeyDown`, [GateVerdictBox.tsx:215-240](../../../web-app/src/components/backlog/GateVerdictBox.tsx))
already avoids this problem: it is wired via `onKeyDown` on the dialog element
and reads `cancelRef.current`/`confirmRef.current` fresh on every keypress
rather than snapshotting once. It has been running in production without
requiring a stale-snapshot workaround.

## Decision

Move the `container.querySelectorAll(FOCUSABLE_SELECTORS)` call (and the
`first`/`last` derivation) from the effect body into `handleKeyDown` itself,
so it re-runs on every `Tab` keypress instead of once per activation. The
initial `first?.focus()` on activation keeps its existing one-time snapshot
(no known component in this project or the 7 existing adopters (5 components + 2 page-level dialogs, app/page.tsx and app/review-queue/page.tsx) needs the
*initial* focus target to track post-mount DOM changes — only the *ongoing*
Tab cycle does).

This is a same-file, backward-compatible change:
- The 7 existing adopters (5 components + 2 page-level dialogs, app/page.tsx and app/review-queue/page.tsx) (`ResumeSessionModal`, `WorkspaceSwitchModal`,
  `TagEditor`, `SessionActionsOverflow`, `DebugMenu`) have static focusable
  sets once open — re-querying on each keypress is a no-op for them
  behaviorally, at the cost of one extra `querySelectorAll` call per Tab
  press (already proven cheap and portal-safe via `SessionActionsOverflow`'s
  9 portaled dialogs, per `pitfalls.md` finding #4).
- No hook signature change — `ref`, `isActive`, `triggerRef` are unchanged.

## Alternatives Considered

1. **Leave the snapshot as-is; accept the partial-trap as good-enough for
   this bug's scope.** Rejected: it's a correctness gap the hook can close
   for every future async-content adopter, not just this project's three, for
   the cost of moving four lines within one file. Deferring it would also
   mean documenting a known limitation instead of just fixing it.
2. **Re-run the whole effect (including the `document.addEventListener`
   attach/detach) via a `MutationObserver` watching the container.**
   Rejected: substantially more code and a new failure surface
   (observer lifecycle, disconnect timing) to solve the same problem the
   simpler per-keypress requery already solves. GateVerdictBox's existing
   hand-rolled trap is proof the simpler approach works in this codebase.

## Consequences

- All 7 new adopters plus the 7 existing adopters (5 components + 2 page-level dialogs, app/page.tsx and app/review-queue/page.tsx) get an always-current
  focusable set on every Tab press, matching `GateVerdictBox`'s pre-existing
  (and more robust) hand-rolled behavior — which this project also retires
  in favor of the shared hook (see plan.md Story 3.1.2).
- No new dependency, no public API change, no test needs to change for the
  7 existing static-content adopters beyond re-running their current suites
  to confirm no regression (plan.md Task 1.1.1b).
