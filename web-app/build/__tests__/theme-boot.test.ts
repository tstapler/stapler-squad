/** @jest-environment jsdom */
import { buildThemeBootScript, THEME_NAMES, type ThemeMap } from "../../src/styles/theme-boot-script";
import { buildThemeMap } from "../theme-map";
import type { VeCompiler } from "../plugins/vanilla-extract";

const MAP: ThemeMap = { matrix: "m1", cyberpunk77: "c1", wh40k: "w1", clean: "cl", light: "l1", dark: "d1" };

function run(setup: () => void): DOMTokenList {
  document.documentElement.className = "cl font-x";
  localStorage.clear();
  setup();
  new Function(buildThemeBootScript(MAP))();
  return document.documentElement.classList;
}

describe("theme boot script", () => {
  it("applies the stored theme and leaves unrelated classes alone", () => {
    const cls = run(() => localStorage.setItem("stapler-theme", "matrix"));
    expect(cls.contains("m1")).toBe(true);
    expect(cls.contains("cl")).toBe(false);
    expect(cls.contains("font-x")).toBe(true);
  });

  it("applies a custom theme class and its CSS variables", () => {
    const cls = run(() => {
      localStorage.setItem("stapler-theme", "custom:42");
      localStorage.setItem("stapler-theme-custom", JSON.stringify({ id: "custom:42", cls: "cust42", props: { "--x": "red" } }));
    });
    expect(cls.contains("cust42")).toBe(true);
    expect(document.documentElement.style.getPropertyValue("--x")).toBe("red");
  });

  it("degrades to clean for a stale stored theme", () => {
    const cls = run(() => localStorage.setItem("stapler-theme", "retired-theme"));
    expect(cls.contains("cl")).toBe(true);
  });

  it("applies clean without throwing when localStorage throws", () => {
    document.documentElement.className = "";
    const spy = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(() => new Function(buildThemeBootScript(MAP))()).not.toThrow();
    spy.mockRestore();
    expect(document.documentElement.classList.contains("cl")).toBe(true);
  });

  it("is deterministic, so Next layout.tsx and html.ts inline identical text for one map", () => {
    expect(buildThemeBootScript(MAP)).toBe(buildThemeBootScript({ ...MAP }));
  });
});

describe("buildThemeMap", () => {
  it("returns the six names, and names the missing export when one is absent", async () => {
    const exportsOf = (o: Record<string, string>) => ({ classExports: async () => o }) as unknown as VeCompiler;
    const full = Object.fromEntries(THEME_NAMES.map((n) => [`${n}Theme`, `c-${n}`]));
    expect(await buildThemeMap(exportsOf(full), "t.css.ts")).toEqual(Object.fromEntries(THEME_NAMES.map((n) => [n, `c-${n}`])));
    delete full.darkTheme;
    await expect(buildThemeMap(exportsOf(full), "t.css.ts")).rejects.toThrow(/darkTheme/);
  });
});
