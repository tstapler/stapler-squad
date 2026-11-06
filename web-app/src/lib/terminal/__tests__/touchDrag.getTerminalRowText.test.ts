import { getTerminalRowText, computeMagnifierPosition, type RowTextSource } from "../touchDrag";

const LINE = Array.from({ length: 80 }, (_, i) => String.fromCharCode(97 + (i % 26))).join("");

function makeTerm(opts: { viewportY?: number; lines?: Record<number, string | undefined>; cols?: number } = {}): RowTextSource {
  const lines = opts.lines ?? { 0: LINE };
  return {
    cols: opts.cols ?? 80,
    buffer: {
      active: {
        viewportY: opts.viewportY ?? 0,
        getLine: (row: number) => {
          const s = lines[row];
          return s === undefined
            ? undefined
            : { translateToString: (_trim: boolean, start = 0, end = s.length) => s.slice(start, end) };
        },
      },
    },
  };
}

describe("getTerminalRowText", () => {
  it("returns a centered window for a mid-row column", () => {
    const r = getTerminalRowText(makeTerm(), 0, 40);
    expect(r.text).toBe(LINE.slice(30, 50));
    expect(r.cursorOffset).toBe(10);
    expect(r.text[r.cursorOffset]).toBe(LINE[40]);
  });

  it("clamps the window start at column 0", () => {
    const r = getTerminalRowText(makeTerm(), 0, 2);
    expect(r.text).toBe(LINE.slice(0, 20));
    expect(r.cursorOffset).toBe(2);
  });

  it("clamps the window end at terminal.cols", () => {
    const r = getTerminalRowText(makeTerm(), 0, 78);
    expect(r.text).toBe(LINE.slice(68, 80));
    expect(r.cursorOffset).toBe(10);
  });

  it("returns blank padding when the buffer line is missing", () => {
    const r = getTerminalRowText(makeTerm({ lines: {} }), 3, 40);
    expect(r.text).toBe(" ".repeat(20));
    expect(r.cursorOffset).toBe(10);
  });

  it("pads a short row to the full window", () => {
    const r = getTerminalRowText(makeTerm({ lines: { 0: "abc" } }), 0, 5);
    expect(r.text).toBe("abc".padEnd(20, " "));
    expect(r.cursorOffset).toBe(5);
  });

  it("reads buffer row viewportRow + viewportY", () => {
    const term = makeTerm({ viewportY: 100, lines: { 0: "wrong", 102: LINE.toUpperCase() } });
    const r = getTerminalRowText(term, 2, 40);
    expect(r.text).toBe(LINE.toUpperCase().slice(30, 50));
  });
});

describe('computeMagnifierPosition', () => {
  it.each([
    { name: 'centered above touch', x: 400, y: 300, vw: 800, left: 300, top: 212 },
    { name: 'clamps to left edge', x: 10, y: 300, vw: 800, left: 8, top: 212 },
    { name: 'clamps to right edge', x: 790, y: 300, vw: 800, left: 592, top: 212 },
    { name: 'clamps to top edge', x: 400, y: 20, vw: 800, left: 300, top: 8 },
    { name: 'viewport narrower than bubble', x: 100, y: 300, vw: 150, left: 8, top: 212 },
  ])('$name', ({ x, y, vw, left, top }) => {
    expect(computeMagnifierPosition(x, y, vw)).toEqual({ left, top });
  });
});
