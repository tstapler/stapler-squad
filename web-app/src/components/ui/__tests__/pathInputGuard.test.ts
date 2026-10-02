/**
 * Guard: a bare <input> that looks like a filesystem path field must use RepoPathInput.
 * New offenders fail this test; legitimate exceptions go in ALLOWLIST with a reason.
 */
import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "../../..");

// file (relative to src) -> why a plain <input> is correct there
const ALLOWLIST: Record<string, string> = {
  "components/settings/AddRemoteForm.tsx": "base path on the REMOTE host; local completions would mislead",
  "components/ui/RepoPathInput.tsx": "the rich field itself",
  "app/insights/SessionsTable.tsx": "free-text search over paths, not a path value",
  "components/sessions/Omnibar.tsx": "session title / omnibar command input, not a path value",
  "components/sessions/OmnibarCreationPanel.tsx": "working dir is a RELATIVE subdirectory of the chosen repo; absolute-path completions don't apply",
  "components/sessions/WorkspaceSwitchModal.tsx": "branch/revision/worktree filter text, not a path value",
};

const PATHISH = /(?:id|placeholder|aria-label|data-testid|name)=["{`'][^>]*?(?:\bpath\b|[-_ ]path|path[-_ ]|\bdir(?:ectory)?\b|[-_ ]dir\b|folder|worktree)/i;

function walk(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name === "gen" || e.name === "__tests__" || e.name === "node_modules") continue;
      walk(p, out);
    } else if (e.name.endsWith(".tsx") && !/\.(test|stories)\.tsx$/.test(e.name)) out.push(p);
  }
  return out;
}

describe("path-like inputs use RepoPathInput", () => {
  const offenders: string[] = [];
  for (const file of walk(SRC)) {
    const rel = path.relative(SRC, file);
    if (rel in ALLOWLIST) continue;
    const text = fs.readFileSync(file, "utf8");
    for (const m of text.matchAll(/<input\b[^>]*?\/?>/gs)) {
      if (/type=["'](checkbox|radio|file|number|password|url|email|hidden)["']/.test(m[0])) continue;
      if (PATHISH.test(m[0])) {
        offenders.push(`${rel}:${text.slice(0, m.index).split("\n").length}`);
      }
    }
  }

  it("has no bare path inputs outside the allowlist", () => {
    expect(offenders).toEqual([]);
  });

  it("allowlist entries still exist", () => {
    for (const rel of Object.keys(ALLOWLIST)) expect(fs.existsSync(path.join(SRC, rel))).toBe(true);
  });
});
