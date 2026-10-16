/**
 * Source-level checks for SessionRow.css.ts (session-list-density Epic 2.1).
 *
 * jest.config.js maps every `.css.ts` import to a proxy mock (styleMock.js),
 * so vanilla-extract's `style()` never actually runs under Jest and there is
 * no way to introspect a real, compiled CSS rule from a component test.
 * These tests instead read the source file as text — the same "lightweight
 * snapshot" approach `styles/__tests__/sessionDetailTokens.test.ts` uses for
 * a similar banned/required-property check — as a stand-in for the pixel-
 * level assertions validation.md pushes to the Playwright e2e suite.
 */
import * as fs from "fs";
import * as path from "path";

const SESSION_ROW_CSS = path.resolve(__dirname, "./SessionRow.css.ts");

function extractBlock(source: string, exportName: string): string {
  const start = source.indexOf(`export const ${exportName} = style({`);
  if (start === -1) {
    throw new Error(`Could not find "export const ${exportName} = style({" in SessionRow.css.ts`);
  }
  const end = source.indexOf("\n});", start);
  if (end === -1) {
    throw new Error(`Could not find closing "});" for ${exportName} in SessionRow.css.ts`);
  }
  return source.slice(start, end);
}

describe("SessionRow.css.ts — wrap instead of ellipsis (Epic 2.1 Story 2.1.1)", () => {
  let source: string;

  beforeAll(() => {
    source = fs.readFileSync(SESSION_ROW_CSS, "utf-8");
  });

  it("SessionRow_should_ApplyOverflowWrapAnywhere_When_TitleHasNoWhitespaceBreak", () => {
    const nameBlock = extractBlock(source, "name");
    expect(nameBlock).toContain('overflowWrap: "anywhere"');
    expect(nameBlock).not.toContain("whiteSpace");
    expect(nameBlock).not.toContain("textOverflow");
    expect(nameBlock).not.toMatch(/[^-]overflow:\s*"hidden"/);
  });

  it("SessionRow_should_ApplyOverflowWrapAnywhere_When_PathHasNoWhitespaceBreak", () => {
    const pathBlock = extractBlock(source, "path");
    expect(pathBlock).toContain('overflowWrap: "anywhere"');
    expect(pathBlock).not.toContain("whiteSpace");
    expect(pathBlock).not.toContain("textOverflow");
  });

  it("SessionRow_should_ExceedMinHeight38px_When_NameWrapsToMultipleLines", () => {
    // jsdom can't measure real layout height, so this asserts the style
    // declarations that make growth possible: minHeight stays a floor and
    // no maxHeight/fixed height caps the row.
    const rowBlock = extractBlock(source, "row");
    expect(rowBlock).toContain('minHeight: "38px"');
    expect(rowBlock).not.toContain("maxHeight");
    expect(rowBlock).not.toMatch(/[^-]height:\s*"/);
  });
});

describe("SessionRow.css.ts — container-query narrow layout (Epic 2.1 Story 2.1.2)", () => {
  let source: string;

  beforeAll(() => {
    source = fs.readFileSync(SESSION_ROW_CSS, "utf-8");
  });

  it("SessionRow_should_ApplyContainerQueryProperties_When_RootRendered", () => {
    const rowBlock = extractBlock(source, "row");
    expect(rowBlock).toContain('containerType: "inline-size"');
    expect(rowBlock).toContain('containerName: "sessionRow"');
  });

  it("SessionRow_should_DefineNarrowBreakpointNarrowerThanFixedSidebarWidth_When_Configured", () => {
    // Regression guard: the sidebar's real fixed width is ~280px
    // (SessionList.css.ts's own comment), so NARROW must stay below that or
    // it would match unconditionally at the default width (the bug the
    // original NARROW = "(max-width: 320px)" had — plan.md's Resolution Note).
    expect(source).toMatch(/NARROW = "\(max-width: (\d+)px\)"/);
    const match = source.match(/NARROW = "\(max-width: (\d+)px\)"/);
    const narrowPx = Number(match?.[1]);
    expect(narrowPx).toBeLessThan(280);
  });

  it("SessionRow_should_NotSwitchTruncationBudgetAtNarrowBreakpoint_When_ContainerQueryDefined", () => {
    // Story 2.1.2's Resolution Note: no dual-render, no JS-conditional
    // truncation budget lives in this file's @container blocks.
    expect(source).not.toContain("ROW_PATH_MAX_LEN");
    expect(source).not.toMatch(/display:\s*"none"/);
  });
});
