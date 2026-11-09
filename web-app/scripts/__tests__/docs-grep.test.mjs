// Run by `bun run test:codemods` (node --test). Current guidance must not tell anyone to run Next; a mention is allowed only when marked historical.
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "../../..");

const NEXT_ONLY = /next dev|next build|NEXT_BUILD_MODE|\.next\/|Next\.js static export/;
const MARKED = /\(historical\)/;

// Current guidance only: docs/explanation, docs/tasks and docs/archive are dated records of what was true then.
const SCOPE = ["CLAUDE.md", "docs/how-to", "docs/reference", "docs/adr", "server/web/embed.go", ".claude/commands", "Makefile"];

/** Hits whose paragraph (contiguous non-blank lines) does not carry the historical marker. */
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

test("no Next-only command in current guidance is left unmarked", () => {
  const hits = SCOPE.flatMap((p) => [...filesUnder(p)]).flatMap((f) => unmarkedHits(f, readFileSync(path.join(root, f), "utf8")));
  assert.deepEqual(hits, [], `delete the Next-only command, or mark the paragraph "(historical)":\n${hits.join("\n")}`);
});

test("the checker can fail: an unmarked hit is reported, a marked paragraph is not", () => {
  assert.equal(unmarkedHits("a.md", "Run `next dev` to start.\n").length, 1);
  assert.equal(unmarkedHits("a.md", "Run `next dev` (historical).\n").length, 0);
  assert.equal(unmarkedHits("a.md", "Marked paragraph (historical).\n\nSeparate `next build` paragraph.\n").length, 1);
});
