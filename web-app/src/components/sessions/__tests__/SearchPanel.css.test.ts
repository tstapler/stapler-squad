/** jest maps `.css.ts` to a proxy mock, so (like WindowPanel.css.test.ts) this reads the source as text. */
import * as fs from "fs";
import * as path from "path";

const source = fs.readFileSync(path.resolve(__dirname, "../files/SearchPanel.css.ts"), "utf8");

describe("SearchPanel.css.ts", () => {
  it("tree_should_KeepMinHeight_So_VirtualizerRendersRowsWhenFiltersFillThePanel", () => {
    const tree = source.slice(source.indexOf("export const tree "), source.indexOf("export const treeDimmed"));
    expect(tree).toMatch(/minHeight:\s*"72px"/);
    expect(tree).toMatch(/"\(pointer: coarse\)":\s*\{\s*minHeight:\s*"132px"/);
  });

  it("panel_should_Scroll_When_FormPlusMinTreeExceedsThePane", () => {
    const panel = source.slice(source.indexOf("export const panel "), source.indexOf("export const header"));
    expect(panel).toMatch(/overflowY:\s*"auto"/);
  });
});
