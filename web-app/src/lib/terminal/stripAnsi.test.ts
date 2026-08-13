import { stripAnsi } from "./stripAnsi";

describe("stripAnsi", () => {
  it("stripAnsi_should_returnPlainText_When_noEscapeSequences", () => {
    expect(stripAnsi("Hello, world!")).toBe("Hello, world!");
  });

  it("stripAnsi_should_removeSGRCodes_When_letterTerminated", () => {
    expect(stripAnsi("\x1b[31mError\x1b[0m")).toBe("Error");
  });

  it("stripAnsi_should_removeOSCHyperlink_When_terminatedByBEL", () => {
    expect(stripAnsi("\x1b]8;;https://example.com\x07link\x1b]8;;\x07")).toBe("link");
  });

  it("stripAnsi_should_stripInsertCharacterSequence_When_terminatedByAtSign", () => {
    // BUG-025: CSI final byte range is 0x40-0x7E per ECMA-48, not just letters.
    expect(stripAnsi("\x1b[5@Hello")).toBe("Hello");
  });

  it("stripAnsi_should_stripSequence_When_terminatedByTilde", () => {
    expect(stripAnsi("\x1b[3~Hello")).toBe("Hello");
  });
});
