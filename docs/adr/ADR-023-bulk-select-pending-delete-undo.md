# ADR-023: Client-Side Pending-Delete for Bulk Session Undo

**Date**: 2026-06-23
**Status**: Accepted
**Deciders**: Tyler Stapler
**Feature**: bulk-select-ux

---

## Context

The session list's bulk delete action permanently destroys sessions via the `DeleteSession` RPC. The `session.proto` has no `RestoreSession` or `UndeleteSession` RPC. Deletion is a hard-delete with no server-side recovery path.

UX research strongly recommends an undo toast over a confirmation modal for bulk destructive actions:
- NN/G: "An undo mechanism is preferable to a confirmation dialog. Confirmations add friction and users habituate to clicking OK without reading."
- Gmail, Linear, and Notion all use undo-on-delete rather than confirm-before-delete.
- Without undo, users fear bulk delete and tolerate cluttered session lists — degrading the tool's core value.

The existing codebase provides a `NotificationContext` with auto-close timers and action callbacks. No external toast library exists (`sonner`, `react-hot-toast`, etc. are absent from `package.json`).

---

## Decision

Implement a **client-side pending-delete pattern** rather than adding a `RestoreSession` RPC or keeping the confirmation modal.

### Mechanics

1. User clicks "Delete N sessions" in `BulkActions`
2. Selected sessions are **optimistically removed** from the displayed list immediately
3. `DeleteSession` RPCs are **not yet fired**
4. A `pendingDeleteRef` in `SessionList` holds: `{ ids: Set<string>, timer: ReturnType<typeof setTimeout>, toastId: string }`
5. An undo toast is shown via `NotificationContext.showUndoToast("Deleted N sessions", undoFn)` for 5 seconds
6. **Undo path**: user clicks "Undo" → `clearTimeout`, restore sessions to list, nullify ref, dismiss toast, zero RPCs fired
7. **Commit path**: timer fires → call `DeleteSession` for all IDs in parallel → nullify ref

### Replace-Not-Stack Semantics

`pendingDeleteRef` holds at most one pending-delete window at a time. If a second bulk delete fires while a window is active, the first window is **flushed immediately** (RPCs fired for first batch without waiting for undo), then a new window opens for the second batch. This avoids stacked undo states and ensures the user always has a clear one-step undo.

### Lifecycle Safety

- **Component unmount** (`useEffect` cleanup): `flushPendingDeletes()` is called synchronously
- **Tab close** (`beforeunload`): attempt to flush via `navigator.sendBeacon` or synchronous RPC; if not feasible, accept the risk that sessions remain undestroyed (acceptable for a developer tool)

---

## Alternatives Considered

### Option A: Keep the confirmation modal (no undo)

**Strength**: Zero new state complexity; modal already exists (`showBulkDeleteConfirm`).

**Weakness**: NN/G research shows confirmation fatigue — users click through without reading. The "Undo" model is significantly better UX and is the expectation from Gmail and other familiar tools. The requirements spec explicitly calls for undo.

**Rejected**: The UX cost is too high; the implementation cost of the pending-delete pattern is modest.

### Option B: Add `RestoreSession` proto RPC

**Strength**: True server-side undo; no risk of silent non-deletion on tab close.

**Weakness**: Requires new proto + backend implementation + migration. `DeleteSession` currently hard-deletes the tmux session and git worktree immediately — a restore RPC would need soft-delete infrastructure (tombstoning, TTL cleanup). Estimated additional scope: 3–5 days. Requirements spec explicitly forbids new proto RPCs unless strictly required.

**Rejected**: Out of scope per requirements constraints.

### Option C: Add `sonner` toast library

**Strength**: Sonner has a built-in promise-chaining undo pattern; 4.6 KB gzipped; zero runtime deps.

**Weakness**: Adds a second toast system alongside the existing `NotificationContext`. Two toast systems create visual and behavioral inconsistency.

**Rejected**: Extending `NotificationContext` with a new `"undo"` type is cleaner and zero-dependency.

---

## Consequences

### Positive

- Zero new proto RPCs or backend changes
- UX quality matches Gmail/Linear expectations
- `NotificationContext` gets a reusable `"undo"` notification type for future features
- No confirmation modal friction on bulk delete

### Negative / Accepted Risks

- **Tab close during undo window**: If the user closes the tab during the 5-second undo window, sessions may not be deleted (the timer fires in the browser engine but the RPCs are async and may not complete before unload). Mitigated by `beforeunload` flush; residual risk is accepted.
- **Single undo window**: Replace-not-stack means a second bulk delete commits the first batch without undo opportunity. This is the correct behavior (matches Gmail) but may surprise users who expect an undo stack.
- **Optimistic removal race with WatchSessions**: If the server pushes a `SessionEvent` update for a "pending delete" session during the undo window, the session may reappear in the list. Guard: the `WatchSessions` stream handler must check `pendingDeleteRef` and ignore events for pending-delete session IDs.

---

## Implementation Notes

- `pendingDeleteRef`: `useRef<{ ids: Set<string>; timer: ReturnType<typeof setTimeout>; toastId: string } | null>`
- `flushPendingDeletes`: stable `useCallback` — safe to use in `useEffect` cleanup
- `showUndoToast`: new method on `NotificationContextValue`; calls `addNotification({ notificationType: "undo", message, onUndo })`
- `NotificationToast.tsx`: render "Undo" button when `notificationType === "undo" && onUndo !== undefined`
