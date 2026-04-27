"use client";

import { useState, useCallback, useEffect } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UnfinishedWorktree } from "@/gen/session/v1/types_pb";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import {
  DismissWorktreeRequestSchema,
  SnoozeWorktreeRequestSchema,
} from "@/gen/session/v1/unfinished_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { useUnfinishedWork } from "@/lib/hooks/useUnfinishedWork";
import { UnfinishedRepoGroup } from "@/components/unfinished/UnfinishedRepoGroup";
import * as styles from "./UnfinishedTab.css";

type FilterType = "all" | "uncommitted" | "ahead" | "behind";

/**
 * Main Unfinished Work tab component.
 * Groups worktrees by repo, supports filter chips, and handles dismiss/snooze.
 */
export function UnfinishedTab() {
  const { worktrees, lastScanTime, isScanning, triggerScan } = useUnfinishedWork();
  const [filter, setFilter] = useState<FilterType>("all");
  const [secondsAgo, setSecondsAgo] = useState(0);

  const transport = createConnectTransport({
    baseUrl: getApiBaseUrl(),
    interceptors: [createAuthInterceptor()],
  });
  const client = createClient(UnfinishedWorkService, transport);

  // Update "last scanned N seconds ago" counter every second
  useEffect(() => {
    if (!lastScanTime) return;
    const tick = () => {
      setSecondsAgo(Math.floor((Date.now() - lastScanTime.getTime()) / 1000));
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [lastScanTime]);

  const handleDismiss = useCallback(
    async (repoPath: string, branch: string) => {
      try {
        const req = create(DismissWorktreeRequestSchema, { repoPath, branch });
        await client.dismissWorktree(req);
      } catch {
        // ignore
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  const handleSnooze = useCallback(
    async (repoPath: string, branch: string) => {
      try {
        const req = create(SnoozeWorktreeRequestSchema, { repoPath, branch });
        await client.snoozeWorktree(req);
      } catch {
        // ignore
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  // Apply client-side filter
  const filtered = worktrees.filter((wt) => {
    if (filter === "uncommitted") return wt.hasUncommitted;
    if (filter === "ahead") return wt.commitsAhead > 0;
    if (filter === "behind") return wt.commitsBehind > 0;
    return true;
  });

  // Group by repoName
  const groups = new Map<string, UnfinishedWorktree[]>();
  for (const wt of filtered) {
    const name = wt.repoName || wt.repoPath;
    if (!groups.has(name)) groups.set(name, []);
    groups.get(name)!.push(wt);
  }

  const chips: { label: string; value: FilterType }[] = [
    { label: "All", value: "all" },
    { label: "Uncommitted", value: "uncommitted" },
    { label: "Ahead", value: "ahead" },
    { label: "Behind", value: "behind" },
  ];

  return (
    <div className={styles.container}>
      {/* Toolbar */}
      <div className={styles.toolbar}>
        <div className={styles.toolbarLeft}>
          <h1 className={styles.title}>Unfinished Work</h1>
          <div className={styles.scanInfo}>
            {isScanning ? (
              <>
                <span className={styles.spinner} aria-label="Scanning" />
                Scanning…
              </>
            ) : lastScanTime ? (
              `Last scanned ${secondsAgo}s ago`
            ) : (
              "Not yet scanned"
            )}
          </div>
        </div>
        <div className={styles.toolbarRight}>
          <button
            className={styles.btn}
            onClick={triggerScan}
            disabled={isScanning}
            aria-label="Refresh — trigger an immediate scan"
          >
            Refresh
          </button>
        </div>
      </div>

      {/* Filter chips */}
      <div className={styles.filterRow} role="group" aria-label="Filter worktrees">
        {chips.map(({ label, value }) => (
          <button
            key={value}
            className={`${styles.chip} ${filter === value ? styles.chipActive : ""}`}
            onClick={() => setFilter(value)}
            aria-pressed={filter === value}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Repo groups */}
      {groups.size === 0 ? (
        <div className={styles.empty}>
          {worktrees.length === 0
            ? "No unfinished work found. All repos are clean."
            : "No items match the current filter."}
        </div>
      ) : (
        <div className={styles.repoList}>
          {Array.from(groups.entries()).map(([repoName, wts]) => (
            <UnfinishedRepoGroup
              key={repoName}
              repoName={repoName}
              worktrees={wts}
              onDismiss={handleDismiss}
              onSnooze={handleSnooze}
            />
          ))}
        </div>
      )}
    </div>
  );
}
