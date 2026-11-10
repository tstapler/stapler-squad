"use client";

import type { FlowRatioMetric } from "@/gen/session/v1/backlog_flow_pb";
import { DrillButton } from "./DrillButton";
import { formatInt, formatPercent, formatRatio, isLowSample } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowRatioProps {
  ratio: FlowRatioMetric | undefined;
  onDrill: (key: string) => void;
  prefix?: string;
}

/** `32/105 (30.5%)`; a small denominator greys the percentage and says so. */
export function FlowRatio({ ratio, onDrill, prefix = "" }: FlowRatioProps) {
  if (!ratio || ratio.den === 0) return <span className={styles.muted}>no data</span>;
  const low = isLowSample(ratio);
  const counts = `${prefix}${formatInt(ratio.num)}/${formatInt(ratio.den)}`;
  return (
    <DrillButton drillKey={ratio.drillKey} onDrill={onDrill} label={`${prefix}${formatRatio(ratio.num, ratio.den)}${low ? ", low sample" : ""}`}>
      {counts} <span className={low ? styles.lowSample : undefined} data-low-sample={low || undefined}>({formatPercent(ratio.num, ratio.den)})</span>
      {low && <span className={styles.lowSample}> low sample</span>}
    </DrillButton>
  );
}
