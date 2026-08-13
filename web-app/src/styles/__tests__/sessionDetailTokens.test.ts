/**
 * Enforcement test for Bug 5 — SessionDetail ignores theme colors.
 *
 * `terminalTokens` is a shared constant of VS Code dark colors spread unchanged
 * into every theme. SessionDetail.css.ts used these tokens for the header/tabs/
 * actions chrome, so the header never responded to theme switching.
 *
 * Fix: replace all `terminalHeaderBg`, `terminalTabsBg`, `terminalHoverBg`,
 * `terminalHeaderFg`, `terminalTextMuted`, `terminalForeground`, `terminalBorder`
 * references in SessionDetail.css.ts with standard `vars.color.*` tokens.
 *
 * Pre-fix failure: the file contained e.g. `terminalHeaderBg` which this test
 * explicitly bans.
 *
 * The xterm.js container background (`terminalBackground`) and cursor
 * (`terminalCursor`) ARE legitimate in non-chrome contexts — those are allowed.
 */
import * as fs from "fs";
import * as path from "path";

const SESSION_DETAIL_CSS = path.resolve(
  __dirname,
  "../../components/sessions/SessionDetail.css.ts"
);

// These tokens are hardcoded to VS Code dark values and shared across all
// themes unchanged. Using them in chrome (header, tabs, buttons) makes the UI
// theme-blind. Only terminalBackground and terminalCursor are allowed since
// they correctly control xterm.js appearance.
const BANNED_CHROME_TOKENS = [
  "terminalHeaderBg",
  "terminalHeaderFg",
  "terminalTabsBg",
  "terminalHoverBg",
  "terminalTextMuted",
];

describe("SessionDetail.css.ts — theme token usage (Bug 5)", () => {
  let source: string;

  beforeAll(() => {
    source = fs.readFileSync(SESSION_DETAIL_CSS, "utf-8");
  });

  for (const token of BANNED_CHROME_TOKENS) {
    it(`must not use hardcoded terminal chrome token: ${token}`, () => {
      // This test fails against pre-fix code where e.g. terminalHeaderBg was used
      // for `header.background` — making the header always dark gray.
      expect(source).not.toContain(token);
    });
  }

  it("uses vars.color references for all chrome colors, not raw hex strings", () => {
    // Strip comment lines and string literals that are just documentation
    const codeLines = source
      .split("\n")
      .filter((line) => !line.trim().startsWith("//") && !line.trim().startsWith("*"));

    // The only hex values allowed are inside terminalBackground/terminalCursor
    // (legitimate terminal-widget colors). No bare hex in chrome styles.
    const hexPattern = /#[0-9a-fA-F]{3,8}/g;
    const violations = codeLines.filter((line) => {
      const matches = line.match(hexPattern);
      if (!matches) return false;
      // Allow hex only if it appears on a line that also references a terminal content token
      return !line.includes("terminalBackground") && !line.includes("terminalCursor");
    });

    expect(violations).toHaveLength(0);
  });
});
