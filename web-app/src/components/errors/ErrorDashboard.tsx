"use client";
// +feature: error-dashboard

import { useEffect, useState, useRef, useCallback, Fragment } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { ErrorEventRecord } from "@/gen/session/v1/session_pb";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { getApiBaseUrl } from "@/lib/config";
import { formatTimestamp } from "@/lib/utils/datetime";
import * as styles from "./ErrorDashboard.css";

export function ErrorDashboard() {
  const [errors, setErrors] = useState<ErrorEventRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [includeAcknowledged, setIncludeAcknowledged] = useState(false);
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [acknowledging, setAcknowledging] = useState<Set<string>>(new Set());

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  const fetchErrors = useCallback(async () => {
    if (!clientRef.current) return;
    setFetchError(null);
    setLoading(true);
    try {
      const resp = await clientRef.current.listErrors({ includeAcknowledged });
      setErrors(resp.errors);
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [includeAcknowledged]);

  useEffect(() => {
    fetchErrors();
  }, [fetchErrors]);

  const handleAcknowledge = useCallback(async (fingerprint: string) => {
    if (!clientRef.current) return;
    setAcknowledging((prev) => new Set(prev).add(fingerprint));
    try {
      await clientRef.current.acknowledgeError({ fingerprint });
      setErrors((prev) =>
        prev.map((e) =>
          e.fingerprint === fingerprint ? { ...e, acknowledged: true } : e
        )
      );
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : String(e));
    } finally {
      setAcknowledging((prev) => {
        const next = new Set(prev);
        next.delete(fingerprint);
        return next;
      });
    }
  }, []);

  const formatLastSeen = (record: ErrorEventRecord): string => {
    if (!record.lastSeen) return "—";
    return formatTimestamp(timestampDate(record.lastSeen));
  };

  const activeCount = errors.filter((e) => !e.acknowledged).length;
  const displayedErrors = includeAcknowledged
    ? errors
    : errors.filter((e) => !e.acknowledged);

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Error Dashboard</h1>
        <div className={styles.headerActions}>
          <button className={styles.refreshButton} onClick={fetchErrors}>
            Refresh
          </button>
        </div>
      </div>

      {fetchError && (
        <div className={styles.errorState}>{fetchError}</div>
      )}

      <div className={styles.filterRow}>
        <label className={styles.filterLabel}>
          <input
            type="checkbox"
            className={styles.filterCheckbox}
            checked={includeAcknowledged}
            onChange={(e) => setIncludeAcknowledged(e.target.checked)}
          />
          Show acknowledged
        </label>
        <span className={styles.countBadge}>
          {activeCount} active error{activeCount !== 1 ? "s" : ""}
        </span>
      </div>

      {loading ? (
        <div className={styles.loadingState}>Loading errors…</div>
      ) : (
        <div className={styles.tableWrapper}>
          <table className={styles.table}>
            <thead className={styles.thead}>
              <tr>
                <th className={styles.th}></th>
                <th className={styles.th}>Error Type</th>
                <th className={styles.th}>Message</th>
                <th className={styles.th}>Count</th>
                <th className={styles.th}>Last Seen</th>
                <th className={styles.th}>Procedure</th>
                <th className={styles.th}>Status</th>
                <th className={styles.th}></th>
              </tr>
            </thead>
            <tbody>
              {displayedErrors.length === 0 ? (
                <tr>
                  <td colSpan={8} className={styles.emptyState}>
                    {includeAcknowledged
                      ? "No errors recorded."
                      : "No active errors. 🎉"}
                  </td>
                </tr>
              ) : (
                displayedErrors.map((err) => (
                  <Fragment key={err.fingerprint}>
                    <tr
                      key={err.fingerprint}
                      className={`${styles.tr} ${err.acknowledged ? styles.trAcknowledged : ""}`}
                    >
                      <td className={styles.td}>
                        <button
                          className={styles.expandButton}
                          onClick={() =>
                            setExpandedRow(
                              expandedRow === err.fingerprint
                                ? null
                                : err.fingerprint
                            )
                          }
                          title={
                            expandedRow === err.fingerprint
                              ? "Collapse stack trace"
                              : "Expand stack trace"
                          }
                        >
                          {expandedRow === err.fingerprint ? "▾" : "▸"}
                        </button>
                      </td>
                      <td className={styles.td}>
                        <span className={styles.errorType} title={err.errorType}>
                          {err.errorType || "unknown"}
                        </span>
                      </td>
                      <td className={styles.td}>
                        <span className={styles.message} title={err.message}>
                          {err.message || "—"}
                        </span>
                      </td>
                      <td className={styles.td}>
                        <span className={styles.count}>
                          {err.occurrenceCount.toLocaleString()}
                        </span>
                      </td>
                      <td className={styles.td}>
                        <span className={styles.timestamp}>
                          {formatLastSeen(err)}
                        </span>
                      </td>
                      <td className={styles.td}>
                        <span
                          className={styles.procedure}
                          title={err.rpcProcedure}
                        >
                          {err.rpcProcedure || "—"}
                        </span>
                      </td>
                      <td className={styles.td}>
                        <span
                          className={`${styles.statusBadge} ${
                            err.acknowledged
                              ? styles.statusAcknowledged
                              : styles.statusActive
                          }`}
                        >
                          {err.acknowledged ? "Acknowledged" : "Active"}
                        </span>
                      </td>
                      <td className={`${styles.td} ${styles.actionCell}`}>
                        {!err.acknowledged && (
                          <button
                            className={styles.acknowledgeButton}
                            onClick={() => handleAcknowledge(err.fingerprint)}
                            disabled={acknowledging.has(err.fingerprint)}
                          >
                            {acknowledging.has(err.fingerprint)
                              ? "…"
                              : "Acknowledge"}
                          </button>
                        )}
                      </td>
                    </tr>
                    {expandedRow === err.fingerprint && (
                      <tr
                        key={`${err.fingerprint}-trace`}
                        className={styles.stackTraceRow}
                      >
                        <td colSpan={8} className={styles.stackTraceCell}>
                          <pre className={styles.stackTrace}>
                            {err.stackTrace || "No stack trace available."}
                          </pre>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
