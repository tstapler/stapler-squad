import { act, render, screen } from "@testing-library/react";
import { CommandBar } from "../CommandBar";
import { registry } from "@/lib/shortcuts/shortcutRegistry";
import { LEADER_STORAGE_KEY } from "@/lib/shortcuts/leaderConfig";

function press(init: KeyboardEventInit & { key: string }, target: EventTarget = document.body) {
  const ev = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  act(() => {
    target.dispatchEvent(ev);
  });
  return ev;
}
const leader = { key: ";", code: "Semicolon", altKey: true };

describe("CommandBar", () => {
  const cleanups: Array<() => void> = [];
  let next: jest.Mock;
  let rename: jest.Mock;
  const bubble = jest.fn();

  beforeEach(() => {
    localStorage.removeItem(LEADER_STORAGE_KEY);
    next = jest.fn();
    rename = jest.fn();
    const base = { context: "global" as const, group: "Windows" };
    cleanups.push(
      registry.register("t.next", { ...base, label: "Next window", sequence: ["w", "n"], action: next }),
      registry.register("t.rename", { ...base, label: "Rename window", sequence: ["w", ","], action: rename }),
      registry.register("t.tray", { context: "global", group: "Notifications", label: "Toggle notification tray", sequence: ["n", "t"], action: jest.fn() })
    );
    document.addEventListener("keydown", bubble);
  });

  afterEach(() => {
    cleanups.splice(0).forEach((c) => c());
    document.removeEventListener("keydown", bubble);
    bubble.mockClear();
    jest.useRealTimers();
  });

  it("leader_should_openBarAndNeverReachBubblePhase", () => {
    render(<CommandBar />);
    const ev = press(leader);
    expect(screen.getByTestId("command-bar")).toBeInTheDocument();
    expect(ev.defaultPrevented).toBe(true);
    expect(bubble).not.toHaveBeenCalled();
  });

  it("leader_should_beNoOpDuringComposition", () => {
    render(<CommandBar />);
    const ev = press({ ...leader, isComposing: true });
    expect(screen.queryByTestId("command-bar")).toBeNull();
    expect(ev.defaultPrevented).toBe(false);
  });

  it("leader_should_beNoOpWhileAnotherDialogIsOpen", () => {
    const dlg = document.createElement("div");
    dlg.setAttribute("role", "dialog");
    document.body.appendChild(dlg);
    render(<CommandBar />);
    press(leader);
    expect(screen.queryByTestId("command-bar")).toBeNull();
    dlg.remove();
  });

  it("whichKey_should_descendOnBranchKeyAndExecuteLeaf", () => {
    jest.useFakeTimers();
    render(<CommandBar />);
    press(leader);
    press({ key: "w" });
    expect(screen.getByTestId("command-bar-path")).toHaveTextContent("Windows");
    press({ key: "n" });
    act(() => {
      jest.runAllTimers();
    });
    expect(next).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("command-bar")).toBeNull();
  });

  it("jk_should_moveAriaActiveDescendantAndEscBacksOutThenCloses", () => {
    render(<CommandBar />);
    press(leader);
    const input = screen.getByTestId("command-bar-input");
    const first = input.getAttribute("aria-activedescendant");
    press({ key: "j" });
    expect(input.getAttribute("aria-activedescendant")).not.toBe(first);
    press({ key: "k" });
    expect(input.getAttribute("aria-activedescendant")).toBe(first);
    press({ key: "w" });
    press({ key: "Escape" });
    expect(screen.getByTestId("command-bar")).toBeInTheDocument();
    expect(screen.queryByTestId("command-bar-path")).toBeNull();
    press({ key: "Escape" });
    expect(screen.queryByTestId("command-bar")).toBeNull();
  });

  it("unboundPrintable_should_switchToFuzzyAndFilter", () => {
    render(<CommandBar />);
    press(leader);
    press({ key: "r" });
    press({ key: "e" });
    press({ key: "n" });
    expect(screen.getByTestId("command-bar")).toHaveAttribute("data-mode", "fuzzy");
    const items = screen.getAllByTestId("command-bar-item");
    expect(items.map((i) => i.textContent)).toEqual([expect.stringContaining("Rename window")]);
  });

  it("question_should_toggleHelpForCurrentLevel", () => {
    render(<CommandBar />);
    press(leader);
    press({ key: "?" });
    expect(screen.getByTestId("command-bar-help")).toBeInTheDocument();
  });

  it("unknownKeys_should_beSwallowedWhileOpen", () => {
    render(<CommandBar />);
    press(leader);
    const ev = press({ key: "F5" });
    expect(ev.defaultPrevented).toBe(true);
    expect(bubble).not.toHaveBeenCalled();
  });

  it("palette_should_virtualiseWhenOverThreshold", () => {
    const many = Array.from({ length: 300 }, (_, i) =>
      registry.register(`t.bulk.${i}`, { context: "global", label: `Bulk action ${i}`, action: () => {} })
    );
    render(<CommandBar />);
    press(leader);
    press({ key: "b" });
    const rendered = screen.getAllByTestId("command-bar-item").length;
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThan(60);
    expect(screen.getAllByTestId("command-bar-item")[0]).toHaveAttribute("aria-setsize", "300");
    many.forEach((c) => c());
  });

  it("idle_should_closeBarAfterThreeSecondsWithNoAction", () => {
    jest.useFakeTimers();
    render(<CommandBar />);
    press(leader);
    expect(screen.getByTestId("command-bar")).toBeInTheDocument();
    act(() => {
      jest.advanceTimersByTime(3000);
    });
    expect(screen.queryByTestId("command-bar")).toBeNull();
    expect(next).not.toHaveBeenCalled();
  });

  it("heldLeader_should_notToggleBarClosedOnAutoRepeat", () => {
    render(<CommandBar />);
    press(leader);
    press({ ...leader, repeat: true });
    expect(screen.getByTestId("command-bar")).toBeInTheDocument();
  });

  it("leaderPressedAgain_should_closeBar", () => {
    render(<CommandBar />);
    press(leader);
    press(leader);
    expect(screen.queryByTestId("command-bar")).toBeNull();
  });

  it("closedTrayDialog_should_notBlockLeader", () => {
    const tray = document.createElement("div");
    tray.setAttribute("role", "dialog");
    tray.setAttribute("data-state", "closed");
    document.body.appendChild(tray);
    render(<CommandBar />);
    press(leader);
    expect(screen.getByTestId("command-bar")).toBeInTheDocument();
    tray.remove();
  });

  it("tab_should_passThroughSoFocusTrapCanCycle", () => {
    render(<CommandBar />);
    press(leader);
    const ev = press({ key: "Tab" });
    expect(ev.defaultPrevented).toBe(false);
  });
});
