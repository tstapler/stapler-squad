/** @jest-environment node */
import type { Ghostty } from "ghostty-web";
import { describeConformance } from "./runBaseline";
import { createQueryResponder } from "../../ghosttyQueryResponder";
import { normalizeScreen, type Vector } from "./vectors";

let ghostty: Ghostty;
beforeAll(async () => {
  // The UMD build reads `self`, which a node test environment lacks.
  (globalThis as { self?: unknown }).self ??= globalThis;
  ghostty = await (await import("ghostty-web")).Ghostty.load();
});

// Headless: the WASM terminal alone, no canvas — it is the same parser the browser renderer drives.
async function renderGhostty(v: Vector) {
  const term = ghostty.createTerminal(v.cols, v.rows, { scrollbackLimit: 100 });
  const responder = createQueryResponder();
  term.write(v.input);
  term.update();
  let response = responder.feed(v.input);
  while (term.hasResponse()) response += term.readResponse() ?? "";
  const rows = Array.from({ length: v.rows }, (_, y) =>
    (term.getLine(y) ?? [])
      .map((c, x) => (c.width === 0 ? "" : c.grapheme_len > 0 ? term.getGraphemeString(y, x) : c.codepoint ? String.fromCodePoint(c.codepoint) : " "))
      .join(""));
  term.free();
  return { screen: normalizeScreen(rows), response };
}

describe("VT conformance: ghostty-web", () => {
  describeConformance("ghostty", renderGhostty);
});
