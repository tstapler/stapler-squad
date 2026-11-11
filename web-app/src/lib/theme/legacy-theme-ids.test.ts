import { LEGACY_THEME_IDS, parseThemeId } from "./legacy-theme-ids";

describe("parseThemeId", () => {
  it("maps the legacy id to grimdark", () => {
    expect(parseThemeId("wh40k")).toBe("grimdark");
  });

  it.each(["grimdark", "clean", "custom:x", "nope", ""])("passes %p through unchanged", (id) => {
    expect(parseThemeId(id)).toBe(id);
  });

  it("does not resolve inherited object keys", () => {
    expect(parseThemeId("constructor")).toBe("constructor");
    expect(parseThemeId("__proto__")).toBe("__proto__");
  });

  it("only aliases onto canonical ids (no chains)", () => {
    for (const target of Object.values(LEGACY_THEME_IDS)) {
      expect(Object.hasOwn(LEGACY_THEME_IDS, target)).toBe(false);
    }
  });
});
