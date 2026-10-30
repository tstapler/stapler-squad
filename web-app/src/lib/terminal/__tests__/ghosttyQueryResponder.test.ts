import { createQueryResponder } from "../ghosttyQueryResponder";

describe("createQueryResponder", () => {
  it("answers the Kitty keyboard query with flags 0", () => {
    expect(createQueryResponder().feed("a\x1b[?ub")).toBe("\x1b[?0u");
  });

  it("reports synchronized output as reset, then set after ?2026h", () => {
    const r = createQueryResponder();
    expect(r.feed("\x1b[?2026$p")).toBe("\x1b[?2026;2$y");
    r.feed("\x1b[?2026h");
    expect(r.feed("\x1b[?2026$p")).toBe("\x1b[?2026;1$y");
    r.feed("\x1b[?2026l");
    expect(r.feed("\x1b[?2026$p")).toBe("\x1b[?2026;2$y");
  });

  it("matches a query split across chunks exactly once", () => {
    const r = createQueryResponder();
    expect(r.feed("x\x1b[?20")).toBe("");
    expect(r.feed("26$p")).toBe("\x1b[?2026;2$y");
    expect(r.feed("\x1b")).toBe("");
    expect(r.feed("[?u")).toBe("\x1b[?0u");
  });

  it("ignores other modes and unrelated escapes", () => {
    expect(createQueryResponder().feed("\x1b[?1049h\x1b[2J\x1b[?25l")).toBe("");
  });
});
