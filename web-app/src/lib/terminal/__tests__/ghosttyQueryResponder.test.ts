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

describe("createQueryResponder on large input", () => {
  it("handles a multi-megabyte replay chunk without overflowing the stack", () => {
    const r = createQueryResponder();
    const big = "\x1b[0m\x1b[31mline of text\x1b[0m\r\n".repeat(200_000);
    expect(r.feed(`${big}\x1b[?u${big}`)).toBe("\x1b[?0u");
  });

  it("does not carry over a long digit run indefinitely", () => {
    const r = createQueryResponder();
    expect(r.feed(`\x1b[?${"1".repeat(40)}`)).toBe("");
    expect(r.feed("$p\x1b[?u")).toBe("\x1b[?0u");
  });
});
