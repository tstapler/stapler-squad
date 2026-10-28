/** jest maps `.css.ts` to a proxy mock, so (like SessionCard.css.test.ts) this reads the source as text. */
import * as fs from "fs";
import * as path from "path";

describe("WindowPanel.css.ts", () => {
  it("windowPanel_should_BeDisplayNone_When_HiddenAttributeSetByKeepAliveHost", () => {
    const source = fs.readFileSync(path.resolve(__dirname, "../WindowPanel.css.ts"), "utf8");
    expect(source).toMatch(/"&\[hidden\]":\s*\{\s*display:\s*"none"\s*\}/);
  });
});
