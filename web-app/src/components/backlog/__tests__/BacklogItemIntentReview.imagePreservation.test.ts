import { extractImageMarkdown } from "../BacklogItemIntentReview";

describe("extractImageMarkdown", () => {
  it("returns every image reference verbatim", () => {
    const text = "see ![a](/api/local/serve/1.png) and ![b c](/api/local/serve/2.png) done";
    expect(extractImageMarkdown(text)).toEqual(["![a](/api/local/serve/1.png)", "![b c](/api/local/serve/2.png)"]);
  });

  it("returns nothing when there are no images", () => {
    expect(extractImageMarkdown("plain [link](http://x) text")).toEqual([]);
  });
});
