import { parseExternalRef } from "./externalRef";

describe("parseExternalRef", () => {
  it("parses a github.com issue URL", () => {
    expect(parseExternalRef("https://github.com/tstapler/stapler-mcp/issues/22")).toEqual({
      repo: "tstapler/stapler-mcp",
      number: "22",
    });
  });

  it("parses a pull URL and tolerates trailing slash, query and fragment", () => {
    expect(parseExternalRef("https://github.com/o/r/pull/7/?x=1#issuecomment-2")).toEqual({
      repo: "o/r",
      number: "7",
    });
  });

  it("parses a GHE host", () => {
    expect(parseExternalRef("https://ghe.example.com/acme/widget/issues/5")).toEqual({
      repo: "acme/widget",
      number: "5",
    });
  });

  it.each([
    undefined,
    "",
    "not a url",
    "https://github.com/acme/widget",
    "https://github.com/acme/widget/issues/abc",
    "https://github.com/acme/widget/issues/42/extra",
    "https://github.com/acme/widget/discussions/42",
    "ftp://github.com/acme/widget/issues/42",
  ])("returns undefined for %p", (input) => {
    expect(parseExternalRef(input)).toBeUndefined();
  });
});
