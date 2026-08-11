// logParser.test.ts — unit tests for the security-critical ANSI pipeline and
// core parsing utilities.
//
// In jsdom (Jest's default environment), `window` is defined so DOMPurify runs.
// The OSC-strip pre-pass and `escapeXML: true` in ansi-to-html are the primary
// XSS defenses tested here; DOMPurify provides an additional layer.

import { detectLevel, segmentText, tryParseJson, renderAnsi } from "../logParser";

// ---------------------------------------------------------------------------
// detectLevel
// ---------------------------------------------------------------------------

describe("detectLevel", () => {
  const cases: Array<[string, string]> = [
    ["2026-01-01 ERROR foo bar", "ERROR"],
    ["[ERR: connection refused]", "ERROR"],
    ["WARN: disk 90% full", "WARN"],
    ["WARNING: rate limit approaching", "WARN"],
    ["INFO server started on port 8543", "INFO"],
    ["DEBUG ← tmux signal received", "DEBUG"],
    ["TRACE ← entering function", "TRACE"],
    ["no level marker here", "UNKNOWN"],
    ["", "UNKNOWN"],
  ];

  test.each(cases)("detectLevel(%j) → %s", (input, expected) => {
    expect(detectLevel(input)).toBe(expected);
  });

  it("is case-insensitive for ERROR", () => {
    expect(detectLevel("error: something went wrong")).toBe("ERROR");
  });

  it("is case-insensitive for WARN", () => {
    expect(detectLevel("warn: approaching limit")).toBe("WARN");
  });

  it("matches ERR as ERROR", () => {
    expect(detectLevel("ERR connection refused")).toBe("ERROR");
  });

  it("matches WARNING as WARN", () => {
    expect(detectLevel("WARNING disk usage high")).toBe("WARN");
  });

  it("prefers ERROR over lower levels when both present", () => {
    // ERROR appears first in LEVEL_PATTERNS, so it wins
    expect(detectLevel("ERROR: info level message included")).toBe("ERROR");
  });
});

// ---------------------------------------------------------------------------
// segmentText
// ---------------------------------------------------------------------------

describe("segmentText", () => {
  it("returns single non-highlighted segment for empty query", () => {
    expect(segmentText("hello world", "")).toEqual([
      { text: "hello world", highlight: false },
    ]);
  });

  it("highlights a single match", () => {
    const result = segmentText("hello world", "world");
    expect(result).toEqual([
      { text: "hello ", highlight: false },
      { text: "world", highlight: true },
    ]);
  });

  it("is case-insensitive", () => {
    const result = segmentText("Hello World", "hello");
    expect(result[0]).toEqual(expect.objectContaining({ highlight: true }));
  });

  it("preserves original casing in highlighted segment", () => {
    const result = segmentText("Hello World", "hello");
    expect(result[0].text).toBe("Hello");
  });

  it("handles query not found", () => {
    expect(segmentText("hello world", "xyz")).toEqual([
      { text: "hello world", highlight: false },
    ]);
  });

  it("handles multiple matches", () => {
    const result = segmentText("error error error", "error");
    const highlighted = result.filter((s) => s.highlight);
    expect(highlighted).toHaveLength(3);
  });

  it("handles match at the start", () => {
    const result = segmentText("ERROR: oops", "ERROR");
    expect(result[0]).toEqual({ text: "ERROR", highlight: true });
  });

  it("handles match at the end", () => {
    const result = segmentText("oh no ERROR", "ERROR");
    const last = result[result.length - 1];
    expect(last).toEqual({ text: "ERROR", highlight: true });
  });

  it("returns non-empty array for empty text and empty query", () => {
    const result = segmentText("", "");
    expect(result).toHaveLength(1);
    expect(result[0].text).toBe("");
  });

  it("handles adjacent matches (no gap between highlights)", () => {
    const result = segmentText("abab", "ab");
    const highlighted = result.filter((s) => s.highlight);
    expect(highlighted).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// tryParseJson
// ---------------------------------------------------------------------------

describe("tryParseJson", () => {
  it("returns parsed object for valid JSON object", () => {
    const result = tryParseJson('{"key":"value","n":42}');
    expect(result).toEqual({ key: "value", n: 42 });
  });

  it("returns null for plain text", () => {
    expect(tryParseJson("plain text log")).toBeNull();
  });

  it("returns null for invalid JSON", () => {
    expect(tryParseJson("{key: value}")).toBeNull();
  });

  it("returns null for JSON primitives (strings)", () => {
    expect(tryParseJson('"just a string"')).toBeNull();
  });

  it("returns null for JSON primitives (numbers)", () => {
    expect(tryParseJson("42")).toBeNull();
  });

  it("returns null for JSON primitives (booleans)", () => {
    expect(tryParseJson("true")).toBeNull();
  });

  it("returns parsed array for valid JSON array", () => {
    const result = tryParseJson("[1,2,3]");
    expect(result).toEqual([1, 2, 3]);
  });

  it("handles nested JSON object", () => {
    const result = tryParseJson('{"a":{"b":1}}');
    expect(result).toEqual({ a: { b: 1 } });
  });

  it("trims leading/trailing whitespace before parsing", () => {
    const result = tryParseJson('  {"key":"val"}  ');
    expect(result).toEqual({ key: "val" });
  });

  it("returns null for empty string", () => {
    expect(tryParseJson("")).toBeNull();
  });

  it("caches repeated calls (same result, no throw on re-call)", () => {
    const input = '{"cached":true}';
    const first = tryParseJson(input);
    const second = tryParseJson(input);
    expect(first).toEqual(second);
    expect(second).toEqual({ cached: true });
  });
});

// ---------------------------------------------------------------------------
// renderAnsi — security tests (jsdom context, DOMPurify active)
//
// jsdom defines `window`, so DOMPurify.sanitize runs in the test environment.
// The critical security layers tested here are:
//   1. OSC stripping — runs unconditionally before ansi-to-html
//   2. ansi-to-html escapeXML:true — escapes < and > in log content
//   3. DOMPurify — additional sanitization layer (active in jsdom)
// ---------------------------------------------------------------------------

describe("renderAnsi security (Node/jsdom context)", () => {
  it("does not throw on empty string", () => {
    expect(() => renderAnsi("")).not.toThrow();
  });

  it("returns a string", () => {
    expect(typeof renderAnsi("hello")).toBe("string");
  });

  it("does not throw on truncated ANSI sequence", () => {
    expect(() => renderAnsi("text\x1b[31")).not.toThrow();
  });

  it("strips OSC 8 hyperlink sequences (BEL terminator) before processing", () => {
    // OSC 8 hyperlink: \x1b]8;;javascript:alert(1)\x07text\x1b]8;;\x07
    const input = "\x1b]8;;javascript:alert(1)\x07click me\x1b]8;;\x07";
    const result = renderAnsi(input);
    expect(result).not.toContain("javascript:");
    // The visible text "click me" may or may not be preserved depending on
    // ansi-to-html behaviour after stripping, but the dangerous href is gone.
  });

  it("strips OSC 8 hyperlink sequences (ST terminator) before processing", () => {
    // ESC-backslash (string terminator) form
    const input = "\x1b]8;;https://evil.example\x1b\\link text\x1b]8;;\x1b\\";
    const result = renderAnsi(input);
    expect(result).not.toContain("evil.example");
  });

  it("strips OSC sequences with arbitrary parameters", () => {
    const input = "\x1b]0;window title\x07normal text";
    const result = renderAnsi(input);
    expect(result).not.toContain("window title");
  });

  it("does not contain raw script tags from XSS payload (escapeXML:true)", () => {
    // ansi-to-html with escapeXML:true escapes < and > in the input
    const result = renderAnsi("<script>alert(1)</script>");
    expect(result).not.toContain("<script>");
  });

  it("does not contain unescaped angle brackets from XSS payload", () => {
    const result = renderAnsi("<img src=x onerror=alert(1)>");
    // escapeXML:true converts < to &lt;
    expect(result).not.toMatch(/<img\s/i);
  });

  it("handles a valid ANSI color sequence without throwing", () => {
    // \x1b[31m = red; \x1b[0m = reset
    expect(() => renderAnsi("\x1b[31mred text\x1b[0m")).not.toThrow();
  });

  it("processes plain text (no ANSI) without modification", () => {
    const result = renderAnsi("plain log line");
    expect(result).toContain("plain log line");
  });
});
