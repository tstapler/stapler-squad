"use client";

import type { CompareFlowWindowsResponse, FlowCompareMetric } from "@/gen/session/v1/backlog_flow_pb";
import { formatInt } from "./flowFormatters";
import * as styles from "./flow.css";

/** Which direction of change is an improvement for a compared metric. */
export type GoodDirection = "up" | "down";

interface FlowDeltaProps {
  comparison: CompareFlowWindowsResponse | null;
  /** `FlowCompareMetric.name`, for example `merged_items`. */
  metric: string;
  good: GoodDirection;
  /** Noun for the sample counts, for example "items" or "verdicts". */
  sampleNoun?: string;
}

const NEED_PATTERN = /need (\d+)/;
const HOURS_PER_DAY = 24;
const PERCENT = 100;

function formatSigned(delta: number, unit: string, reference: number): string {
  const sign = delta > 0 ? "+" : delta < 0 ? "-" : "";
  const size = Math.abs(delta);
  if (unit === "ratio") return `${sign}${(size * PERCENT).toFixed(1)} pts`;
  if (unit === "hours") {
    return reference >= HOURS_PER_DAY ? `${sign}${(size / HOURS_PER_DAY).toFixed(1)}d` : `${sign}${size.toFixed(1)}h`;
  }
  return `${sign}${formatInt(size)}`;
}

function verdict(delta: number, good: GoodDirection): string {
  if (delta === 0) return "no change";
  return (delta > 0) === (good === "up") ? "better" : "worse";
}

function directionWord(delta: number): { glyph: string; word: string } {
  if (delta > 0) return { glyph: "▲", word: "up" };
  if (delta < 0) return { glyph: "▼", word: "down" };
  return { glyph: "▬", word: "flat" };
}

function incomparableText(metric: FlowCompareMetric, noun: string): string {
  const need = NEED_PATTERN.exec(metric.reason)?.[1];
  const needText = need ? `, need ${need}` : "";
  return `n/a (low sample: ${metric.nA} vs ${metric.nB} ${noun}${needText})`;
}

/** The compare delta of one tile: arrow, signed text and a better/worse word, or an n/a with its reason. */
export function FlowDelta({ comparison, metric: name, good, sampleNoun = "items" }: FlowDeltaProps) {
  if (comparison === null) return null;
  const metric = comparison.metrics.find((candidate) => candidate.name === name);

  if (metric?.state === "incomparable") {
    return (
      <span className={styles.muted} data-testid={`flow-delta-${name}`}>
        {incomparableText(metric, sampleNoun)} (previous {metric.a}, now {metric.b})
      </span>
    );
  }
  if (metric === undefined || metric.delta === undefined) {
    return (
      <span className={styles.muted} data-testid={`flow-delta-${name}`}>
        n/a (no data in previous period)
      </span>
    );
  }
  const { glyph, word } = directionWord(metric.delta);
  const signed = formatSigned(metric.delta, metric.unit, Math.max(Math.abs(metric.a), Math.abs(metric.b)));
  return (
    <span className={styles.secondary} data-testid={`flow-delta-${name}`}>
      vs prev: <span aria-hidden="true">{glyph}</span> {word} {signed} ({verdict(metric.delta, good)})
    </span>
  );
}
