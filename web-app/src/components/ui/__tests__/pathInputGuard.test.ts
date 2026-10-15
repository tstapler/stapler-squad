/**
 * Guard: a text field that looks like a filesystem path must use RepoPathInput.
 * Parses each .tsx with the TypeScript AST, so attribute order and `=>` in handlers don't matter.
 * A field is "path-like" if its id/name/placeholder/aria-label/data-testid string, or the identifiers
 * its `value`/`onChange` are bound to, match PATHISH. Covers <input>, <textarea> and the shared <Input>.
 *
 * Heuristic limits: it cannot see a path field whose naming gives no hint, and it matches substrings,
 * so a non-path field whose name contains "dir"/"path" needs an allowlist entry with a reason.
 */
import fs from "fs";
import path from "path";
import ts from "typescript";

const SRC = path.resolve(__dirname, "../../..");
const TAGS = new Set(["input", "textarea", "Input"]);
const SKIP_TYPES = new Set(["checkbox", "radio", "file", "number", "password", "url", "email", "hidden", "range", "color", "date", "time"]);
const PATHISH = /path|\bdirs?\b|[a-z]Dirs?\b|directory|cwd|folder|worktree|repoRoot/i;

// file (relative to src) -> { count: number of flagged fields allowed, reason }
const ALLOWLIST: Record<string, { count: number; reason: string }> = {
  "components/settings/AddRemoteForm.tsx": { count: 1, reason: "base path on the REMOTE host; local completions would mislead" },
  "app/insights/SessionsTable.tsx": { count: 1, reason: "free-text search over paths, not a path value" },
  "components/sessions/Omnibar.tsx": { count: 1, reason: "session title / omnibar command input, not a path value" },
  "components/sessions/OmnibarCreationPanel.tsx": { count: 1, reason: "working dir is a RELATIVE subdirectory of the chosen repo (session/instance.go:215)" },
  "components/sessions/SessionDetailView.tsx": { count: 1, reason: "edits Instance.WorkingDir, a RELATIVE subdirectory within the repo (session/instance.go:215)" },
  "components/sessions/WorkspaceSwitchModal.tsx": { count: 1, reason: "branch/revision/worktree filter text, not a path value" },
};

type Found = { file: string; line: number; why: string };

function attrText(attr: ts.JsxAttribute, sf: ts.SourceFile): string {
  return attr.initializer ? attr.initializer.getText(sf) : "";
}

function scan(file: string): Found[] {
  return scanSource(file, fs.readFileSync(file, "utf8"));
}

function scanSource(file: string, text: string): Found[] {
  const sf = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const out: Found[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
      if (TAGS.has(node.tagName.getText(sf))) {
        const attrs = node.attributes.properties.filter(ts.isJsxAttribute);
        const get = (n: string) => attrs.find((a) => a.name.getText(sf) === n);
        const type = get("type") ? attrText(get("type")!, sf).replace(/["'{}`\s]/g, "") : "text";
        if (!SKIP_TYPES.has(type)) {
          const hits = attrs
            .filter((a) => ["id", "name", "placeholder", "aria-label", "data-testid", "value", "onChange"].includes(a.name.getText(sf)))
            .filter((a) => PATHISH.test(attrText(a, sf)));
          if (hits.length) {
            out.push({ file: path.relative(SRC, file), line: sf.getLineAndCharacterOfPosition(node.getStart(sf)).line + 1, why: hits.map((h) => h.name.getText(sf)).join(",") });
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return out;
}

function walk(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (!["gen", "__tests__", "node_modules"].includes(e.name)) walk(p, out);
    } else if (e.name.endsWith(".tsx") && !/\.(test|stories)\.tsx$/.test(e.name)) out.push(p);
  }
  return out;
}

describe("path-like text fields use RepoPathInput", () => {
  const found = walk(SRC).flatMap(scan);

  it("flags nothing outside the allowlist (and no allowlisted file exceeds its count)", () => {
    const byFile = new Map<string, Found[]>();
    for (const f of found) byFile.set(f.file, [...(byFile.get(f.file) ?? []), f]);
    const problems: string[] = [];
    for (const [file, list] of byFile) {
      const allowed = ALLOWLIST[file]?.count ?? 0;
      if (list.length > allowed) problems.push(`${file}: ${list.length} path-like field(s), ${allowed} allowed -> ${list.map((l) => `:${l.line}(${l.why})`).join(" ")}`);
    }
    expect(problems).toEqual([]);
  });

  it("every allowlist entry still matches a field (no stale exemptions)", () => {
    const stale = Object.entries(ALLOWLIST).filter(([file, { count }]) => found.filter((f) => f.file === file).length < count).map(([f]) => f);
    expect(stale).toEqual([]);
  });

  it("detects the shapes a regex scan missed (self-test, in memory)", () => {
    const cases = [
      '<input onChange={(e) => set(e.target.value)} placeholder="Path" />',
      "<input value={repoPath} onChange={noop} />",
      '<Input aria-label="Working dir" />',
      "<textarea value={cwd} onChange={noop} />",
    ];
    for (const c of cases) expect(scanSource("selftest.tsx", `export const X = () => (${c});`).length).toBe(1);
    expect(scanSource("selftest.tsx", '<input type="checkbox" placeholder="path" />').length).toBe(0);
  });
});
