// Oracle: every src/**/*.css.ts must compile to the same JS and CSS under Node (the
// reference) and Bun (through the patched eval). Run: `bun run oracle:ve` from web-app/.
// Exits 1 on any difference or compile error.
import { compile, getSourceFromVirtualCssFile, processVanillaFile } from "@vanilla-extract/integration";
import { Glob } from "bun";
import path from "node:path";

const cwd = process.cwd();
const files = [...new Glob("src/**/*.css.ts").scanSync({ cwd, absolute: true })].sort();
const MAX_NODE_OUTPUT_BYTES = 2 ** 30;
const VIRTUAL_CSS_IMPORT = /["'][^"']*\.vanilla\.css\?source=[^"']+["']/g;

async function underBun(): Promise<Record<string, string>> {
  const out: Record<string, string> = {};
  for (const filePath of files) {
    try {
      const { source } = await compile({ filePath, cwd, identOption: "short" });
      out[filePath] = await processVanillaFile({ source, filePath, identOption: "short" });
    } catch (e) {
      out[filePath] = `ERROR: ${e}`;
    }
  }
  return out;
}

// Gzip bytes in the inlined virtual CSS differ between Node's and Bun's zlib, so compare the decoded CSS.
async function normalize(output: string): Promise<string> {
  const css: string[] = [];
  for (const m of output.matchAll(VIRTUAL_CSS_IMPORT)) {
    css.push((await getSourceFromVirtualCssFile(m[0].slice(1, -1))).source);
  }
  return JSON.stringify({ js: output.replace(VIRTUAL_CSS_IMPORT, '"@CSS@"'), css });
}

const node = Bun.spawnSync(["node", path.join(import.meta.dir, "ve-oracle-node.cjs"), ...files], {
  cwd,
  stdout: "pipe",
  stderr: "inherit",
  maxBuffer: MAX_NODE_OUTPUT_BYTES,
});
if (node.exitCode !== 0) throw new Error(`node half exited ${node.exitCode}`);
const nodeOut: Record<string, string> = JSON.parse(node.stdout.toString());
const bunOut = await underBun();

const bad: string[] = [];
for (const f of files) {
  const failed = nodeOut[f].startsWith("ERROR: ") || bunOut[f].startsWith("ERROR: ");
  if (failed || (await normalize(nodeOut[f])) !== (await normalize(bunOut[f]))) bad.push(path.relative(cwd, f));
}
console.log(JSON.stringify({ files: files.length, mismatches: bad.length, bad: bad.slice(0, 10) }));
process.exit(bad.length === 0 ? 0 : 1);
