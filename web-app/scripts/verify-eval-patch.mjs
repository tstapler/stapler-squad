// Fails loudly when eval@0.1.8 is unpatched (ADR-008), instead of the opaque
// "setAdapter of undefined" later. Marker check only; the functional canary lives in
// build/__tests__/ve-eval-patch.test.ts.
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import path from "node:path";

export const MARKER = "ssq-patch: eval-bun-createRequire";

function defaultEvalDir() {
  const fromIntegration = createRequire(createRequire(import.meta.url).resolve("@vanilla-extract/integration/package.json"));
  return path.dirname(fromIntegration.resolve("eval/package.json"));
}

export function check(evalDir) {
  const version = JSON.parse(readFileSync(path.join(evalDir, "package.json"), "utf8")).version;
  if (version !== "0.1.8") {
    return `eval is ${version}, not 0.1.8: patches/eval@0.1.8.patch no longer applies; re-author it (docs/how-to/patch-vanilla-extract-eval.md)`;
  }
  if (!readFileSync(path.join(evalDir, "eval.js"), "utf8").includes(MARKER)) {
    return "eval is not patched: patches/eval@0.1.8.patch was not applied (rerun bun install; see oven-sh/bun#33520 for patches silently skipped on a warm cache)";
  }
  return null;
}

if (import.meta.main ?? process.argv[1] === new URL(import.meta.url).pathname) {
  const i = process.argv.indexOf("--eval-dir");
  const problem = check(i === -1 ? defaultEvalDir() : path.resolve(process.argv[i + 1]));
  if (problem) {
    console.error(`verify-eval-patch: ${problem}`);
    process.exit(1);
  }
  console.log("verify-eval-patch: ok");
}
