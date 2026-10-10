// T-RO-19 / Story 5.3: hidden sessions are never pulled into the main session list.
import fs from "fs";
import path from "path";

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "gen" || entry.name === "node_modules" || entry.name === "__tests__") continue;
      walk(full, out);
    } else if (/\.(ts|tsx)$/.test(entry.name) && !/\.test\.|\.stories\./.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

// The only code allowed to pass includeHidden: the user's explicit "Show hidden sessions"
// toggle (SessionList → PaneSplitRenderer) and the transport that forwards it. The default
// list load, the app shell and every other consumer must stay free of it.
const SHOW_HIDDEN_TOGGLE_FILES = [
  path.join("components", "sessions", "SessionList.tsx"),
  path.join("components", "pane", "PaneSplitRenderer.tsx"),
  path.join("lib", "hooks", "useSessionService.ts"),
];

describe("include_hidden guard", () => {
  it("main_listSessions_should_set_include_hidden_only_in_the_explicit_show_hidden_toggle", () => {
    const root = path.join(process.cwd(), "src");
    const offenders = walk(root).filter((file) => {
      const text = fs.readFileSync(file, "utf8");
      // Code only: comments may explain why the flag is absent.
      const code = text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:])\/\/.*$/gm, "$1");
      if (/\bincludeHidden\s*:|\binclude_hidden\s*:/.test(code)) {
        return !SHOW_HIDDEN_TOGGLE_FILES.some((allowed) => file.endsWith(allowed));
      }
      // hidden_only is the Background section's own query (Story 5.4) and nobody else's.
      return /\bhiddenOnly\s*:/.test(code) && !file.endsWith(path.join("hooks", "useBackgroundSessions.ts"));
    });
    expect(offenders).toEqual([]);
  });
});
