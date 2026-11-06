"use client";

import { srOnly } from "@/components/ui/LiveRegion.css";
import type { FlowFunnelSection, FlowFunnelStageRow } from "@/gen/session/v1/backlog_flow_pb";
import { DrillButton } from "./DrillButton";
import { durationText, formatInt, plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowFunnelProps {
  funnel: FlowFunnelSection;
  /** Raw events collapsed into thrash episodes (`notes.collapsed_events`), named beside the real reopens. */
  collapsedEvents: number;
  onDrill: (key: string) => void;
}

const DWELL_MISSING_REASON = "No dwell samples for this stage";
const PERCENT = 100;

function BackEdge({ stage, funnel, collapsedEvents, onDrill }: { stage: FlowFunnelStageRow } & FlowFunnelProps) {
  const episodes = funnel.thrash?.n ?? 0;
  const excluded =
    episodes > 0 && collapsedEvents > 0 ? ` (${formatInt(collapsedEvents)} raw events in ${plural(episodes, "thrash episode")} excluded)` : "";
  return (
    <div className={styles.backEdge}>
      <DrillButton drillKey={stage.entered?.drillKey ?? ""} onDrill={onDrill}>
        {stage.status} -&gt; earlier stage: {formatInt(stage.backEdges)} real reopens
      </DrillButton>
      {excluded && <span>{excluded}</span>}
    </div>
  );
}

function StageRow({ stage, index, total, widest, ...rest }: { stage: FlowFunnelStageRow; index: number; total: number; widest: number } & FlowFunnelProps) {
  const entered = stage.entered?.value ?? 0;
  const dwell = durationText(stage.dwell);
  return (
    <li className={styles.funnelStage} data-testid={`flow-funnel-stage-${stage.status}`}>
      <span className={styles.muted}>
        Stage {index + 1} of {total}: {stage.status}
      </span>
      {entered === 0 ? (
        <span>no items entered</span>
      ) : (
        <>
          <div className={styles.funnelBarTrack} aria-hidden="true">
            <div className={styles.funnelBar} style={{ width: `${(entered / widest) * PERCENT}%` }} />
          </div>
          <span className={styles.row}>
            <DrillButton drillKey={stage.entered?.drillKey ?? ""} onDrill={rest.onDrill}>
              entered {formatInt(entered)}
            </DrillButton>
            <span>advanced {formatInt(stage.advanced)}</span>
          </span>
        </>
      )}
      {dwell ? (
        <span>
          median {dwell.median} p90 {dwell.p90}
        </span>
      ) : (
        <span title={DWELL_MISSING_REASON}>dwell n/a</span>
      )}
      {stage.backEdges > 0 && <BackEdge stage={stage} {...rest} />}
    </li>
  );
}

function FunnelTable({ stages }: { stages: readonly FlowFunnelStageRow[] }) {
  return (
    <div className={srOnly}>
      <table data-testid="flow-funnel-table">
        <caption>Funnel data (mirrors the bars)</caption>
        <thead>
          <tr>
            {["Stage", "Entered", "Advanced", "Median dwell", "p90 dwell", "Back-edges"].map((heading) => (
              <th key={heading} scope="col">
                {heading}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {stages.map((stage) => {
            const dwell = durationText(stage.dwell);
            return (
              <tr key={stage.status}>
                <th scope="row">{stage.status}</th>
                <td>{stage.entered?.value ?? 0}</td>
                <td>{stage.advanced}</td>
                <td>{dwell?.median ?? "n/a"}</td>
                <td>{dwell?.p90 ?? "n/a"}</td>
                <td>{stage.backEdges}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

/** Stages render in the order the response carries (the pipeline definition), never from a constant. */
export function FlowFunnel(props: FlowFunnelProps) {
  const { funnel } = props;
  const widest = Math.max(1, ...funnel.stages.map((stage) => stage.entered?.value ?? 0));
  return (
    <div className={styles.block}>
      <ol className={styles.funnelList} aria-label="Funnel stages">
        {funnel.stages.map((stage, index) => (
          <StageRow key={stage.status} stage={stage} index={index} total={funnel.stages.length} widest={widest} {...props} />
        ))}
        {funnel.other > 0 && (
          <li className={styles.funnelStage} data-testid="flow-funnel-other">
            Other stages: {funnel.other}
          </li>
        )}
      </ol>
      <FunnelTable stages={funnel.stages} />
      <span className={styles.muted}>Archived items are excluded from stages ({formatInt(funnel.archived)}) and included in throughput.</span>
    </div>
  );
}
