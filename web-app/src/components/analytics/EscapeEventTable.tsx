// +feature: escape-analytics

import type { EscapeEventProto } from "@/gen/session/v1/session_pb";
import * as styles from "./EscapeEventTable.css";

interface EscapeEventTableProps {
  events: EscapeEventProto[];
  loading: boolean;
  onLoadMore: () => void;
  hasMore: boolean;
}

/**
 * Convert a Uint8Array to a hex string for safe display.
 * IMPORTANT: raw_bytes MUST NEVER be passed to a terminal renderer — display as hex only.
 */
function toHexString(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join(" ");
}

function formatTimestamp(wallTime: { seconds: bigint; nanos: number } | undefined): string {
  if (!wallTime) return "—";
  const ms = Number(wallTime.seconds) * 1000 + Math.floor(wallTime.nanos / 1_000_000);
  const d = new Date(ms);
  return d.toISOString().replace("T", " ").replace("Z", "");
}

export function EscapeEventTable({
  events,
  loading,
  onLoadMore,
  hasMore,
}: EscapeEventTableProps) {
  if (!loading && events.length === 0) {
    return (
      <div className={styles.container}>
        <p className={styles.emptyState}>No escape events found.</p>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <table className={styles.table} data-testid="escape-event-table">
        <thead className={styles.thead}>
          <tr>
            <th className={styles.th} scope="col">Time</th>
            <th className={styles.th} scope="col">Stage</th>
            <th className={styles.th} scope="col">Sequence Type</th>
            <th className={styles.th} scope="col">Subtype</th>
            <th className={styles.th} scope="col">Byte Length</th>
            <th className={styles.th} scope="col">Mangled</th>
            <th className={styles.th} scope="col">Mangle Type</th>
            <th className={styles.th} scope="col">Raw Bytes (hex)</th>
          </tr>
        </thead>
        <tbody>
          {events.map((event) => (
            <tr
              key={event.id}
              className={event.mangled ? styles.trMangled : styles.tr}
              data-testid="escape-event-row"
              data-mangled={event.mangled ? "true" : "false"}
            >
              <td className={styles.td}>
                <span className={styles.codeCell}>
                  {formatTimestamp(event.wallTime as { seconds: bigint; nanos: number } | undefined)}
                </span>
              </td>
              <td className={styles.td}>
                <span className={styles.codeCell}>{event.stage || "—"}</span>
              </td>
              <td className={styles.td}>
                <span className={styles.codeCell}>{event.sequenceType || "—"}</span>
              </td>
              <td className={styles.td}>
                <span className={styles.codeCell}>{event.sequenceSubtype || "—"}</span>
              </td>
              <td className={styles.td}>{event.byteLength}</td>
              <td className={styles.td}>
                {event.mangled ? (
                  <span className={styles.mangledBadge} aria-label="Mangled">Yes</span>
                ) : (
                  <span>No</span>
                )}
              </td>
              <td className={styles.td}>
                <span className={styles.codeCell}>{event.mangleType || "—"}</span>
              </td>
              <td className={styles.td}>
                {/* SECURITY: raw_bytes rendered as hex string ONLY — never passed to terminal renderer */}
                <span className={styles.codeCell} title="Raw bytes (hex)" aria-label="Raw bytes as hex">
                  {event.rawBytes.length > 0 ? toHexString(event.rawBytes) : "—"}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {loading && (
        <p className={styles.loadingState} role="status" aria-live="polite">
          Loading…
        </p>
      )}

      {!loading && hasMore && (
        <div className={styles.loadMoreRow}>
          <button
            className={styles.loadMoreButton}
            onClick={onLoadMore}
            data-testid="load-more-button"
          >
            Load more
          </button>
        </div>
      )}
    </div>
  );
}
