/**
 * Tests for useFocusRestoreOnRemoval (review finding #8): extracted from the
 * near-duplicate focus-restore logic that used to live inline in
 * ReviewQueuePanel and NotificationItem's NeedsDecisionSection, but shipped
 * with zero coverage. Verifies focus moves to the sibling at the same index
 * after the focused item is removed, with a fallback when no sibling remains.
 *
 * Uses a manual requestAnimationFrame mock (same pattern as
 * useTerminalMetrics.test.ts) since the hook defers the focus move by one
 * frame to let the removed row's DOM node actually unmount first.
 */

import { renderHook, act } from "@testing-library/react";
import type { RefObject } from "react";
import { useFocusRestoreOnRemoval } from "./useFocusRestoreOnRemoval";

// ── requestAnimationFrame mock ──────────────────────────────────────────────

let rafCallback: FrameRequestCallback | null = null;

function setupRAFMock() {
  rafCallback = null;
  global.requestAnimationFrame = (cb: FrameRequestCallback): number => {
    rafCallback = cb;
    return 1;
  };
  global.cancelAnimationFrame = (): void => {
    rafCallback = null;
  };
}

function flushRAF(): void {
  if (rafCallback) {
    const cb = rafCallback;
    rafCallback = null;
    act(() => {
      cb(0);
    });
  }
}

// ── DOM fixture helpers ──────────────────────────────────────────────────────

function makeFocusableElements(ids: string[]): Map<string, HTMLElement> {
  const elements = new Map<string, HTMLElement>();
  for (const id of ids) {
    const el = document.createElement("button");
    el.id = id;
    el.tabIndex = 0;
    document.body.appendChild(el);
    elements.set(id, el);
  }
  return elements;
}

describe("useFocusRestoreOnRemoval", () => {
  let elements: Map<string, HTMLElement>;
  let fallbackEl: HTMLElement;
  let fallbackRef: RefObject<HTMLElement | null>;

  beforeEach(() => {
    setupRAFMock();
    document.body.innerHTML = "";
    elements = makeFocusableElements(["a", "b", "c"]);
    // tabIndex={-1} matches the real call sites (NotificationItem's headingRef,
    // ReviewQueuePanel's panelHeadingRef) — without it jsdom (like a real browser)
    // won't move activeElement to a plain <h3>.focus() call.
    fallbackEl = document.createElement("h3");
    fallbackEl.tabIndex = -1;
    document.body.appendChild(fallbackEl);
    fallbackRef = { current: fallbackEl };
  });

  function resolveElement(id: string): HTMLElement | null {
    return elements.get(id) ?? null;
  }

  it("moves focus to the sibling at the same index when the focused item is removed", () => {
    const { result, rerender } = renderHook(
      ({ ids }) => useFocusRestoreOnRemoval(ids, resolveElement, fallbackRef),
      { initialProps: { ids: ["a", "b", "c"] } }
    );

    // Simulate "b" (index 1) holding focus, as the onFocus wiring at the call
    // sites does: focusedIdRef.current = <row's id>.
    act(() => {
      result.current.current = "b";
    });

    // "b" is removed — "c" shifts into index 1, the position "b" held.
    rerender({ ids: ["a", "c"] });
    flushRAF();

    expect(document.activeElement).toBe(elements.get("c"));
    expect(fallbackEl).not.toBe(document.activeElement);
    // The ref is cleared once the restore fires so a later unrelated removal
    // doesn't reuse a stale focused-id.
    expect(result.current.current).toBeNull();
  });

  it("falls back to fallbackRef when no sibling remains after removal", () => {
    const { result, rerender } = renderHook(
      ({ ids }) => useFocusRestoreOnRemoval(ids, resolveElement, fallbackRef),
      { initialProps: { ids: ["a"] } }
    );

    act(() => {
      result.current.current = "a";
    });

    rerender({ ids: [] });
    flushRAF();

    expect(document.activeElement).toBe(fallbackEl);
  });

  it("does nothing when the removed item did not hold focus", () => {
    const { result, rerender } = renderHook(
      ({ ids }) => useFocusRestoreOnRemoval(ids, resolveElement, fallbackRef),
      { initialProps: { ids: ["a", "b", "c"] } }
    );

    act(() => {
      result.current.current = "a";
    });

    // "b" is removed, but "a" (not "b") holds focus — no restore should fire.
    rerender({ ids: ["a", "c"] });

    expect(rafCallback).toBeNull();
    expect(result.current.current).toBe("a");
  });
});
