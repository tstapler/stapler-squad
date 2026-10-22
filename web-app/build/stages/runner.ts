import type { StageName } from "../config";

export interface Stage<C> {
  name: StageName;
  run(ctx: C): Promise<void> | void;
}

/** Names the failing stage and, when the error carries one, the source file (UX G7). */
export class StageError extends Error {
  constructor(
    readonly stage: StageName,
    cause: unknown,
  ) {
    const file = fileOf(cause);
    const msg = cause instanceof Error ? cause.message : String(cause);
    super(`[web-build:${stage}] ${file ? `${file}: ` : ""}${msg}`);
    this.name = "StageError";
    this.cause = cause;
  }
}

function fileOf(e: unknown): string | undefined {
  const o = e as { file?: unknown; importer?: unknown; position?: { file?: unknown } } | undefined;
  for (const v of [o?.file, o?.importer, o?.position?.file]) if (typeof v === "string" && v) return v;
  return undefined;
}

export interface StageTiming {
  name: string;
  durationMs: number;
}

export async function runStages<C>(ctx: C, stages: Stage<C>[], onTiming: (t: StageTiming) => void): Promise<void> {
  for (const stage of stages) {
    const t0 = performance.now();
    try {
      await stage.run(ctx);
    } catch (e) {
      throw e instanceof StageError ? e : new StageError(stage.name, e);
    } finally {
      onTiming({ name: stage.name, durationMs: Math.round(performance.now() - t0) });
    }
  }
}
