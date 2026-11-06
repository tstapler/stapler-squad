// Probes an `eval` package dir (default: the one @vanilla-extract/integration resolves)
// and prints one JSON line. Exit code carries the verdict; the error text is informational
// only because its wording differs between Bun versions.
import { createRequire } from "node:module";
import path from "node:path";

function arg(name: string): string | undefined {
  const i = process.argv.indexOf(name);
  return i === -1 ? undefined : process.argv[i + 1];
}

function defaultEvalDir(): string {
  const fromIntegration = createRequire(require.resolve("@vanilla-extract/integration/package.json"));
  return path.dirname(fromIntegration.resolve("eval/package.json"));
}

try {
  const evalDir = path.resolve(arg("--eval-dir") ?? defaultEvalDir());
  const evalFn = createRequire(path.join(evalDir, "noop.js"))("./eval.js");
  // Unpatched eval returns an undefined `path` under Bun, so reading p.sep throws.
  const out = evalFn("var p=require('path'); module.exports={sep:p.sep}", path.join(evalDir, "probe.js"), null, true);
  if (out.sep !== "/" && out.sep !== "\\") throw new Error(`unexpected probe result ${JSON.stringify(out)}`);
  console.log(JSON.stringify({ ok: true, evalDir }));
} catch (e) {
  console.log(JSON.stringify({ ok: false, error: String(e) }));
  process.exit(1);
}
