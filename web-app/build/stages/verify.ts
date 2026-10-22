import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { computeOutputStats, type BuildStats } from "../stats";
import { validateAllowlist, verifyOutput, type AllowlistEntry, type Budgets } from "../verify-output";
import type { BuildContext } from "./context";
import type { Stage } from "./runner";

export const DEFAULT_BUDGETS = path.resolve(__dirname, "../budgets.json");
export const DEFAULT_ALLOWLIST = path.resolve(__dirname, "../../../server/middleware/testdata/public-allowlist.json");

function readJson<T>(file: string): T {
  return JSON.parse(readFileSync(file, "utf8")) as T;
}

function peakRssKb(): number | undefined {
  try {
    return process.resourceUsage().maxRSS;
  } catch {
    return undefined;
  }
}

export function collectStats(ctx: BuildContext): BuildStats {
  return {
    mode: "bun",
    minify: ctx.opts.minify,
    minifyExplicit: ctx.opts.minifyExplicit,
    bunVersion: Bun.version,
    stages: ctx.timings,
    totalDurationMs: Math.round(performance.now() - ctx.startedAt),
    peakRssKb: peakRssKb(),
    ...computeOutputStats(ctx.tmpDir),
  };
}

export const verify: Stage<BuildContext> = {
  name: "verify",
  run(ctx) {
    const allowlist = readJson<AllowlistEntry[]>(ctx.opts.allowlistPath ?? DEFAULT_ALLOWLIST);
    const schema = validateAllowlist(allowlist);
    if (schema.length > 0) throw new Error(schema.join("; "));
    const stats = collectStats(ctx);
    mkdirSync(path.dirname(ctx.statsPath), { recursive: true });
    writeFileSync(ctx.statsPath, `${JSON.stringify(stats, null, 2)}\n`);
    const { errors, warnings } = verifyOutput({
      outDir: ctx.tmpDir,
      statsPath: ctx.statsPath,
      stats,
      budgets: readJson<Budgets>(ctx.opts.budgetsPath ?? DEFAULT_BUDGETS),
      allowlist,
      enforceBytes: ctx.opts.enforceBytes,
      loginClosure: ctx.loginClosure,
    });
    ctx.warnings.push(...warnings);
    if (errors.length > 0) throw new Error(`verify-output failed:\n  - ${errors.join("\n  - ")}`);
  },
};
