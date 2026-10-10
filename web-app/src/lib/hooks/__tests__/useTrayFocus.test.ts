import { shouldRestoreFocus, isTerminalInput, type CloseFocusContext } from "../useTrayFocus";

const base: CloseFocusContext = {
  coarse: false,
  closedBy: "keyboard",
  storedConnected: true,
  storedIsTerminal: true,
  focusInsideTray: true,
};

describe("shouldRestoreFocus", () => {
  it("restores the terminal on a keyboard close with a fine pointer (TY-2, TK-4)", () => {
    expect(shouldRestoreFocus(base)).toBe(true);
  });

  it("never focuses the terminal textarea on a coarse pointer (TK-4)", () => {
    expect(shouldRestoreFocus({ ...base, coarse: true })).toBe(false);
    expect(shouldRestoreFocus({ ...base, coarse: true, closedBy: "pointer" })).toBe(false);
  });

  it("restores a non-terminal opener on a coarse keyboard close", () => {
    expect(shouldRestoreFocus({ ...base, coarse: true, storedIsTerminal: false })).toBe(true);
  });

  it("does not steal focus on a pointer close when focus left the tray", () => {
    expect(shouldRestoreFocus({ ...base, closedBy: "pointer", focusInsideTray: false })).toBe(false);
    expect(shouldRestoreFocus({ ...base, closedBy: "pointer", focusInsideTray: true })).toBe(true);
  });

  it("skips an opener that is gone", () => {
    expect(shouldRestoreFocus({ ...base, storedConnected: false })).toBe(false);
  });
});

describe("isTerminalInput", () => {
  it("matches the xterm helper textarea by class or accessible name", () => {
    const byClass = document.createElement("textarea");
    byClass.className = "xterm-helper-textarea";
    const byName = document.createElement("textarea");
    byName.setAttribute("aria-label", "Terminal input");
    expect(isTerminalInput(byClass)).toBe(true);
    expect(isTerminalInput(byName)).toBe(true);
    expect(isTerminalInput(document.createElement("input"))).toBe(false);
    expect(isTerminalInput(null)).toBe(false);
  });
});
