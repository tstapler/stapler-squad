import { readFileSync } from "fs";
import { join } from "path";

const PURE_FILES = ["composition.ts", "rewrite.ts", "keymap.ts", "deliveryLedger.ts", "inputEngine.ts", "deferredIndicator.ts", "physicalKeys.ts"];
const FORBIDDEN = /^(react|react-dom|@xterm\/|ghostty|@\/lib\/terminal\/(?!modifierKeys)|\.\.\/(?!modifierKeys)|\.\/(attachMobileInput|inputSource|normalize|inputElement|tapTarget))/;

/** Module specifiers of every static import / re-export in `source`. */
export function importSpecifiers(source: string): string[] {
  return [...source.matchAll(/(?:^|\n)\s*(?:import|export)\s[^;]*?from\s+["']([^"']+)["']/g)].map((m) => m[1]);
}

const DOM_GLOBALS = /\b(document|window|HTMLElement|HTMLTextAreaElement|addEventListener|requestAnimationFrame|navigator)\b/;

describe("import graph", () => {
  it.each(PURE_FILES)("mobileInputModules_should_NotImportXtermOrGhosttyOrDom_When_ImportGraphScanned: %s", (file) => {
    const source = readFileSync(join(__dirname, file), "utf8");
    const bad = importSpecifiers(source).filter((s) => FORBIDDEN.test(s));
    expect({ file, bad }).toEqual({ file, bad: [] });
    const code = source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    expect({ file, dom: DOM_GLOBALS.exec(code)?.[0] ?? null }).toEqual({ file, dom: null });
  });

  it("scan_should_FlagViolation_When_NegativeFixtureImportsRendererOrDom", () => {
    const bad = `import { Terminal } from "@xterm/xterm";\nimport x from "./attachMobileInput";\nexport { y } from "ghostty-web";\n`;
    const found = importSpecifiers(bad).filter((s) => FORBIDDEN.test(s));
    expect(found).toEqual(["@xterm/xterm", "./attachMobileInput", "ghostty-web"]);
    expect(DOM_GLOBALS.test("const d = document.body;")).toBe(true);
  });

  it("scan_should_AllowSiblingPureModules_When_ImportingTypesAndPolicy", () => {
    const ok = `import { reduce } from "./composition";\nimport { x } from "../modifierKeys";\n`;
    expect(importSpecifiers(ok).filter((s) => FORBIDDEN.test(s))).toEqual([]);
  });
});
