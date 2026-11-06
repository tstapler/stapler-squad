"use client";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { formatClock } from "@/app/backlog/flow/flowFormat";
import { AppLink } from "@/components/ui/AppLink";
import type { FlowDrillItem } from "@/gen/session/v1/backlog_flow_pb";
import type { FlowWindow } from "@/lib/hooks/useBacklogFlow";
import { useFlowDrilldown, type UseFlowDrilldownReturn } from "@/lib/hooks/useFlowDrilldown";
import { useSearchParams } from "@/lib/nav";
import { DialogError, DialogSkeleton, FlowDialog } from "./FlowDialog";
import { drillTitle, windowPhrase } from "./drillLabels";
import { formatDurationMs, formatInt, plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowDrilldownDrawerProps {
  /** A drill key the page already validated against the registry. */
  drillKey: string;
  window: FlowWindow;
  returnFocusTo: HTMLElement | null;
  onClose: () => void;
  /** The List view URL that shows the same items, when a filter for this key exists. */
  listHrefFor?: (key: string) => string | null;
}

/** Keeps the flow page's other parameters but swaps `drill` for `item`, so Back returns to the drawer. */
function itemHref(params: URLSearchParams | { toString(): string }, itemId: string): string {
  const next = new URLSearchParams(params.toString());
  next.delete("drill");
  next.set("item", itemId);
  return `?${next.toString()}`;
}

function ItemRow({ item, href }: { item: FlowDrillItem; href: string }) {
  return (
    <tr>
      <th scope="row" className={styles.cell}>
        <AppLink href={href} className={styles.link}>
          {item.title}
        </AppLink>
      </th>
      <td className={styles.cell}>
        <span className={styles.chip}>{item.status}</span>
      </td>
      <td className={styles.cell}>{item.reason}</td>
      <td className={styles.cell}>{item.durationMs > 0n ? formatDurationMs(item.durationMs) : "n/a"}</td>
    </tr>
  );
}

interface CountNoteProps {
  drill: UseFlowDrilldownReturn;
  asOf: string;
  listHref: string | null;
}

/** Says how the rows relate to the count, and offers the one action that can fix a gap. */
function CountNote({ drill, asOf, listHref }: CountNoteProps) {
  const { items, totalCount, hasMore, truncated } = drill;
  if (items.length === 0) {
    return (
      <p className={styles.dialogMeta}>
        No items match (the count may have changed since as-of {asOf}){" "}
        <button type="button" className={styles.textButton} onClick={drill.reload}>
          Refresh
        </button>
      </p>
    );
  }
  if (items.length < totalCount && (truncated || hasMore)) {
    return (
      <p className={styles.dialogMeta}>
        Showing {formatInt(items.length)} of {formatInt(totalCount)} items{" "}
        {truncated && listHref && <AppLink href={listHref}>Open full list in List view</AppLink>}
        {!truncated && hasMore && (
          <button type="button" className={styles.textButton} disabled={drill.loadingMore} onClick={drill.loadMore}>
            {drill.loadingMore ? "Loading..." : "Load more"}
          </button>
        )}
      </p>
    );
  }
  if (items.length !== totalCount) {
    return (
      <p className={styles.dialogMeta}>
        Counts changed since {asOf}; refresh{" "}
        <button type="button" className={styles.textButton} onClick={drill.reload}>
          Refresh
        </button>
      </p>
    );
  }
  return null;
}

/** The items behind one number. Open state lives in the URL (`drill`), so the page controls mounting. */
export function FlowDrilldownDrawer({ drillKey, window, returnFocusTo, onClose, listHrefFor }: FlowDrilldownDrawerProps) {
  const drill = useFlowDrilldown(drillKey, window);
  const params = useSearchParams();
  const ready = drill.phase === "ready";
  const asOfDate = drill.meta?.asOf ? timestampDate(drill.meta.asOf) : null;
  const asOf = asOfDate ? formatClock(asOfDate) : "the last refresh";
  const heading = `${drillTitle(drillKey)}, ${windowPhrase(window)}${ready ? `, ${plural(drill.totalCount, "item")}` : ""}`;
  return (
    <FlowDialog title={heading} testId="flow-drilldown-drawer" onClose={onClose} returnFocusTo={returnFocusTo} busy={drill.phase === "loading"}>
      <p className={styles.dialogMeta}>Window {window === "all" ? "all time" : `${window}d`}</p>
      {drill.phase === "loading" && <DialogSkeleton />}
      {drill.phase === "error" && <DialogError message="Could not load items" onRetry={drill.reload} />}
      {ready && drill.items.length > 0 && (
        <div className={styles.scrollRegion} role="region" aria-label="Items" tabIndex={0}>
          <table className={styles.table}>
            <caption className={styles.caption}>Items behind {drillTitle(drillKey)}</caption>
            <thead>
              <tr>
                {["Title", "Status", "Reason", "Duration"].map((column) => (
                  <th key={column} scope="col" className={styles.cell}>
                    {column}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {drill.items.map((item) => (
                <ItemRow key={item.itemId} item={item} href={itemHref(params, item.itemId)} />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {ready && <CountNote drill={drill} asOf={asOf} listHref={listHrefFor?.(drillKey) ?? null} />}
      {drill.moreError && <DialogError message={drill.moreError} onRetry={drill.loadMore} />}
    </FlowDialog>
  );
}
