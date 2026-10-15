/**
 * Static source assertions for overscroll hardening (REQ-7). jest maps both
 * *.css and *.css.ts to a style mock, so computed styles are unavailable;
 * real behavior is covered by device checklist D4.
 */
import { readFileSync } from "fs";
import { join } from "path";

const read = (rel: string) => readFileSync(join(__dirname, rel), "utf8");

describe("overscroll hardening CSS", () => {
  it("globalCss_should_SetOverscrollBehaviorNoneOnHtmlBody", () => {
    const css = read("../../../app/globals.css");
    const rule = /html\s*,\s*body\s*\{([^}]*)\}/g;
    const bodies = [...css.matchAll(rule)].map((m) => m[1]);
    expect(bodies.some((b) => /overscroll-behavior\s*:\s*none\s*;/.test(b))).toBe(true);
  });

  it("terminalCss_should_SetOverscrollBehaviorContainAndTouchActionNone", () => {
    const src = read("../XtermTerminal.css.ts");
    const match = src.match(/export const terminal = style\(\{([\s\S]*?)\n\}\);/);
    expect(match).not.toBeNull();
    const block = match![1];
    expect(block).toMatch(/overscrollBehavior:\s*"contain"/);
    expect(block).toMatch(/\[data-gesture-scroll="on"\]'?\s*:\s*\{\s*touchAction:\s*"none"/);
  });

  it("terminalCss_should_RestoreDefaultTouchAction_When_GestureScrollOff", () => {
    const src = read("../XtermTerminal.css.ts");
    const block = src.match(/export const terminal = style\(\{([\s\S]*?)\n\}\);/)![1];
    // No unconditional touchAction outside the "on" selector, and none keyed to "off".
    const withoutSelectors = block.replace(/selectors:\s*\{[\s\S]*?\n  \},/, "");
    expect(withoutSelectors).not.toMatch(/touchAction/);
    expect(block).not.toMatch(/data-gesture-scroll="off"/);
  });
});
