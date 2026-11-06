#!/usr/bin/env node
// Moves production imports of next/navigation and next/link onto the @/lib/nav facade.
//   node scripts/codemods/next-nav-to-facade.mjs [--check] [dir-or-file ...]   (default: src)
// ast-grep (the pinned devDependency, never `sg`) locates the imports; this script applies the edits.
// Idempotent. Imports it cannot translate (type-only, `redirect`, `LinkProps`) are left and listed on stderr.
// --check prints the files that would change and exits 1 if any.
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const webApp = path.resolve(here, "../..");
const astGrep = path.join(webApp, "node_modules/.bin/ast-grep");
const rules = readFileSync(path.join(here, "next-nav-to-facade.yml"), "utf8");

const FACADE = "@/lib/nav";
const FACADE_NAMES = new Set(["useRouter", "usePathname", "useSearchParams"]);
const isTestFile = (file) => /\.(test|spec)\.tsx?$/.test(file) || file.split(path.sep).includes("__tests__");
const isSeam = (file) => /(^|[\\/])src[\\/]lib[\\/](nav|lazy)[\\/]/.test(file);

function scan(language, glob, targets) {
  const inline = language === "Tsx" ? rules : rules.replaceAll("language: Tsx", "language: TypeScript");
  const args = ["scan", "--inline-rules", inline, "--json=compact", "--globs", glob, ...targets];
  const run = spawnSync(astGrep, args, { encoding: "utf8", maxBuffer: 256 * 1024 * 1024 });
  if (run.status !== 0 && run.status !== 1) throw new Error(`ast-grep failed: ${run.stderr}`);
  return run.stdout.trim() ? JSON.parse(run.stdout) : [];
}

/** Statement text around a matched module-specifier fragment. */
function statementAround(source, fragment) {
  // ast-grep reports UTF-8 byte offsets; string indexes differ once a file has non-ASCII text.
  const charAt = (byte) => Buffer.from(source).subarray(0, byte).toString().length;
  const start = source.lastIndexOf("import", charAt(fragment.range.byteOffset.start));
  let end = charAt(fragment.range.byteOffset.end) + 1; // closing quote
  if (source[end] === ";") end++;
  if (source[end] === "\n") end++;
  return { start, end, text: source.slice(start, end) };
}

/** Specifier strings (`Link`, `useRouter`, `Link as NextLink`), or null when the import is not translatable. */
function specifiersOf(text) {
  const m = text.match(/^import\s+(?:(\w+)\s*,?\s*)?(?:\{([^}]*)\})?\s*from\s*["']next\/(navigation|link)["']/s);
  if (!m) return null;
  const [, def, named, module] = m;
  const out = [];
  if (def) {
    if (module !== "link") return null;
    out.push(def === "Link" ? "Link" : `Link as ${def}`);
  }
  for (const raw of (named ?? "").split(",").map((s) => s.trim()).filter(Boolean)) {
    const name = raw.split(/\s+as\s+/)[0];
    if (module === "link" || !FACADE_NAMES.has(name)) return null;
    out.push(raw);
  }
  return out.length ? out : null;
}

const sortKey = (spec) => spec.split(/\s+as\s+/)[0].toLowerCase();

function rewrite(source, matches) {
  const seen = new Set();
  const statements = [];
  const skipped = [];
  for (const m of matches) {
    const st = statementAround(source, m);
    if (seen.has(st.start)) continue;
    seen.add(st.start);
    const specs = specifiersOf(st.text);
    if (specs) statements.push({ ...st, specs });
    else skipped.push(st.text.trim().split("\n")[0]);
  }
  if (!statements.length) return { next: source, skipped };

  const existing = source.match(/import\s*\{([^}]*)\}\s*from\s*["']@\/lib\/nav["'];?\n?/);
  const merged = new Set(statements.flatMap((s) => s.specs));
  if (existing) existing[1].split(",").map((s) => s.trim()).filter(Boolean).forEach((s) => merged.add(s));
  const line = `import { ${[...merged].sort((a, b) => sortKey(a).localeCompare(sortKey(b))).join(", ")} } from "${FACADE}";\n`;

  const edits = statements.map((s) => ({ start: s.start, end: s.end, text: "" }));
  if (existing) edits.push({ start: existing.index, end: existing.index + existing[0].length, text: "" });
  const anchor = existing ? existing.index : statements[0].start;
  edits.find((e) => e.start === anchor).text = line;
  let out = source;
  // Back to front so earlier offsets stay valid.
  for (const e of edits.sort((a, b) => b.start - a.start)) out = out.slice(0, e.start) + e.text + out.slice(e.end);
  return { next: out, skipped };
}

export function run(targets, { check = false } = {}) {
  const byFile = new Map();
  const found = [...scan("Tsx", "*.tsx", targets), ...scan("TypeScript", "*.ts", targets)];
  for (const m of found.filter((x) => !isTestFile(x.file) && !isSeam(x.file))) {
    byFile.set(m.file, [...(byFile.get(m.file) ?? []), m]);
  }
  const changed = [];
  const manual = [];
  for (const [file, matches] of byFile) {
    const source = readFileSync(file, "utf8");
    const { next, skipped } = rewrite(source, matches);
    skipped.forEach((s) => manual.push(`${file}: ${s}`));
    if (next !== source) {
      changed.push(file);
      if (!check) writeFileSync(file, next);
    }
  }
  return { changed: changed.sort(), manual: manual.sort() };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const check = args.includes("--check");
  const targets = args.filter((a) => a !== "--check");
  const { changed, manual } = run(targets.length ? targets : ["src"], { check });
  console.log(changed.join("\n"));
  if (manual.length) console.error(`needs manual edit:\n${manual.join("\n")}`);
  console.error(`${changed.length} file(s) ${check ? "would change" : "changed"}`);
  if (check && changed.length) process.exit(1);
}
