import { cursorCellRect, readTerminalCursor, registerTerminalCursorSource } from "@/lib/terminal/cursorRect";

describe("cursorRect", () => {
  it("reports no terminal until one registers, then its rectangle, then no terminal again", () => {
    expect(readTerminalCursor()).toEqual({ kind: "no-terminal" });
    const unregister = registerTerminalCursorSource(() => ({ left: 1, top: 2, width: 8, height: 16 }));
    expect(readTerminalCursor()).toEqual({ kind: "rect", rect: { left: 1, top: 2, width: 8, height: 16 } });
    unregister();
    expect(readTerminalCursor()).toEqual({ kind: "no-terminal" });
  });

  it("reports unreadable when a terminal is registered but hidden", () => {
    const unregister = registerTerminalCursorSource(() => null);
    expect(readTerminalCursor()).toEqual({ kind: "unreadable" });
    unregister();
  });

  it("derives the cursor cell from the screen box and cell grid", () => {
    const screen = { getBoundingClientRect: () => ({ left: 100, top: 50, width: 800, height: 400 }) } as unknown as Element;
    const terminal = { cols: 80, rows: 25, buffer: { active: { cursorX: 10, cursorY: 24 } } };
    expect(cursorCellRect(terminal, screen)).toEqual({ left: 200, top: 50 + 24 * 16, width: 10, height: 16 });
  });

  it("returns null for a zero-size (hidden) screen or missing element", () => {
    const hidden = { getBoundingClientRect: () => ({ left: 0, top: 0, width: 0, height: 0 }) } as unknown as Element;
    const terminal = { cols: 80, rows: 25, buffer: { active: { cursorX: 0, cursorY: 0 } } };
    expect(cursorCellRect(terminal, hidden)).toBeNull();
    expect(cursorCellRect(terminal, null)).toBeNull();
  });
});
