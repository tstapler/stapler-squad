import { selectTrayVariant, trayAffordanceVariant } from "../trayVariant";

describe("selectTrayVariant", () => {
  it.each([
    [{ isInnerScreen: true, isVirtualKeyboardOpen: false }, "side-overlay"],
    [{ isInnerScreen: false, isVirtualKeyboardOpen: false }, "bottom-sheet"],
    [{ isInnerScreen: false, isVirtualKeyboardOpen: true }, "bottom-sheet"],
  ] as const)("%j -> %s", (viewport, expected) => {
    expect(selectTrayVariant(viewport)).toBe(expected);
  });
});

describe("trayAffordanceVariant (overflow, toasts, keyboard, hasTerminal)", () => {
  it.each([
    // [overflow, toasts, keyboard, hasTerminal, expected]
    [0, 0, false, true, "bell"],
    [0, 1, false, true, "bell"], // exactly 1 toast, 0 overflow: the bell is the only entry
    [2, 3, false, true, "more-row"],
    [1, 1, false, true, "more-row"],
    [0, 0, true, true, "bell"], // keyboard open with nothing to say keeps the bell
    [3, 3, true, true, "keyboard-chip"],
    [0, 1, true, true, "keyboard-chip"],
    [0, 0, false, false, "floating-bottom"],
    [4, 4, true, false, "floating-bottom"],
  ] as const)("(%i, %i, kb=%s, term=%s) -> %s", (overflow, toasts, keyboard, terminal, expected) => {
    expect(trayAffordanceVariant(overflow, toasts, keyboard, terminal)).toBe(expected);
  });

  it("never returns a position, only content, on a session page", () => {
    const contents = new Set<string>();
    for (const overflow of [0, 1, 5]) {
      for (const toasts of [0, 1, 3]) {
        for (const keyboard of [false, true]) contents.add(trayAffordanceVariant(overflow, toasts, keyboard, true));
      }
    }
    expect([...contents].sort()).toEqual(["bell", "keyboard-chip", "more-row"]);
  });
});
