"use client";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { FlowTimelineEntry } from "@/gen/session/v1/backlog_flow_pb";
import type { FlowWindow } from "@/lib/hooks/useBacklogFlow";
import { useFlowItemTimeline } from "@/lib/hooks/useFlowItemTimeline";
import { DialogError, DialogSkeleton, FlowDialog } from "./FlowDialog";
import { formatInt } from "./flowFormatters";
import * as styles from "./flow.css";

/** The server collapses thrash into episodes; this caps what one dialog lists regardless. */
export const MAX_TIMELINE_SPANS = 50;

interface FlowItemTimelineProps {
  itemId: string;
  window: FlowWindow;
  returnFocusTo: HTMLElement | null;
  onClose: () => void;
}

const stamp = (entry: FlowTimelineEntry["at"]): string => (entry ? timestampDate(entry).toISOString().slice(0, 16).replace("T", " ") : "unknown time");

function describe(entry: FlowTimelineEntry): string {
  const pair = `${entry.from} <-> ${entry.to}`;
  if (entry.kind !== "thrash_episode") return `${entry.from} -> ${entry.to} at ${stamp(entry.at)}`;
  const cause = entry.cause === "" ? "" : `, cause ${entry.cause}`;
  return `Thrash episode: ${pair}, ${formatInt(entry.events)} events, ${stamp(entry.at)} to ${stamp(entry.last)}${cause}`;
}

/** One item's collapsed timeline: thrash episodes are single rows, so 87,000 raw events never render. */
export function FlowItemTimeline({ itemId, window, returnFocusTo, onClose }: FlowItemTimelineProps) {
  const { state, reload } = useFlowItemTimeline(itemId, window);
  const data = state.status === "ready" ? state.data : null;
  const shown = data ? data.entries.slice(0, MAX_TIMELINE_SPANS) : [];
  const total = data ? Math.max(data.totalEntries, data.entries.length) : 0;
  return (
    <FlowDialog title={data ? `Events for ${data.title}` : "Item events"} testId="flow-item-timeline" onClose={onClose} returnFocusTo={returnFocusTo} busy={state.status === "loading"}>
      {state.status === "loading" && <DialogSkeleton />}
      {state.status === "error" && <DialogError message="Could not load events" onRetry={reload} />}
      {data && (
        <>
          {shown.length === 0 ? (
            <p>No events for this item in this window</p>
          ) : (
            <ol className={styles.dialogRows} aria-label="Item events">
              {shown.map((entry, index) => (
                <li key={index}>{describe(entry)}</li>
              ))}
            </ol>
          )}
          {total > shown.length && <p className={styles.dialogMeta}>Showing {shown.length} of {formatInt(total)} spans</p>}
        </>
      )}
    </FlowDialog>
  );
}
