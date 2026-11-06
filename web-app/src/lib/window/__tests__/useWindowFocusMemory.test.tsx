import { renderHook } from "@testing-library/react";
import {
  focusKindForTrigger,
  focusWindowContent,
  useWindowFocusMemory,
} from "../useWindowFocusMemory";

function buildPanel(): HTMLElement {
  document.body.innerHTML = `
    <div id="panel" role="tabpanel" tabindex="-1">
      <div data-testid="pane-leaf-p1"><button>p1 header</button></div>
      <div data-testid="pane-leaf-p2"><button>p2 header</button><textarea aria-label="Terminal p2"></textarea></div>
      <div data-testid="pane-leaf-p3"><span>no focusable</span></div>
    </div>`;
  return document.getElementById("panel") as HTMLElement;
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("useWindowFocusMemory", () => {
  it("focusMemory_should_RestorePaneP2AndScroll400_When_SwitchAwayAndBack", () => {
    const { result } = renderHook(() => useWindowFocusMemory());
    result.current.record("w1", { paneId: "p2", scrollTop: 400 });
    result.current.record("w2", { paneId: "p1", scrollTop: 0 });
    expect(result.current.recall("w1")).toEqual({ paneId: "p2", scrollTop: 400 });
    expect(result.current.recall("w2")?.paneId).toBe("p1");
    expect(result.current.recall("never")).toBeNull();
  });

  it("focusMemory_should_KeepOneEntryPerWindow_When_RecordedTwice", () => {
    const { result } = renderHook(() => useWindowFocusMemory());
    result.current.record("w1", { paneId: "p1", scrollTop: 1 });
    result.current.record("w1", { paneId: "p2", scrollTop: 2 });
    expect(result.current.recall("w1")).toEqual({ paneId: "p2", scrollTop: 2 });
  });
});

describe("focusWindowContent", () => {
  it("focusWindowContent_should_FocusTerminalInsidePane_When_PaneHasTextarea", () => {
    const panel = buildPanel();
    const target = focusWindowContent(panel, "p2");
    expect(target?.getAttribute("aria-label")).toBe("Terminal p2");
    expect(document.activeElement).toBe(target);
  });

  it("focusWindowContent_should_FocusFirstFocusableInPane_When_NoTextarea", () => {
    const panel = buildPanel();
    const target = focusWindowContent(panel, "p1");
    expect(target?.textContent).toBe("p1 header");
    expect(document.activeElement).toBe(target);
  });

  it("focusWindowContent_should_FallBackToTabpanel_When_PaneMissingOrHasNothingFocusable", () => {
    const panel = buildPanel();
    expect(focusWindowContent(panel, "p3")).toBe(panel);
    expect(document.activeElement).toBe(panel);
    panel.focus();
    (document.activeElement as HTMLElement).blur();
    expect(focusWindowContent(panel, "gone")).toBe(panel);
    expect(focusWindowContent(panel, null)).toBe(panel);
  });

  it("focusWindowContent_should_ReturnNull_When_NoPanel", () => {
    expect(focusWindowContent(null, "p1")).toBeNull();
  });
});

describe("focusKindForTrigger", () => {
  const coarse = (matches: boolean) =>
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      writable: true,
      value: (query: string) => ({ matches: matches && query === "(pointer: coarse)" }),
    });

  afterEach(() => {
    Reflect.deleteProperty(window, "matchMedia");
  });

  it.each([
    ["tab-key", "tab"],
    ["touch", "none"],
    ["swipe", "none"],
    ["mouse", "pane"],
    ["leader", "pane"],
    ["overview", "pane"],
    ["programmatic", "pane"],
  ] as const)("focusKindForTrigger_should_Return_%s_%s_When_FinePointer", (trigger, kind) => {
    coarse(false);
    expect(focusKindForTrigger(trigger)).toBe(kind);
  });

  it("focusKindForTrigger_should_NeverFocusTerminal_When_OverviewPickOnCoarsePointer", () => {
    coarse(true);
    expect(focusKindForTrigger("overview")).toBe("none");
    expect(focusKindForTrigger("leader")).toBe("pane");
  });
});
