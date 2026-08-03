# Architecture Research: repo-path-picker-parity

No EventStorming table — this is CRUD/UI plumbing (per requirements.md), not a multi-actor
business domain. No dedicated React/frontend "interface pollution" rule exists in this repo
(`.claude/rules/interface-pollution-checklist.md` is explicitly Go-oriented); the closest
applicable idiom is `.claude/rules/css-architecture.md`'s general principle of fixing shared
behavior at the shared component (vanilla-extract tokens, not per-consumer hacks) and the
existing in-file precedent inside `Omnibar.tsx` itself (self-contained dropdowns each own
their own `stopImmediatePropagation`, never leak the concern to the parent).

## 1. Escape propagation: fix at RepoPathInput (component-local), not the call site

**Fix `RepoPathInput.tsx`'s own `handleKeyDown` (line 134-137), not `OmnibarCreationPanel.tsx`
or `Omnibar.tsx`.**

```ts
case "Escape":
  if (open) {
    e.nativeEvent.stopImmediatePropagation();
  }
  setOpen(false);
  setSelectedIndex(-1);
  break;
```

Guard on `open` (not unconditional) so Escape still bubbles normally when no dropdown is
showing — this preserves AC6's "no regression to ... Escape-to-reset behavior when no
RepoPathInput dropdown is open."

**Why component-local, not a workaround at the Omnibar call site:** a workaround in
`OmnibarCreationPanel.tsx`/`Omnibar.tsx` (e.g. wrapping just the two new `RepoPathInput`
usages in a keydown-swallowing div) would only patch the two fields this ticket touches,
leave the bug live inside `RepoPathInput` itself, and require every *future* consumer nested
under any keydown-capturing ancestor to rediscover and re-fix the same bug. The component that
owns a dropdown should own suppressing propagation of the keys that control it — see
`Omnibar.tsx` lines 748/772/820/839, which already apply exactly this pattern to Omnibar's
own internal dropdowns (`@command`, alias, discovery mode) rather than pushing the concern up
to `modal`'s `onKeyDown` or the document listener.

**This is not a hypothetical risk for other consumers — it is a second, already-shipping
instance of the same bug.** `NewShellDialog.tsx` (lines 32-38) renders a `RepoPathInput` for
its working-directory field and separately registers:

```ts
// NewShellDialog.tsx:32-38
useEffect(() => {
  const handler = (e: KeyboardEvent) => {
    if (e.key === "Escape") onCancel();
  };
  document.addEventListener("keydown", handler);
  return () => document.removeEventListener("keydown", handler);
}, [onCancel]);
```

This is unconditional — it does not check whether a `RepoPathInput` dropdown is open. Today,
pressing Escape to dismiss the path-suggestion dropdown inside `NewShellDialog` already closes
the *entire dialog* instead of just the dropdown, because `RepoPathInput`'s current Escape
handler doesn't stop propagation and the native keydown bubbles to `document`. Fixing this in
`RepoPathInput` itself fixes both the Omnibar hazard this ticket is scoped to *and* this
pre-existing `NewShellDialog` bug, for free, in one place.

Checked the other two consumers for the same shape of hazard:
- `WorkflowForm.tsx` — has its own `Escape` handling (line 109) but it's scoped to a
  `textarea`'s slash-command dropdown (`handleCommandKeyDown`), not a document-level
  close-the-form listener. `onCancel` (line 342) is only wired to a button `onClick`, never to
  Escape. No hazard here today.
- `BacklogItemForm.tsx` — `onCancel` (line 782) is likewise only a button `onClick`; no
  document-level or ancestor `keydown` listener exists. No hazard.
- `LocalFileBrowser.tsx` — no `Escape`/`onCancel`/`onClose` keydown wiring at all near its
  `RepoPathInput` usage. No hazard.

So the fix is strictly additive/corrective for `NewShellDialog` and neutral for
`WorkflowForm`/`BacklogItemForm`/`LocalFileBrowser` — no other consumer relies on Escape
bubbling out of `RepoPathInput`.

## 2. Tiebreak comparator for `selectActiveSessionsSortedByUpdatedAt`

The generated `Session` type (`web-app/src/gen/session/v1/types_pb.ts:23-96`) has both
`id: string` (line 29, "Unique identifier (uses title as ID for now)") and
`createdAt?: Timestamp` (line 90) alongside `updatedAt?: Timestamp` (line 95) — both are
available as deterministic secondary keys.

Recommended comparator (`sessionsSlice.ts:117-123`):

```ts
export const selectActiveSessionsSortedByUpdatedAt = createSelector(
  selectAllSessions,
  (sessions) =>
    sessions
      .filter((s) => s.status !== SessionStatus.UNSPECIFIED)
      .sort((a, b) => {
        const byUpdated = Number(b.updatedAt?.seconds ?? 0) - Number(a.updatedAt?.seconds ?? 0);
        if (byUpdated !== 0) return byUpdated;
        const byCreated = Number(b.createdAt?.seconds ?? 0) - Number(a.createdAt?.seconds ?? 0);
        if (byCreated !== 0) return byCreated;
        return a.id < b.id ? -1 : a.id > b.id ? 1 : 0; // final deterministic tiebreak
      })
);
```

Three-level tiebreak: `updatedAt` desc → `createdAt` desc (a session that was created more
recently but never updated is still "more recent" than one created long ago) → `id` ascending
as the final, always-defined deterministic key (guarantees total order even if both timestamps
are simultaneously absent/zero, e.g. brand-new in-memory sessions not yet persisted).

**Memoization semantics — the concern in the research question doesn't actually apply.**
`createSelector`'s memoization is keyed on referential equality of the selector's *inputs*
(`selectAllSessions`'s output array reference), not on the comparator function's identity.
Whether the comparator is inlined (as today) or extracted to a module-level named function
makes zero difference to when the selector recomputes — the combiner (`.filter().sort(...)`)
only re-runs when `selectAllSessions`'s output reference changes, exactly as it does today with
the simpler one-line comparator. Extending the comparator's body doesn't create a new selector
instance per render (the selector itself is still declared once at module scope, as it already
is); no memoization-breaking risk exists here at all. No extraction to a named module-level
function is required — inlining the extended comparator is consistent with the existing style.

## 3. Impact of switching `useSessionRepoPaths`'s source

`useSessionRepoPaths` (`web-app/src/lib/hooks/useSessionRepoPaths.ts`) is `RepoPathInput`'s
**sole** caller — grepped and confirmed no other component imports it directly. So "other
consumers" of the hook fix are, transitively, `RepoPathInput`'s four current call sites
(`BacklogItemForm`, `NewShellDialog`, `WorkflowForm`, `LocalFileBrowser`), all of which get the
new source through the same shared hook — no divergent consumption path to reconcile.

Two behavior changes from swapping `selectAllSessions` → `selectActiveSessionsSortedByUpdatedAt`:

1. **Order**: today the hook dedupes in `selectAllSessions`'s raw insertion order (entity
   adapter order, not meaningful recency). After the swap it becomes true recency-first. This
   is the intended improvement per requirements.md R3/R7 framing — "recency ordering was never
   differentiated per-consumer" — and is a strict improvement for all four consumers, not a
   behavior consumers depend on today (nothing sorts or otherwise relies on the old order).

2. **Filtering**: `selectActiveSessionsSortedByUpdatedAt` drops sessions where
   `status === SessionStatus.UNSPECIFIED`. Checked whether this could silently drop
   previously-suggested paths: `SESSION_STATUS_UNSPECIFIED` is the proto3 zero-value, and its
   own doc comment in the generated code (`session_pb.ts:1376`, in the history-entry
   status-matching context) states it explicitly means **"no live session matches this history
   entry"** — i.e. a sentinel for "not a real/live session," not a legitimate state a genuine
   active session sits in. `HistoryEntryCard.tsx:55` already filters on this exact same
   condition (`entry.sessionStatus !== SessionStatus.UNSPECIFIED`) to distinguish real sessions
   from placeholder/history-only entries, which is precedent that this filter is the
   established, safe way to exclude non-real entries elsewhere in this codebase. Net: the
   swap is a pure improvement, not a regression — no consumer currently depends on
   UNSPECIFIED-status sessions surfacing as path suggestions, and excluding them removes noise
   consistent with how the rest of the app already treats that status.

## Summary of the pattern to apply

- **(a) RepoPathInput swap-in**: direct component substitution in `OmnibarCreationPanel.tsx`
  at both call sites (~488-501, ~651-660) — no new abstraction, matches R1's "no
  new/duplicated picker implementation."
- **(b) Sourcing + tiebreak**: two small, additive edits — `useSessionRepoPaths.ts` swaps its
  selector import/call; `sessionsSlice.ts`'s existing `selectActiveSessionsSortedByUpdatedAt`
  gains a two-level tiebreak appended to its existing comparator. No new selector, no
  memoization-affecting restructuring needed.
- **(c) Escape fix**: one-line guard added inside `RepoPathInput.tsx`'s existing
  `handleKeyDown` `Escape` case, using the codebase's own established
  `e.nativeEvent.stopImmediatePropagation()` idiom (already used 4x in `Omnibar.tsx`). Fixing
  it here — not at the Omnibar/OmnibarCreationPanel call site — is required because it also
  fixes a pre-existing, already-shipping identical bug in `NewShellDialog.tsx` (unconditional
  document-level Escape-closes-dialog listener with no dropdown-open guard), which a call-site
  workaround would leave unfixed.
