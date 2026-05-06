import { toSessionSlug } from "./slugify";

describe("toSessionSlug", () => {
  it("converts spaces to hyphens", () => {
    expect(toSessionSlug("my feature branch")).toBe("my-feature-branch");
  });

  it("lowercases input", () => {
    expect(toSessionSlug("MY FEATURE")).toBe("my-feature");
  });

  it("strips non-alphanumeric characters", () => {
    expect(toSessionSlug("fix: auth bug!")).toBe("fix-auth-bug");
  });

  it("collapses multiple hyphens", () => {
    expect(toSessionSlug("foo---bar")).toBe("foo-bar");
  });

  it("trims leading and trailing hyphens", () => {
    expect(toSessionSlug("-hello-")).toBe("hello");
  });

  it("returns empty string for all-special input", () => {
    expect(toSessionSlug("!!!")).toBe("");
  });

  it("returns empty string for emoji-only input", () => {
    expect(toSessionSlug("🚀")).toBe("");
  });

  it("strips emoji and slugifies remaining text", () => {
    expect(toSessionSlug("🚀 launch")).toBe("launch");
  });

  it("returns empty string for CJK-only input", () => {
    expect(toSessionSlug("中文")).toBe("");
  });

  it("returns empty string for whitespace-only input", () => {
    expect(toSessionSlug("   ")).toBe("");
  });

  it("strips shell special characters without injection risk", () => {
    expect(toSessionSlug("$(rm -rf /)")).toBe("rm-rf");
  });

  it("truncates at 60 characters", () => {
    const long = "a".repeat(70);
    expect(toSessionSlug(long).length).toBeLessThanOrEqual(60);
  });

  it("does not leave trailing hyphen after truncation", () => {
    const slug = toSessionSlug("word-".repeat(15));
    expect(slug.endsWith("-")).toBe(false);
  });

  it("handles leading digits (valid but unusual)", () => {
    expect(toSessionSlug("123 fix auth")).toBe("123-fix-auth");
  });
});
