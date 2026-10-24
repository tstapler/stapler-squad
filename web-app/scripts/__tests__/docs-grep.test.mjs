// Run by `bun run test:codemods` (node --test). Guidance that names Next-only commands must say which build it means.
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "../../..");

const NEXT_ONLY = /next dev|next build|NEXT_BUILD_MODE|\.next\/|Next\.js static export/;
const MARKED = /WEB_BUILD|Next mode \(removed in PR 9\)/;

// Current guidance only: docs/explanation, docs/tasks and docs/archive are dated records of what was true then.
const SCOPE = ["CLAUDE.md", "docs/how-to", "docs/reference", "docs/adr", "server/web/embed.go", ".claude/commands", "Makefile"];

/** Hits whose paragraph (contiguous non-blank lines) neither describes WEB_BUILD nor carries the removal marker. */
export function unmarkedHits(relPath, text) {
  const hits = [];
  let block = [];
  const flush = () => {
    if (!MARKED.test(block.map((b) => b.line).join("\n"))) {
      for (const b of block) if (NEXT_ONLY.test(b.line)) hits.push(`${relPath}:${b.no}: ${b.line.trim()}`);
    }
    block = [];
  };
  text.split("\n").forEach((line, i) => {
    if (line.trim() === "") flush();
    else block.push({ line, no: i + 1 });
  });
  flush();
  return hits;
}

function* filesUnder(rel) {
  const abs = path.join(root, rel);
  if (statSync(abs).isFile()) {
    yield rel;
    return;
  }
  for (const entry of readdirSync(abs)) yield* filesUnder(path.join(rel, entry));
}

test("every Next-only command in current guidance is marked or describes WEB_BUILD", () => {
  const hits = SCOPE.flatMap((p) => [...filesUnder(p)]).flatMap((f) => unmarkedHits(f, readFileSync(path.join(root, f), "utf8")));
  assert.deepEqual(hits, [], `mark with "Next mode (removed in PR 9)" or describe WEB_BUILD:\n${hits.join("\n")}`);
});

test("the checker can fail: an unmarked hit is reported, a marked or WEB_BUILD paragraph is not", () => {
  assert.equal(unmarkedHits("a.md", "Run `next dev` to start.\n").length, 1);
  assert.equal(unmarkedHits("a.md", "Run `next dev` (Next mode (removed in PR 9)).\n").length, 0);
  assert.equal(unmarkedHits("a.md", "WEB_BUILD=next uses `next build`.\n").length, 0);
  assert.equal(unmarkedHits("a.md", "Marked paragraph WEB_BUILD.\n\nSeparate `next build` paragraph.\n").length, 1);
});
