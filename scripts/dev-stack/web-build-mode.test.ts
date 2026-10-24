import { parseWebBuildMode } from "./web-build-mode";

describe("parseWebBuildMode", () => {
  it("parses next and bun", () => {
    expect(parseWebBuildMode({ WEB_BUILD: "next" })).toBe("next");
    expect(parseWebBuildMode({ WEB_BUILD: "bun" })).toBe("bun");
  });

  it("defaults to next when unset or empty", () => {
    expect(parseWebBuildMode({})).toBe("next");
    expect(parseWebBuildMode({ WEB_BUILD: "" })).toBe("next");
  });

  it("throws naming the allowed values for anything else", () => {
    expect(() => parseWebBuildMode({ WEB_BUILD: "vite" })).toThrow(/next, bun/);
    expect(() => parseWebBuildMode({ WEB_BUILD: "Bun" })).toThrow(/"Bun"/);
  });
});
