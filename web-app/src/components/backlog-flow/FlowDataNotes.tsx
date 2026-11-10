"use client";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { FlowDataQuality, FlowMeta, FlowNotesSection } from "@/gen/session/v1/backlog_flow_pb";
import { formatInt, formatIsoDate, plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowDataNotesProps {
  notes: FlowNotesSection | undefined;
  /** Missing meta means the methodology cannot be described, which must never render blank. */
  meta: FlowMeta | undefined;
}

const COLLAPSE_RULE =
  "Repeated flips between the same two statuses within 15 minutes collapse into one thrash episode; episodes less than 1 hour apart merge. Thrashing items are excluded from averages.";
const SOURCES = "Backlog status-change events, review verdicts and stuck intervals";

function qualityParts(quality: FlowDataQuality | undefined): string[] {
  if (!quality) return [];
  const parts: [number, string, string][] = [
    [quality.selfLoops, "self-loop", "self-loops"],
    [quality.unparseable, "unparseable event", "unparseable events"],
    [quality.negativeGaps, "negative gap", "negative gaps"],
    [quality.unknownSpans, "unknown span", "unknown spans"],
  ];
  return parts.filter(([n]) => n > 0).map(([n, one, many]) => plural(n, one, many));
}

function EstimatedList({ meta }: { meta: FlowMeta }) {
  if (meta.estimated.length === 0) return null;
  return (
    <ul>
      {meta.estimated.map((code) => (
        <li key={code}>
          <code>{code}</code>: {meta.estimatedLegend[code] ?? "heuristic metric, no explanation recorded"}
        </li>
      ))}
    </ul>
  );
}

/** The body of the closed-by-default Data notes section. */
export function FlowDataNotes({ notes, meta }: FlowDataNotesProps) {
  if (!meta) return <p>Methodology details unavailable</p>;
  const counts = meta.rowCounts;
  const quality = qualityParts(notes?.quality);
  return (
    <div className={styles.block} data-testid="flow-data-notes">
      <p>{COLLAPSE_RULE}</p>
      <p>
        Sources: {SOURCES}.
        {counts && ` Rows read: ${plural(counts.events, "event")}, ${plural(counts.verdicts, "verdict")}, ${plural(counts.stuckIntervals, "stuck interval")}, ${plural(counts.items, "item")}.`}
      </p>
      {notes && (
        <p>
          {formatInt(notes.rawEvents)} raw events, {plural(notes.realSpans, "real span")}; {formatInt(notes.collapsedEvents)} raw events collapsed into
          thrash episodes.
        </p>
      )}
      <p>
        Query {formatInt(Number(meta.queryMs))} ms
        {meta.windowStart && meta.windowEnd && `, window ${formatIsoDate(timestampDate(meta.windowStart))} to ${formatIsoDate(timestampDate(meta.windowEnd))}`}.
      </p>
      <p>Archived excluded from stages, included in throughput.</p>
      <EstimatedList meta={meta} />
      {meta.clamps.length > 0 && <p>Adjusted requests: {meta.clamps.join("; ")}.</p>}
      {quality.length > 0 && <p>Data quality: {quality.join(", ")}.</p>}
      {notes && notes.notes.length > 0 && (
        <ul>
          {notes.notes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
