# Stack Research: modal-focus-trap

## Versions (source: `web-app/package.json`)

| Package | Version |
|---|---|
| React / react-dom | `^19.0.0` |
| Next.js | `15.3.2` (`reactStrictMode: true` in `web-app/next.config.ts:15`) |
| TypeScript | `^5.9.3` (`@types/react` `^19`) |
| Jest | `^30.2.0`, `jest-environment-jsdom` `^30.2.0` |
| @testing-library/react | `^16.3.0` |
| @testing-library/user-event | `^14.5.2` |
| @testing-library/jest-dom | `^6.9.1` |
| @playwright/test | `^1.57.0` |

Jest config (`web-app/jest.config.js`) uses a multi-project setup: the jsdom project (`testEnvironment: "jest-environment-jsdom"`, `setupFilesAfterEach: jest.setup.js`) is the one relevant to component/hook tests; three other projects run `testEnvironment: "node"` for non-DOM suites. Playwright config (`web-app/playwright.config.ts`) sets `testDir: './tests/e2e'` and a dynamic `baseURL` built from `TEST_PORT`.

## Existing focus-trap dependencies — none to prefer

Checked `dependencies` and `devDependencies` in `package.json` for `focus-trap`, `focus-trap-react`, `react-aria`, `react-focus-lock`, and Radix's focus primitives. Findings:
- `@radix-ui/react-dialog` (`^1.1.15`), `@radix-ui/react-accordion`, `@radix-ui/react-tabs`, `@radix-ui/react-tooltip` are installed, but no component in the affected set (`ReviewChangesModal`, `BacklogFileBrowserModal`, `VaguenessPromptModal`, `GateVerdictBox`, `CommitPushModal`, `WorktreeDiffModal`, `BacklogQueueSection`) is built on Radix's `Dialog` — they're all hand-rolled `role="dialog"` markup with a plain `<div>`, so there's no existing Radix `FocusScope` to lean on without a larger rewrite.
- No standalone `focus-trap`, `focus-trap-react`, or `react-aria`/`react-aria-components` package is installed anywhere in `package.json`.
- Confirms the requirements doc's constraint: reuse the in-house `useFocusTrap` hook, not a new dependency.

## `useFocusTrap` hook — current implementation

File: `web-app/src/lib/hooks/useFocusTrap.ts` (65 lines, full read).

```ts
export function useFocusTrap(
  ref: AnyElementRef,
  isActive: boolean,
  triggerRef?: AnyElementRef
) {
  useEffect(() => {
    if (!isActive || !ref.current) return;
    const container = ref.current as HTMLElement;
    const focusable = Array.from(
      container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)
    ).filter((el) => !el.closest("[aria-hidden='true']"));
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    first?.focus();
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Tab") return;
      if (focusable.length === 0) { e.preventDefault(); return; }
      if (e.shiftKey) {
        if (document.activeElement === first) { e.preventDefault(); last?.focus(); }
      } else {
        if (document.activeElement === last) { e.preventDefault(); first?.focus(); }
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    const triggerEl = triggerRef?.current;
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      triggerEl?.focus();
    };
  }, [isActive, ref, triggerRef]);
}
```

Key facts, confirmed by reading the source (not inferred):

1. **Signature**: `useFocusTrap(ref: RefObject<HTMLElement|null> | MutableRefObject<HTMLElement|null>, isActive: boolean, triggerRef?)`. `ref` must point at the modal container; `isActive` gates the whole effect; `triggerRef` (optional) is refocused on deactivation/unmount.
2. **Snapshot-once bug (AC #4 target)**: `focusable`, `first`, `last` are computed a single time when the effect runs (on activation, since `[isActive, ref, triggerRef]` are the only deps). The `handleKeyDown` closure captures that snapshot. If the modal's focusable-element set changes while open (e.g. a button appears/disappears, an input becomes enabled), Tab-wrapping will use stale `first`/`last` elements — this is exactly what AC #4 requires fixing: re-query `container.querySelectorAll` on every Tab keypress inside `handleKeyDown`, not once outside it.
3. **Listener scope**: attached to `document`, not the container — a global capture, not a per-element listener. Any DOM-based rewrite must preserve this scope or e2e tab-loop tests attached to `document` semantics will change behavior.
4. **Escape key**: **not handled by this hook at all** — matches requirements' "Escape unchanged" (Escape handling lives in each modal's own separate keydown effect, outside this hook's remit).
5. **No inert/aria-hidden application** to the rest of the page — matches the stated out-of-scope item.
6. **Focus-on-activate**: unconditionally calls `first?.focus()` when the effect fires, with no check for whether focus is already correctly placed inside the container — relevant if a re-render toggles `isActive` false→true→false rapidly (StrictMode double-invoke risk, below).

## Current call sites (5, matches requirements' count)

Confirmed via `grep -rn "useFocusTrap" src`:

| File | Call |
|---|---|
| `web-app/src/components/sessions/ResumeSessionModal.tsx` | `useFocusTrap(modalRef, isOpen, triggerRef)`-style (dialog) |
| `web-app/src/components/sessions/WorkspaceSwitchModal.tsx` | modal |
| `web-app/src/components/sessions/TagEditor.tsx` | inline popover-style trap |
| `web-app/src/components/sessions/SessionActionsOverflow.tsx` | dropdown/menu trap |
| `web-app/src/components/ui/DebugMenu.tsx` | menu trap |

Example confirmed in `BacklogFileBrowserModal.tsx` — wait, that file does NOT currently import the hook; grep only matched the 5 files above plus the hook's own definition and pre-existing tests (`ResumeSessionModal.test.tsx`, `SessionActionsOverflow.test.tsx`, plus two unrelated files — `src/app/page.tsx`, `src/app/review-queue/page.tsx`, `ReviewQueueContent.auto-advance.test.tsx`, `SessionCard.*.test.tsx` — that reference the hook only via shared test utilities/mocks, not as new call sites; verify at implementation time if any of those need updating). The two target files (`web-app/src/components/backlog/ReviewChangesModal.tsx`, `web-app/src/components/backlog/BacklogFileBrowserModal.tsx`) and the five audit targets (`VaguenessPromptModal.tsx`, `GateVerdictBox.tsx`, `CommitPushModal.tsx` and `WorktreeDiffModal.tsx` under `web-app/src/components/unfinished/`, and `BacklogQueueSection.tsx` also under `unfinished/`) do **not** currently import `useFocusTrap` — confirming the requirements' premise that they lack the fix.

## React 19 / StrictMode gotchas relevant to this fix

- `web-app/next.config.ts:15` sets `reactStrictMode: true`, so in dev, all `useEffect`s in the affected components double-invoke (mount → cleanup → mount) on initial render. The existing hook's cleanup (`removeEventListener` + `triggerEl?.focus()`) is idempotent and safe under double-invoke, but **the auto-focus-first-element behavior will fire twice** in dev — harmless visually (same element refocused) but worth confirming in new tests (assert final focus state, not call count, to avoid StrictMode-induced test flakiness).
- React 19 continues (from 18) to batch state updates and run effects asynchronously after paint (`useEffect`, not `useLayoutEffect`). The hook's `first?.focus()` call happens post-paint; this is already the existing pattern (matches the 5 current call sites) so no new gotcha is introduced by extending it to 7 more components — but if a future re-query-on-keydown implementation needs the very latest DOM (e.g. an element added in the same render as `isActive` flips true), double-check whether a `useLayoutEffect` is warranted only if a flash-of-unfocused-content is observed; not needed for the Tab-requery fix itself since that happens on keydown, well after the effect has committed.
- No `act()`-related React 19 testing changes affect this work beyond the already-installed `@testing-library/react@16.3.0`, which targets React 18/19's concurrent rendering.

## Summary for the plan phase

- Stack is React 19 + Next 15 (StrictMode on) + TS 5.9, Jest 30 (jsdom project) + RTL 16 + Playwright 1.57 — all already well-exercised by the 5 existing `useFocusTrap` call sites and their tests (e.g. `ResumeSessionModal.test.tsx`, `SessionActionsOverflow.test.tsx` are useful templates for the new Jest tests).
- No focus-trap npm package (Radix or otherwise) is a better fit than the existing hook; none of the 7 affected components use Radix `Dialog` primitives.
- The hook's Tab-handling closure captures `focusable`/`first`/`last` once per activation (`useFocusTrap.ts:26-31`, outside `handleKeyDown`) — this is the concrete AC #4 defect: move the `querySelectorAll` + first/last computation inside `handleKeyDown` so it re-runs on every Tab press.
- Escape handling is out of this hook's scope already (confirmed: hook only listens for `"Tab"`), so no Escape-behavior risk from wiring these 7 components onto it.
