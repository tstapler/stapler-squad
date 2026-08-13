"use client";
// +feature: import-external-sessions-panel

import React, { useMemo, useState, useCallback } from "react";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { InstanceType } from "@/gen/session/v1/types_pb";
import {
  ImportSourceKind,
  type ExternalSessionCandidateRef,
} from "@/gen/session/v1/import_pb";
import * as styles from "./ImportExternalSessionsPanel.css";

export interface ImportExternalSessionsPanelProps {
  onImport?: (candidates: ExternalSessionCandidateRef[]) => void;
  onRefresh?: () => void;
}

interface DiscoveredRow {
  key: string;
  sessionId: string;
  title: string;
  path: string;
  program: string;
  sourceTerminal: string;
  candidate: ExternalSessionCandidateRef;
}

function toCandidate(session: {
  path: string;
  program: string;
  externalMetadata?: {
    originalPid: number;
    tmuxSessionName: string;
    muxSocketPath: string;
    muxEnabled: boolean;
  };
}): ExternalSessionCandidateRef {
  const meta = session.externalMetadata;
  const sourceKind =
    meta?.muxEnabled && meta?.muxSocketPath
      ? ImportSourceKind.MUX_DISCOVERED
      : ImportSourceKind.PLAIN_TMUX;

  return {
    sourceKind,
    path: session.path,
    program: session.program,
    pid: meta?.originalPid ?? 0,
    tmuxSession: meta?.tmuxSessionName ?? "",
    socketPath: meta?.muxSocketPath ?? "",
  } as ExternalSessionCandidateRef;
}

export function ImportExternalSessionsPanel({
  onImport,
  onRefresh,
}: ImportExternalSessionsPanelProps) {
  const sessions = useAppSelector(selectAllSessions);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const rows = useMemo<DiscoveredRow[]>(() => {
    return sessions
      .filter((session) => session.instanceType === InstanceType.EXTERNAL)
      .map((session) => ({
        key: session.id,
        sessionId: session.id,
        title: session.title,
        path: session.path,
        program: session.program,
        sourceTerminal: session.externalMetadata?.sourceTerminal ?? "",
        candidate: toCandidate(session),
      }));
  }, [sessions]);

  const allChecked = rows.length > 0 && selected.size === rows.length;

  const toggleAll = useCallback(() => {
    setSelected((prev) =>
      prev.size === rows.length ? new Set() : new Set(rows.map((r) => r.key))
    );
  }, [rows]);

  const toggleRow = useCallback((key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const importRow = useCallback(
    (row: DiscoveredRow) => {
      onImport?.([row.candidate]);
    },
    [onImport]
  );

  const importSelected = useCallback(() => {
    const candidates = rows
      .filter((r) => selected.has(r.key))
      .map((r) => r.candidate);
    onImport?.(candidates);
  }, [rows, selected, onImport]);

  return (
    <div className={styles.panel} data-testid="import-external-sessions-panel">
      <div className={styles.titleRow}>
        <div>
          <h2 className={styles.title}>External Sessions</h2>
          <p className={styles.subtitle}>
            Terminal sessions discovered outside Stapler Squad that can be
            imported for management.
          </p>
        </div>
        <button
          className={styles.refreshButton}
          onClick={onRefresh}
          aria-label="Refresh discovered sessions"
          type="button"
        >
          ↻
        </button>
      </div>

      {selected.size > 0 && (
        <div className={styles.bulkActionBar}>
          <span className={styles.bulkActionCount}>
            {selected.size} selected
          </span>
          <button
            className={styles.bulkImportBtn}
            onClick={importSelected}
            type="button"
          >
            Import selected →
          </button>
          <button
            className={styles.bulkClearBtn}
            onClick={() => setSelected(new Set())}
            type="button"
          >
            Clear
          </button>
        </div>
      )}

      {rows.length === 0 ? (
        <div className={styles.empty} data-testid="import-external-sessions-empty">
          No external sessions detected. Sessions started outside Stapler
          Squad will appear here for import.
        </div>
      ) : (
        <div className={styles.tableWrapper}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th className={styles.checkboxTh}>
                  <input
                    type="checkbox"
                    checked={allChecked}
                    onChange={toggleAll}
                    aria-label="Select all"
                  />
                </th>
                <th className={styles.th}>Title</th>
                <th className={styles.th}>Path</th>
                <th className={styles.th}>Program</th>
                <th className={styles.th}>Source</th>
                <th className={styles.th}></th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr
                  key={row.key}
                  className={styles.row}
                  data-testid="import-external-session-row"
                >
                  <td
                    className={styles.checkboxTd}
                    onClick={(e) => e.stopPropagation()}
                  >
                    <input
                      type="checkbox"
                      checked={selected.has(row.key)}
                      onChange={() => toggleRow(row.key)}
                      aria-label={`Select ${row.title}`}
                    />
                  </td>
                  <td className={styles.td}>{row.title}</td>
                  <td className={`${styles.td} ${styles.pathText}`}>
                    {row.path}
                  </td>
                  <td className={styles.td}>{row.program}</td>
                  <td className={styles.td}>
                    {row.sourceTerminal ? (
                      <span className={styles.sourceBadge}>
                        {row.sourceTerminal}
                      </span>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className={styles.td}>
                    <button
                      className={styles.rowImportBtn}
                      onClick={() => importRow(row)}
                      type="button"
                      data-testid="import-row-button"
                    >
                      Import
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
