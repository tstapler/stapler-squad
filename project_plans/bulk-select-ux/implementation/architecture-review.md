# Architecture Review: bulk-select-ux
**Date**: 2026-06-23
**Verdict**: CONCERNS

_ADR-000-architecture-constitution.md does not exist — no constitution check performed._

---

## Blockers

_(none)_

---

## Concerns

- [ ] **Task 1.1.1-B / Epic 1.4 — Hardcoded hex color in globals.css** — Task 1.4.1-B specifies `rgba(99, 102, 241, 0.08)` as a raw hex value added directly to `globals.css`. Per the CSS architecture rule (ADR-009), hardcoded values in component CSS are forbidden; tokens must be added to `globals.css` first and then referenced. Adding a magic hex here is technically compliant (it goes into globals.css, not a component file), but the rationale in ADR-009 is that all design decisions live in the token contract. If the indigo palette is already tokenized elsewhere, reuse it — `rgba(var(--indigo-500-rgb), 0.08)` or similar. If not, at minimum document the palette origin in a comment so the value can be found and updated consistently. **Recommendation**: Before hardcoding, check `globals.css` and `theme.css.ts` for existing indigo/accent tokens; derive `--session-selected-bg` from them rather than from a bare hex literal.

- [ ] **Plan section "Anchor state" (Pattern Decisions table, row 2) — Self-contradicting documentation** — The Pattern Decisions table says anchor state uses `useState<string | null>` but immediately corrects itself in parentheses: _"(actually a ref is better here); use `useRef<string | null>`"_. The corresponding task (2.1.1-A) correctly specifies `useRef`. This is a documentation-level inconsistency, not a code defect, but it creates implementor confusion. **Recommendation**: Remove the `useState` row from the Pattern Decisions table or replace it with the `useRef` entry to match what the tasks actually prescribe.

- [ ] **Task 3.3.1-B (`beforeunload` flush) — Incomplete / aspirational implementation** — The task body contains a code stub with comments describing what _should_ happen (`// Use sendBeacon or synchronous XMLHttpRequest as last resort`) but no concrete implementation decision. ConnectRPC uses HTTP/2 with binary-encoded protobuf bodies; `sendBeacon` only accepts `Blob`/`FormData`/string payloads and cannot drive a ConnectRPC unary call without a custom shim. The plan acknowledges "accept the data loss risk" but leaves this unresolved. This means the acceptance criterion — _"an attempt is made to flush pending deletes synchronously"_ — cannot be met as written. **Recommendation**: Make the decision explicit at plan time: either (a) accept that tab close during the undo window silently skips deletion and document this as known behavior, or (b) add a `sendBeacon`-compatible HTTP endpoint (e.g., a simple REST DELETE) alongside the ConnectRPC path. Leaving it as a stub means implementors will invent a solution ad-hoc.

- [ ] **Task 3.2.1-C (optimistic removal) — No concrete mechanism specified for restoring sessions to the virtualizer** — The plan calls for "optimistic removal" (remove sessions from the displayed list immediately) and a matching "restore" in `undoFn` (Task 3.2.1-D). However, neither task specifies the state variable that drives the displayed list. `SessionList` derives its displayed rows from props (sessions passed in from parent) filtered by local filter state — there is no local `displayedSessions: SessionData[]` state. Removing sessions from display requires either (a) adding a new `Set<string> pendingDeleteIds` state that is subtracted from props at render time, or (b) mutating an entirely new local copy of the session list. Neither approach is named in the plan. If the implementor picks option (a), the `useMemo` for `filteredSessions` must exclude `pendingDeleteIds`, and the `activeSelection` memo depends on `filteredSessions`, so the chain is `pendingDeleteIds → filteredSessions → activeSelection → BulkActions count`. This needs to be spelled out. **Recommendation**: Add a concrete statement of which state variable controls optimistic removal, and update the `activeSelection` memo chain in Task 4.1.1-A to account for it.

- [ ] **Task 2.2.1-A (`stopImmediatePropagation` on Escape) — May interfere with Omnibar and other document-level Escape handlers** — The plan uses `e.stopImmediatePropagation()` to prevent `page.tsx`'s Escape handler. `stopImmediatePropagation` blocks _all_ subsequently registered listeners on the same element, not just the intended target. If any other component (e.g., Omnibar, modal, tooltip) registers a `document` `keydown` listener after `SessionList`'s effect runs, that listener will be silently suppressed when `selectMode` is active. The existing codebase convention (per the Escape precedence row in Pattern Decisions) is `stopPropagation` on the modal, not `stopImmediatePropagation` on the `SessionList` handler. Using `stopImmediatePropagation` here reverses the responsibility: the consumer (SessionList) clobbers other consumers rather than the producer (modal) claiming priority. **Recommendation**: Follow the established `stopPropagation`-at-source pattern: the Escape handler in `SessionList` should not use `stopImmediatePropagation`; instead, any modal or overlay that should consume Escape first calls `e.stopPropagation()` on its own handler, which already prevents the `SessionList` document listener from receiving the event.

- [ ] **Epic 3.1 (NotificationContext undo variant) — `onUndo` on `NotificationData` couples the data type to a specific interaction pattern** — Adding `onUndo?: () => void` to `NotificationData` is an ISP violation: it makes every consumer of `NotificationData` aware of the "undo" concept even when they handle `"info"`, `"approval_needed"`, etc. The plan already introduces `notificationType: "undo"` as a discriminant; the `onUndo` callback should be constrained to that variant using a discriminated union rather than an optional field on the base type. As written, TypeScript allows `{ notificationType: "info", onUndo: () => {} }`, which is a nonsensical combination. **Recommendation**: Refine `NotificationData` into a discriminated union (or intersect `onUndo` only on the `"undo"` variant with an intersection type / conditional type guard) so the type system prevents the invalid combination at compile time. Example:
  ```ts
  type NotificationData =
    | { notificationType: "info" | "approval_needed" | ...; }
    | { notificationType: "undo"; onUndo: () => void; };
  ```

---

## Nitpicks

- **Task 1.2.1-B: `onChange={() => {/* controlled via onClick */}}`** — An empty `onChange` on a controlled checkbox suppresses React's "uncontrolled input" warning but is semantically odd. Prefer `readOnly` + `onClick` or handle `onChange` properly with a no-op that acknowledges the intent, e.g., `onChange={noop}` with a named variable.

- **Task 2.2.1-C: `navigator.platform` is deprecated** — Use `navigator.userAgentData?.platform` (with fallback) or detect via `e.metaKey` at runtime rather than sniffing platform at render time. The keyboard hint label approach is fine, but the API choice has MDN deprecation warnings.

- **Task 3.3.1-A: empty-dependency `useEffect`** — The comment `// intentionally empty — runs only on unmount` is correct but the `flushPendingDeletes` callback will be stale (captured at mount) if `deleteSessionRpc` or `removeNotification` ever changes identity. Since `flushPendingDeletes` is defined with `useCallback`, verify its dependencies are stable (memoized) to avoid a stale-closure bug on unmount. Consider using a `useRef` to hold the latest `flushPendingDeletes` callback if dep stability cannot be guaranteed.

- **Build-vs-buy consistency** — The research document recommends adding `lastAnchorId` to `bulkSelectionSlice` (Redux). The plan instead uses a `useRef` in `SessionList` local state and never touches the Redux slice. The plan justifies this by noting the Redux slice is unused by `SessionList` today (Option A was rejected). The decision is defensible, but the discrepancy between the research recommendation and the plan's final approach is not explicitly called out in the plan's "Alternatives Considered" section. Document the pivot so future readers don't think the research was ignored.

- **`computeRangeIds` placement** — The plan offers `SessionList.tsx` or `web-app/src/lib/utils/rangeSelect.ts` as two placements. The SRP-clean choice is the util file; placing a pure algorithm in a 1000-line component makes it harder to unit-test in isolation. Prefer the util file.
