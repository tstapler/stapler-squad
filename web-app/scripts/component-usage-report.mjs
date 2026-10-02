#!/usr/bin/env node
// Lists every component in components/ui + components/common with its consumer files,
// flags components with no consumers (unused) and ones that have no story.
// Usage: node scripts/component-usage-report.mjs [--md]
import fs from "node:fs";
import path from "node:path";

const SRC = path.resolve(import.meta.dirname, "../src");
const DIRS = ["components/ui", "components/common"];

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name !== "gen" && e.name !== "node_modules") walk(p, out);
    } else if (/\.tsx?$/.test(e.name)) out.push(p);
  }
  return out;
}

const all = walk(SRC);
const isProd = (f) => !/\.(test|stories)\.tsx?$/.test(f) && !f.includes("__tests__");
const components = DIRS.flatMap((d) =>
  fs.readdirSync(path.join(SRC, d))
    .filter((f) => /^[A-Z][A-Za-z0-9]*\.tsx$/.test(f) && isProd(f))
    .map((f) => ({ name: f.replace(/\.tsx$/, ""), dir: d }))
);

// Names re-exported by components/ui/index.ts, keyed by source component file.
const barrel = fs.readFileSync(path.join(SRC, "components/ui/index.ts"), "utf8");
const barrelExports = {};
for (const m of barrel.matchAll(/export\s*\{([^}]*)\}\s*from\s*["']\.\/(\w+)["']/g)) {
  barrelExports[m[2]] = m[1].split(",").map((x) => x.trim()).filter(Boolean);
}

const rows = components.map(({ name, dir }) => {
  const own = path.join(SRC, dir, `${name}.tsx`);
  const re = new RegExp(`(?:from\\s+["'](?:@/${dir}/${name}|\\./${name}|\\.\\./${path.basename(dir)}/${name})["'])`);
  const viaBarrel = dir === "components/ui" ? barrelExports[name] ?? [] : [];
  const barrelRe = viaBarrel.length
    ? new RegExp(`import\\s*(?:type\\s*)?\\{[^}]*\\b(?:${viaBarrel.join("|")})\\b[^}]*\\}\\s*from\\s*["']@/components/ui["']`)
    : null;
  const consumers = all.filter((f) => {
    if (f === own || !isProd(f) || f.endsWith(path.join("components/ui", "index.ts"))) return false;
    const text = fs.readFileSync(f, "utf8");
    return re.test(text) || (barrelRe && barrelRe.test(text));
  })
    .map((f) => path.relative(SRC, f));
  const hasStory = fs.existsSync(path.join(SRC, dir, `${name}.stories.tsx`));
  return { name, dir, consumers, hasStory };
});

// Duplicate candidates: same name in two directories.
const dupes = Object.entries(Object.groupBy(rows, (r) => r.name)).filter(([, v]) => v.length > 1).map(([k]) => k);

const md = process.argv.includes("--md");
if (md) {
  console.log("| Component | Dir | Consumers | Story | Flag |\n|---|---|---|---|---|");
  for (const r of rows.sort((a, b) => a.consumers.length - b.consumers.length)) {
    const flag = [r.consumers.length === 0 ? "UNUSED" : "", dupes.includes(r.name) ? "DUPLICATE-NAME" : ""].filter(Boolean).join(" ");
    console.log(`| ${r.name} | ${r.dir} | ${r.consumers.length} | ${r.hasStory ? "yes" : "no"} | ${flag} |`);
  }
} else {
  for (const r of rows) console.log(`${r.name}\t${r.consumers.length}\t${r.hasStory ? "story" : "-"}`);
}
