#!/usr/bin/env ts-node
/**
 * Audit CSS token adoption in .css.ts files.
 * Reports hardcoded hex and px values that should use vars.* tokens.
 * Run: npx ts-node scripts/audit-css-tokens.ts
 */
import { globSync } from "glob";
import { readFileSync } from "fs";

const files = globSync("src/**/*.css.ts", { ignore: ["**/gen/**"] });
const hexPattern = /#[0-9a-fA-F]{3,8}\b/g;
const pxPattern = /["'`]\d+px["'`]/g;

let totalHex = 0;
let totalPx = 0;

for (const file of files) {
  const content = readFileSync(file, "utf-8");
  const hexMatches = content.match(hexPattern) ?? [];
  const pxMatches = content.match(pxPattern) ?? [];
  if (hexMatches.length > 0 || pxMatches.length > 0) {
    console.log(`\n${file}`);
    if (hexMatches.length) console.log(`  hex (${hexMatches.length}): ${hexMatches.slice(0, 5).join(", ")}`);
    if (pxMatches.length) console.log(`  px  (${pxMatches.length}): ${pxMatches.slice(0, 5).join(", ")}`);
    totalHex += hexMatches.length;
    totalPx += pxMatches.length;
  }
}

console.log(`\nTotal: ${totalHex} hex values, ${totalPx} hardcoded px values across ${files.length} files`);
