/**
 * Source-level checks for SessionCard.css.ts (session-list-density Epic 2.2).
 *
 * jest.config.js maps every `.css.ts` import to a proxy mock (styleMock.js),
 * so vanilla-extract's `style()` never actually runs under Jest and there is
 * no way to introspect a real, compiled CSS rule from a component test.
 * These tests instead read the source file as text — the same "lightweight
 * snapshot" approach `SessionRow.css.test.ts` and
 * `styles/__tests__/sessionDetailTokens.test.ts` use for a similar
 * banned/required-property check — as a stand-in for the pixel-level
 * assertions validation.md pushes to the Playwright e2e suite.
 */
import * as fs from "fs";
import * as path from "path";

const SESSION_CARD_CSS = path.resolve(__dirname, "./SessionCard.css.ts");

function extractBlock(source: string, exportName: string): string {
  const start = source.indexOf(`export const ${exportName} = style({`);
  if (start === -1) {
    throw new Error(`Could not find "export const ${exportName} = style({" in SessionCard.css.ts`);
  }
  const end = source.indexOf("\n});", start);
  if (end === -1) {
    throw new Error(`Could not find closing "});" for ${exportName} in SessionCard.css.ts`);
  }
  return source.slice(start, end);
}

describe("SessionCard.css.ts — container-query narrow layout (Epic 2.2 Story 2.2.2)", () => {
  let source: string;

  beforeAll(() => {
    source = fs.readFileSync(SESSION_CARD_CSS, "utf-8");
  });

  it("SessionCard_should_ApplyContainerQueryProperties_When_RootRendered", () => {
    const cardBlock = extractBlock(source, "card");
    expect(cardBlock).toContain('containerType: "inline-size"');
    expect(cardBlock).toContain('containerName: "sessionCard"');

    expect(source).toMatch(/CARD_NARROW = "\(max-width: \d+px\)"/);
    expect(source).toContain('"@container"');

    const infoRowBlock = extractBlock(source, "infoRow");
    expect(infoRowBlock).toContain('[`sessionCard ${CARD_NARROW}`]');

    const valueBlock = extractBlock(source, "value");
    expect(valueBlock).toContain('[`sessionCard ${CARD_NARROW}`]');
  });
});
