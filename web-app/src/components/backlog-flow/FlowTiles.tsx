"use client";

import type { ReactNode } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { srOnly } from "@/components/ui/LiveRegion.css";
import type { CompareFlowWindowsResponse, FlowSummary } from "@/gen/session/v1/backlog_flow_pb";
import { DrillButton } from "./DrillButton";
import { FlowDelta } from "./FlowDelta";
import { FlowRatio } from "./FlowRatio";
import { ThroughputSparkline } from "./FlowTrends";
import { durationText, formatInt, plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowTilesProps {
  summary: FlowSummary;
  comparison: CompareFlowWindowsResponse | null;
  onDrill: (key: string) => void;
}

interface TileProps {
  name: string;
  label: string;
  children: ReactNode;
}

function Tile({ name, label, children }: TileProps) {
  return (
    <div className={styles.tile} data-testid={`flow-tile-${name}`}>
      <span className={styles.tileLabel}>{label}</span>
      {children}
    </div>
  );
}

function ThroughputTile({ summary, comparison, onDrill }: FlowTilesProps) {
  const merged = summary.tiles?.merged;
  return (
    <Tile name="throughput" label="Throughput">
      <span className={styles.tileValue}>
        {merged ? (
          <DrillButton drillKey={merged.drillKey} onDrill={onDrill}>
            {formatInt(merged.value)} merged
          </DrillButton>
        ) : (
          "no data"
        )}
      </span>
      <FlowDelta comparison={comparison} metric="merged_items" good="up" />
      <ThroughputSparkline trends={summary.trends} />
    </Tile>
  );
}

function LeadTimeTile({ summary, comparison, onDrill }: FlowTilesProps) {
  const lead = summary.tiles?.leadTime;
  const text = durationText(lead);
  const excluded = summary.tiles?.leadTimeExcluded ?? 0;
  return (
    <Tile name="lead-time" label="Median lead time">
      <span className={styles.tileValue}>
        {lead && text ? (
          <DrillButton drillKey={lead.drillKey} onDrill={onDrill}>
            {text.median}
          </DrillButton>
        ) : (
          "no data"
        )}
      </span>
      {lead && text && <span className={styles.secondary}>p90 {text.p90}, {plural(lead.n, "item")}</span>}
      {excluded > 0 && <span className={styles.muted}>{plural(excluded, "item")} without an in_progress start excluded</span>}
      <FlowDelta comparison={comparison} metric="lead_time_p50_hours" good="down" />
    </Tile>
  );
}

function FpyTile({ summary, comparison, onDrill }: FlowTilesProps) {
  return (
    <Tile name="fpy" label="First-pass yield">
      <span className={styles.tileValue}>
        <FlowRatio ratio={summary.tiles?.firstPassYield} onDrill={onDrill} />
      </span>
      <FlowDelta comparison={comparison} metric="first_pass_yield" good="up" />
    </Tile>
  );
}

function OpenStuckTile({ summary }: FlowTilesProps) {
  const open = summary.stuck?.open;
  return (
    <Tile name="open-stuck" label="Open stuck">
      <span className={styles.tileValue}>{open === undefined ? "no data" : formatInt(open)}</span>
      <AppLink href="/unfinished" className={styles.link}>
        Unfinished<span className={srOnly}> (opens Unfinished page)</span>
      </AppLink>
    </Tile>
  );
}

/** Leads with the adjusted rate: the raw rate is the number this feature itself calls misleading. */
function ReviewFailTile({ summary, comparison, onDrill }: FlowTilesProps) {
  const review = summary.review;
  return (
    <Tile name="review-fail" label="Review FAIL (judgment verdicts only)">
      <span className={styles.tileValue}>
        <FlowRatio ratio={review?.adjustedFail} onDrill={onDrill} />
      </span>
      <span className={styles.secondary}>FAIL, judgment verdicts only</span>
      {review?.rawFail && review.rawFail.den > 0 && (
        <span className={styles.secondary}>
          <FlowRatio ratio={review.rawFail} onDrill={onDrill} prefix="raw " />
        </span>
      )}
      <FlowDelta comparison={comparison} metric="review_adjusted_fail_rate" good="down" sampleNoun="verdicts" />
    </Tile>
  );
}

export function FlowTiles(props: FlowTilesProps) {
  return (
    <div className={styles.tileRow} role="group" aria-label="Health summary">
      <ThroughputTile {...props} />
      <LeadTimeTile {...props} />
      <FpyTile {...props} />
      <OpenStuckTile {...props} />
      <ReviewFailTile {...props} />
    </div>
  );
}
