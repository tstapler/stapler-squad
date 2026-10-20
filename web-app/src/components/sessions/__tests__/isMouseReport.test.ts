import { isMouseReport } from "../XtermTerminal";

describe("isMouseReport", () => {
  it.each(["\x1b[<35;51;21M", "\x1b[<0;51;21m", "\x1b[M #R"])("matches mouse report %j", (s) => {
    expect(isMouseReport(s)).toBe(true);
  });

  it.each(["a", "\x1b[A", "ls\r", "\x1b[<35;51;21Mx", "\x1b[200~paste"])("passes through %j", (s) => {
    expect(isMouseReport(s)).toBe(false);
  });
});
