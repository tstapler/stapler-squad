"use client";

// +feature: up-next-tabs

import { useState, useCallback, useEffect, useMemo } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
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
import { routes } from "@/lib/routes";
import { useUnfinishedWork } from "@/lib/hooks/useUnfinishedWork";
import { UnfinishedRepoGroup } from "@/components/unfinished/UnfinishedRepoGroup";
import { GitHubPRsSection } from "@/components/unfinished/GitHubPRsSection";
import { BacklogQueueSection } from "@/components/unfinished/BacklogQueueSection";
import { StuckItemsSection } from "@/components/backlog-stuck/StuckItemsSection";
import { UpNextTabs } from "@/components/unfinished/UpNextTabs";
import { useGitHubPRs } from "@/lib/hooks/useGitHubPRs";
import { usePRListFilters } from "@/lib/hooks/usePRListFilters";
import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import { useUpNextTab } from "@/lib/hooks/useUpNextTab";
import { DEGRADED_ATTENTION_TEXT, summarizeAttention } from "@/lib/unfinished/prAttention";
import * as styles from "./UnfinishedTab.css";

const PR_TAB_DESCRIPTION =
  "PRs with failing CI, conflicts, unresolved threads or requested changes. The sidebar badge uses its own count.";

type FilterType = "all" | "uncommitted" | "ahead" | "behind";

/**
 * Main Unfinished Work tab component.
 * Groups worktrees by repo, supports filter chips, and handles dismiss/snooze.
 */
export function UnfinishedTab() {
  const { worktrees, lastScanTime, isScanning, triggerScan } = useUnfinishedWork();
  const [filter, setFilter] = useState<FilterType>("all");
  const [secondsAgo, setSecondsAgo] = useState(0);
  const searchParams = useSearchParams();
  const { tab, setTab } = useUpNextTab();
  // Lifted here so the tab badge and the PRs panel share one WatchUserPRs stream.
  const gitHubPRs = useGitHubPRs();
  const prFilters = usePRListFilters();
  // Shared poll via StuckBacklogItemsProvider; no second poller for the badge/notice.
  const { items: stuckItems, lastFetched: stuckLastFetched } = useStuckBacklogItems();
  // routes.unfinishedItem(itemId) deep link — pre-expands/filters the
  // matching Stuck Backlog Items card (see StuckItemsSection's focusItemId).
  const focusItemId = searchParams.get("item") ?? undefined;

  const transport = useMemo(
    () =>
      createConnectTransport({
        baseUrl: getApiBaseUrl(),
        interceptors: [createAuthInterceptor()],
      }),
    []
  );
  const client = useMemo(() => createClient(UnfinishedWorkService, transport), [transport]);

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

  const attention = summarizeAttention(gitHubPRs.prs);
  const stuckLoaded = stuckLastFetched !== null;
  const itemMissing =
    focusItemId !== undefined && stuckLoaded && !stuckItems.some((i) => i.itemId === focusItemId);

  const showAllStuck = useCallback(() => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("item");
    params.set("tab", "stuck");
    // history.replaceState, like useUpNextTab.setTab: router.replace hard-reloads the static export,
    // which would drop the focus moved below.
    window.history.replaceState(window.history.state, "", `?${params.toString()}`);
    document.querySelector<HTMLElement>('[data-testid="up-next-panel-stuck"] h2')?.focus();
  }, [searchParams]);

  const prsDescription = attention.degraded
    ? `At least ${attention.count} PRs need attention. ${DEGRADED_ATTENTION_TEXT} ${PR_TAB_DESCRIPTION}`
    : PR_TAB_DESCRIPTION;

  const panels = {
    prs: (
      <GitHubPRsSection
        {...gitHubPRs}
        filters={prFilters}
        attention={attention}
      />
    ),
    stuck: (
      <>
        {itemMissing && (
          <div className={styles.notice} role="status">
            <span>Item {focusItemId} was not found. It may have been completed or removed.</span>
            <button className={styles.btn} onClick={showAllStuck}>
              Show all stuck items
            </button>
          </div>
        )}
        <StuckItemsSection focusItemId={focusItemId} />
      </>
    ),
    worktrees: (
      <>
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
      </>
    ),
    queue: <BacklogQueueSection />,
  };

  return (
    <div className={styles.container}>
      {/* Toolbar */}
      <div className={styles.toolbar}>
        <div className={styles.toolbarLeft}>
          <h1 className={styles.title}>Up Next</h1>
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
          <Link href={routes.settingsUnfinished} className={styles.btn} aria-label="Configure scan sources">
            Sources
          </Link>
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

      <UpNextTabs
        value={tab}
        onValueChange={setTab}
        badges={{ prs: attention.count, stuck: stuckLoaded ? stuckItems.length : 0 }}
        degraded={{ prs: attention.degraded }}
        descriptions={{ prs: prsDescription }}
        panels={panels}
      />
    </div>
  );
}
