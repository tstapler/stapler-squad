// Growth gate: a PR fails only when its measured bundle is bigger than main's, so main drifting
// over a cap never blocks an unrelated PR. Pure Node; compare-stats-cli.ts is what CI runs.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import type { BuildStats } from "./stats";

export type GrowthMetric = "jsBytes" | "initialJsBytes" | "largestChunkBytes" | "largestLazyChunkBytes";
export type GrowthTolerances = Record<GrowthMetric, number>;

const LABELS: Record<GrowthMetric, string> = {
  jsBytes: "Total JS",
  initialJsBytes: "Initial JS",
  largestChunkBytes: "Largest chunk",
  largestLazyChunkBytes: "Largest lazy chunk",
};

/** Returns one error per metric that grew by more than its tolerance; a metric absent from the baseline is skipped. */
export function compareToBaseline(current: Partial<BuildStats>, baseline: Partial<BuildStats>, tolerances: GrowthTolerances): string[] {
  const errors: string[] = [];
  for (const metric of Object.keys(LABELS) as GrowthMetric[]) {
    const now = current[metric];
    const before = baseline[metric];
    if (now === undefined || before === undefined) continue;
    const growth = now - before;
    if (growth > tolerances[metric]) {
      errors.push(`growth: ${LABELS[metric]} grew ${growth} B vs main (${before} -> ${now}), over the ${tolerances[metric]} B tolerance`);
    }
  }
  return errors;
}

export function findStatsFile(dir: string, name: string): string | undefined {
  if (!existsSync(dir)) return undefined;
  const direct = path.join(dir, name);
  if (existsSync(direct)) return direct;
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    if (e.isDirectory()) {
      const nested = findStatsFile(path.join(dir, e.name), name);
      if (nested) return nested;
    }
  }
  return undefined;
}

export interface ArtifactRecord {
  id: number;
  expired?: boolean;
  workflow_run?: { head_sha?: string; head_branch?: string };
}

/** Id of the live main-branch artifact built from exactly `baseSha`; undefined when none (never an older commit's). */
export function selectBaselineArtifactId(artifacts: ArtifactRecord[], baseSha: string): number | undefined {
  return artifacts.find((a) => !a.expired && a.workflow_run?.head_sha === baseSha && a.workflow_run?.head_branch === "main")?.id;
}
