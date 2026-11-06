"use client";

import { EstimatedValue } from "@/components/ui/EstimatedValue";
import type { FlowCausesSection, FlowPRSection } from "@/gen/session/v1/backlog_flow_pb";
import { DrillButton } from "./DrillButton";
import { durationText, formatInt, formatIsoDate } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowPRProps {
  pr: FlowPRSection;
  /** The causes section carries the PR-fix trigger counts. */
  causes: FlowCausesSection | undefined;
  /** Estimated codes of the PR section. */
  estimated: readonly string[];
  /** `FlowMeta.capture_since`; null when no trigger capture exists yet. */
  captureSince: Date | null;
  onDrill: (key: string) => void;
}

const DWELL_COMBINED = "pr_fix_dwell:combined";
const CAUSE_UNAVAILABLE = "pr_fix_cause:unavailable";
const CAUSE_SINCE_CAPTURE = "pr_fix_cause:since_capture";
const DWELL_SPLIT_SINCE_CAPTURE = "pr_fix_dwell_split:since_capture";

const dateOrNothing = (captureSince: Date | null): string => (captureSince ? ` ${formatIsoDate(captureSince)}` : "");

function CombinedDwell({ pr, estimated, captureSince, onDrill }: FlowPRProps) {
  const text = durationText(pr.fixDwell);
  if (!pr.fixDwell || !text) return null;
  const label = `median ${text.median}, p90 ${text.p90}, N ${pr.fixDwell.n}`;
  const value = (
    <DrillButton drillKey={pr.fixDwell.drillKey} onDrill={onDrill}>
      {label}
    </DrillButton>
  );
  const combined = estimated.includes(DWELL_COMBINED);
  return (
    <p>
      PR-fix dwell:{" "}
      {combined ? <EstimatedValue title="Combined PR-fix dwell; CI and conflict are not separable">{value}</EstimatedValue> : value}
      {combined && <span className={styles.muted}> CI and conflict dwell are not separable before{captureSince ? dateOrNothing(captureSince) : " trigger capture"}</span>}
    </p>
  );
}

function CauseBucket({ causes, estimated, captureSince, onDrill }: FlowPRProps) {
  const triggers = causes?.prFixTriggers ?? [];
  const prFix = causes?.rework.find((row) => row.cause === "pr_fix");

  if (estimated.includes(CAUSE_SINCE_CAPTURE) && triggers.length > 0) {
    return (
      <div data-testid="flow-pr-triggers" className={styles.block}>
        <span className={styles.secondary}>
          <EstimatedValue title="Trigger split exists only since capture began">
            events since{dateOrNothing(captureSince)}
          </EstimatedValue>
          {estimated.includes(DWELL_SPLIT_SINCE_CAPTURE) && " (dwell split flagged the same way)"}
        </span>
        <ul className={styles.row}>
          {triggers.map((row) => (
            <li key={row.trigger}>
              {row.trigger}:{" "}
              <DrillButton drillKey={row.drillKey} onDrill={onDrill}>
                {formatInt(row.count)}
              </DrillButton>
            </li>
          ))}
        </ul>
      </div>
    );
  }
  if (!estimated.includes(CAUSE_UNAVAILABLE) || !prFix) return null;
  return (
    <p>
      PR fix (CI/review/conflict not separable):{" "}
      <DrillButton drillKey={prFix.count?.drillKey ?? ""} onDrill={onDrill}>
        {formatInt(prFix.count?.value ?? 0)}
      </DrillButton>
      <span className={styles.muted}> CI vs conflict split not recorded {captureSince ? `before ${formatIsoDate(captureSince)}` : "yet"}</span>
    </p>
  );
}

/** Outcome capture is not part of the flow response yet, so this never invents numbers. */
function Outcomes() {
  return (
    <p className={styles.muted} data-testid="flow-pr-outcomes">
      Outcome tracking not yet recording
    </p>
  );
}

export function FlowPR(props: FlowPRProps) {
  const { pr, onDrill } = props;
  const none = (pr.cycles?.value ?? 0) === 0 && (pr.fixDwell?.n ?? 0) === 0;
  if (none) return <p>No PR fixes in this window</p>;
  return (
    <div className={styles.block} data-testid="flow-pr">
      <p>
        Reopened for PR fix:{" "}
        <DrillButton drillKey={pr.cycles?.drillKey ?? ""} onDrill={onDrill}>
          {formatInt(pr.cycles?.value ?? 0)}
        </DrillButton>
      </p>
      <CombinedDwell {...props} />
      <CauseBucket {...props} />
      <Outcomes />
    </div>
  );
}
