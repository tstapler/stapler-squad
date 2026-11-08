import { createSoftResetRewriter, softResetSequence } from "../ghosttySoftReset";

const E = "\x1b";

function run(chunks: string[], cursor = { x: 4, y: 2 }): string[] {
  const rewriter = createSoftResetRewriter();
  const writes: string[] = [];
  for (const c of chunks) rewriter.feed(c, (d) => writes.push(d), () => cursor);
  return writes;
}

describe("createSoftResetRewriter", () => {
  it("passes text without DECSTR through unchanged", () => {
    expect(run([`plain ${E}[31mred${E}[0m`])).toEqual([`plain ${E}[31mred${E}[0m`]);
  });

  it("replaces DECSTR and restores the cursor position absolutely", () => {
    const writes = run([`before${E}[!pafter`]);
    expect(writes).toEqual(["before", softResetSequence({ x: 4, y: 2 }), "after"]);
    expect(writes[1]).toMatch(/\x1b\[3;5H$/);
  });

  it("handles several DECSTRs in one chunk", () => {
    expect(run([`${E}[!p${E}[!p`])).toHaveLength(2);
  });

  it("carries a DECSTR split at every byte boundary", () => {
    for (let cut = 1; cut < 4; cut++) {
      const seq = `${E}[!p`;
      const writes = run([`a${seq.slice(0, cut)}`, `${seq.slice(cut)}b`]);
      expect(writes).toEqual(["a", softResetSequence({ x: 4, y: 2 }), "b"]);
    }
  });

  it("never writes an empty string (ghostty-web write('') throws)", () => {
    expect(run([E, "[", "!"])).toEqual([]);
    expect(run([`${E}[!p`])).toEqual([softResetSequence({ x: 4, y: 2 })]);
  });

  it("releases a held-back prefix that turns out not to be DECSTR", () => {
    expect(run([E, `[31mx`])).toEqual([`${E}[31mx`]);
  });

  it("leaves other CSI sequences with a bang alone", () => {
    expect(run([`${E}[!q${E}[1!p`])).toEqual([`${E}[!q${E}[1!p`]);
  });
});
