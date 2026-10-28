import { LezerHighlighter } from "../LezerHighlighter";

const range = (text: string) => ({ startLine: 1, endLine: 1, text });

describe("LezerHighlighter", () => {
  const h = new LezerHighlighter();

  it("classifies Go keywords and strings", async () => {
    const text = 'package main\nvar s = "hi"';
    const spans = await h.tokenize(range(text), "go");
    expect(spans.some((s) => s.className.includes("ssq-tok-keyword") && text.slice(s.from, s.to) === "package")).toBe(true);
    expect(spans.some((s) => s.className.includes("ssq-tok-string") && text.slice(s.from, s.to) === '"hi"')).toBe(true);
  });

  it("returns [] for an unknown language", async () => {
    expect(await h.tokenize(range("anything"), "text")).toEqual([]);
  });

  it("returns [] for empty text", async () => {
    expect(await h.tokenize(range(""), "go")).toEqual([]);
  });
});
