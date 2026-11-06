#!/usr/bin/env node
// Moves Jest tests from mocking next/navigation and next/link onto the @/lib/nav facade.
//   node scripts/codemods/next-nav-to-facade-tests.mjs [--check] [dir-or-file ...]   (default: src)
// ast-grep (the pinned devDependency, never `sg`) locates the nodes; this script applies the edits.
// Idempotent: a second run finds nothing. --check prints the files that would change and exits 1 if any.
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const webApp = path.resolve(here, "../..");
const astGrep = path.join(webApp, "node_modules/.bin/ast-grep");
const rules = readFileSync(path.join(here, "next-nav-to-facade-tests.yml"), "utf8");

const FACADE = "@/lib/nav";
const HELPER = "@/lib/nav/testing-mock";

function scan(language, globs, targets) {
  const inlineRules = language === "Tsx" ? rules : rules.replaceAll("language: Tsx", "language: TypeScript");
  const args = ["scan", "--inline-rules", inlineRules, "--json=compact", ...globs.flatMap((g) => ["--globs", g]), ...targets];
  const run = spawnSync(astGrep, args, { encoding: "utf8", maxBuffer: 256 * 1024 * 1024 });
  if (run.status !== 0 && run.status !== 1) throw new Error(`ast-grep failed: ${run.stderr}`);
  return run.stdout.trim() ? JSON.parse(run.stdout) : [];
}

// Production files are PR 2b's job; this script only touches tests.
const isTestFile = (file) => /\.test\.tsx?$/.test(file) || file.split(path.sep).includes("__tests__");

export function findMatches(targets) {
  return [...scan("Tsx", ["*.tsx"], targets), ...scan("TypeScript", ["*.ts"], targets)].filter((m) => isTestFile(m.file));
}

const slice = (buf, range) => buf.subarray(range.byteOffset.start, range.byteOffset.end);

function editsFor(file, matches, source) {
  const buf = Buffer.from(source);
  const nav = matches.find((m) => m.ruleId === "nav-mock");
  const link = matches.find((m) => m.ruleId === "link-mock");
  const edits = [];
  const factory = (m) => m.metaVariables.single.FACTORY.text;

  if (nav && !link) {
    edits.push({ range: nav.range, text: `jest.mock("${FACADE}", ${factory(nav)})` });
  } else if (nav || link) {
    // Two mocks of one module would override each other, so a file with both gets one merged mock.
    const nested = (m) => factory(m).replaceAll("\n", "\n    ");
    const parts = [nav && `navigation: ${nested(nav)},`, link && `link: ${nested(link)},`].filter(Boolean).join("\n    ");
    const merged = `jest.mock("${FACADE}", () =>\n  require("${HELPER}").navMockFromNext({\n    ${parts}\n  }),\n)`;
    const [first, second] = [nav, link].filter(Boolean).sort((a, b) => a.range.byteOffset.start - b.range.byteOffset.start);
    edits.push({ range: first.range, text: merged });
    if (second) edits.push({ range: second.range, text: "", swallowStatement: true });
  }
  for (const m of matches.filter((x) => x.ruleId === "nav-import")) {
    edits.push({ range: m.range, text: FACADE });
  }
  return { buf, edits };
}

function apply(buf, edits) {
  let out = buf;
  for (const edit of [...edits].sort((a, b) => b.range.byteOffset.start - a.range.byteOffset.start)) {
    let { start, end } = edit.range.byteOffset;
    if (edit.swallowStatement) {
      // Drop the trailing `;` and the blank line the removed statement leaves behind.
      while (out[end] === 0x3b) end++;
      while (out[end] === 0x0a && out[start - 1] === 0x0a) end++;
    }
    out = Buffer.concat([out.subarray(0, start), Buffer.from(edit.text), out.subarray(end)]);
  }
  return out.toString();
}

export function run(targets, { check = false } = {}) {
  const byFile = new Map();
  for (const m of findMatches(targets)) {
    byFile.set(m.file, [...(byFile.get(m.file) ?? []), m]);
  }
  const changed = [];
  for (const [file, matches] of byFile) {
    const source = readFileSync(file, "utf8");
    const { buf, edits } = editsFor(file, matches, source);
    const next = apply(buf, edits);
    if (next !== source) {
      changed.push(file);
      if (!check) writeFileSync(file, next);
    }
  }
  return changed.sort();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const check = args.includes("--check");
  const targets = args.filter((a) => a !== "--check");
  const changed = run(targets.length ? targets : ["src"], { check });
  console.log(changed.join("\n"));
  console.error(`${changed.length} file(s) ${check ? "would change" : "changed"}`);
  if (check && changed.length) process.exit(1);
}
