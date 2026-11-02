// Run by `bun run test:codemods` (node --test). Next was removed in PR 9; this keeps it from coming back.
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const webApp = path.join(path.dirname(fileURLToPath(import.meta.url)), "../..");

const BANNED = [/^next$/, /^eslint-config-next$/, /^@next\//, /^@storybook\/nextjs/, /^@vanilla-extract\/next-plugin$/];
const IMPORT_FROM_NEXT = /(?:\bfrom|\bimport\s*\(?|\brequire\s*\()\s*["']next(?:\/[^"']*)?["']/;

/** Names of every banned package declared in a package.json document. */
export function bannedDependencies(pkg) {
  const names = ["dependencies", "devDependencies", "peerDependencies", "optionalDependencies"].flatMap((k) => Object.keys(pkg[k] ?? {}));
  return names.filter((n) => BANNED.some((re) => re.test(n)));
}

/** Source lines that import from `next` or `next/*`. */
export function nextImports(text) {
  return text.split("\n").filter((line) => IMPORT_FROM_NEXT.test(line));
}

function* sourceFiles(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* sourceFiles(full);
    else if (/\.(ts|tsx|js|jsx|mjs)$/.test(entry.name)) yield full;
  }
}

test("web-app/package.json declares no Next package", () => {
  const pkg = JSON.parse(readFileSync(path.join(webApp, "package.json"), "utf8"));
  assert.deepEqual(bannedDependencies(pkg), []);
});

test("nothing under web-app/src imports from next", () => {
  const hits = [];
  for (const file of sourceFiles(path.join(webApp, "src"))) {
    for (const line of nextImports(readFileSync(file, "utf8"))) hits.push(`${path.relative(webApp, file)}: ${line.trim()}`);
  }
  assert.deepEqual(hits, []);
});

test("the checks can fail: a seeded Next dependency and a seeded Next import are caught", () => {
  assert.deepEqual(bannedDependencies({ dependencies: { next: "15.3.2" }, devDependencies: { "eslint-config-next": "1", react: "19" } }), ["next", "eslint-config-next"]);
  assert.equal(nextImports('import Link from "next/link";\nimport { x } from "next-themes";\n').length, 1);
});
