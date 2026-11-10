import { readFileSync } from "fs";
import path from "path";
import { THEME_NAMES } from "../../styles/theme-boot-script";

const repoRoot = path.resolve(__dirname, "../../../..");
const read = (rel: string) => readFileSync(path.join(repoRoot, rel), "utf8");
const quoted = (text: string) => [...text.matchAll(/"([a-z0-9]+)"/g)].map((m) => m[1]).sort();

describe("built-in theme id parity", () => {
  const expected = [...THEME_NAMES].sort();

  it("includes grimdark", () => {
    expect(expected).toContain("grimdark");
  });

  it("ThemeName in ThemeContext.tsx matches THEME_NAMES", () => {
    const union = /export type ThemeName =([^;]+);/.exec(read("web-app/src/lib/contexts/ThemeContext.tsx"));
    expect(union).not.toBeNull();
    expect(quoted(union![1])).toEqual(expected);
  });

  it("server/user_themes.go builtinThemeBases matches THEME_NAMES", () => {
    const map = /builtinThemeBases\s*=\s*map\[string\]bool\{([^}]+)\}/.exec(read("server/user_themes.go"));
    expect(map).not.toBeNull();
    expect(quoted(map![1])).toEqual(expected);
  });

  it("the e2e parity harness theme list matches THEME_NAMES", () => {
    const list = /THEME_NAMES = \[([^\]]+)\]/.exec(read("tests/e2e/tools/parity-collect.ts"));
    expect(list).not.toBeNull();
    expect([...list![1].matchAll(/'([a-z0-9]+)'/g)].map((m) => m[1]).sort()).toEqual(expected);
  });
});
