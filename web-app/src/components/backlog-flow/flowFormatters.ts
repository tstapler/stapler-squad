import type { FlowDurationMetric, FlowRatioMetric } from "@/gen/session/v1/backlog_flow_pb";

/** Below this denominator a percentage is shown greyed with "low sample". */
export const LOW_SAMPLE_DENOMINATOR = 5;

export const formatInt = (n: number): string => n.toLocaleString("en-US");

const trimZero = (n: number): string => n.toFixed(1).replace(/\.0$/, "");

export const formatPercent = (num: number, den: number): string => `${((num / den) * 100).toFixed(1)}%`;

/** `32/105 (30.5%)`, count first; a zero denominator is "no data", never "0%". */
export function formatRatio(num: number, den: number): string {
  return den === 0 ? "no data" : `${formatInt(num)}/${formatInt(den)} (${formatPercent(num, den)})`;
}

export const isLowSample = (ratio: Pick<FlowRatioMetric, "den" | "lowSample">): boolean =>
  ratio.den > 0 && (ratio.lowSample || ratio.den < LOW_SAMPLE_DENOMINATOR);

/** `45m`, `5h`, `1.1d`. */
export function formatDurationMs(ms: bigint | number): string {
  const minutes = Number(ms) / 60_000;
  if (minutes < 1) return "<1m";
  if (minutes < 60) return `${Math.round(minutes)}m`;
  const hours = minutes / 60;
  if (hours < 24) return `${trimZero(hours)}h`;
  return `${trimZero(hours / 24)}d`;
}

export interface DurationText {
  median: string;
  p90: string;
}

/** Null when there is no sample, so callers print "n/a" rather than a made-up zero. */
export function durationText(metric: FlowDurationMetric | undefined): DurationText | null {
  if (!metric || metric.n === 0) return null;
  return { median: formatDurationMs(metric.p50Ms), p90: formatDurationMs(metric.p90Ms) };
}

/** `2 items` / `1 item`. */
export const plural = (n: number, one: string, many = `${one}s`): string => `${formatInt(n)} ${n === 1 ? one : many}`;
