"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { AppLink } from "@/components/ui/AppLink";
import { FlowDrilldownDrawer } from "@/components/backlog-flow/FlowDrilldownDrawer";
import { FlowItemTimeline } from "@/components/backlog-flow/FlowItemTimeline";
import { FlowSectionBody, NO_ARCHIVED_IDS } from "@/components/backlog-flow/FlowSectionBody";
import { ThrashBanner } from "@/components/backlog-flow/ThrashBanner";
import { BacklogViewTabs } from "@/components/backlog/BacklogViewTabs";
import { ErrorState } from "@/components/ui/ErrorState";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useBacklogFlow, type FlowSettled, type FlowTooLarge, type FlowWindow } from "@/lib/hooks/useBacklogFlow";
import { usePathname, useRouter, useSearchParams } from "@/lib/nav";
import { FlowHeader } from "./FlowHeader";
import { FlowSection } from "./FlowSection";
import { formatClock, widerWindow, windowLabel } from "./flowFormat";
import { FLOW_SECTIONS, needsAttention, sectionStatus, sectionSummaryLine, type FlowSectionId } from "./flowSections";
import { buildFlowSearch, parseFlowUrlState, type FlowCompare, type FlowSearchPatch, type FlowUrlNotice } from "./flowUrlState";
import * as styles from "./flow.css";

const CLOCK_TICK_MS = 5_000;
const NOTICE_TEXT: Record<FlowUrlNotice, string> = {
  "unsupported-window": "Unsupported window, showing 30d",
  "unknown-drill": "Unknown drill target ignored",
};
const SHORTER_WINDOWS = [7, 30, 90] as const;

function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(timer);
  }, [intervalMs]);
  return now;
}

interface WindowTooLargeProps {
  window: FlowWindow;
  tooLarge: FlowTooLarge | null;
  onPick: (window: FlowWindow) => void;
}

/** Retrying cannot succeed, so this state offers shorter windows instead of Retry. */
function WindowTooLarge({ window, tooLarge, onPick }: WindowTooLargeProps) {
  const detail = tooLarge?.rows !== undefined ? ` (${tooLarge.rows} rows, limit ${tooLarge.limit})` : "";
  const choices = SHORTER_WINDOWS.filter((days) => window === "all" || days < window);
  return (
    <div className={styles.stateBlock} data-testid="flow-window-too-large">
      <p>
        {windowLabel(window)} has too many events to load{detail}. Pick a shorter window.
      </p>
      <div className={styles.stateActions}>
        {choices.map((days, index) => (
          <button key={days} type="button" className={styles.textButton} autoFocus={index === 0} onClick={() => onPick(days)}>
            {days}d
          </button>
        ))}
      </div>
    </div>
  );
}

function EmptyWindow({ window, onWiden }: { window: FlowWindow; onWiden: (window: FlowWindow) => void }) {
  const wider = widerWindow(window);
  return (
    <div className={styles.stateBlock}>
      <p>{window === "all" ? "No items have finished yet" : `No items finished in the last ${window} days`}</p>
      {wider !== null && (
        <button type="button" className={styles.textButton} onClick={() => onWiden(wider)}>
          Widen to {windowLabel(wider)}
        </button>
      )}
    </div>
  );
}

function StaleBanner({ since, onRetry }: { since: Date; onRetry: () => void }) {
  return (
    <div className={styles.staleBanner} role="status" data-testid="flow-stale-banner">
      <span className={styles.staleLabel}>Stale</span>
      <span>Showing data from {formatClock(since)} - refresh failed</span>
      <button type="button" className={styles.textButton} onClick={onRetry}>
        Retry
      </button>
    </div>
  );
}

function HardError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className={styles.stateBlock} data-testid="flow-hard-error">
      <ErrorState title="Could not load flow data" message={message} onRetry={onRetry} actionLabel="Retry" />
      <AppLink href="/backlog/board">Back to Board</AppLink>
    </div>
  );
}

/** Window, compare and notices live in the URL; invalid parameters are replaced in place and explained once. */
function useFlowUrl() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const url = useMemo(() => parseFlowUrlState(searchParams), [searchParams]);
  const [notices, setNotices] = useState<FlowUrlNotice[]>([]);

  useEffect(() => {
    if (!url.needsRewrite) return;
    setNotices(url.notices);
    router.replace(url.cleanSearch === "" ? pathname : `${pathname}?${url.cleanSearch}`);
  }, [url, router, pathname]);

  const navigate = useCallback(
    (patch: FlowSearchPatch) => {
      setNotices([]);
      router.push(`${pathname}${buildFlowSearch(searchParams, patch)}`);
    },
    [router, pathname, searchParams],
  );
  return {
    url,
    notices,
    selectWindow: useCallback((window: FlowWindow) => navigate({ window }), [navigate]),
    selectCompare: useCallback((compare: FlowCompare) => navigate({ compare }), [navigate]),
    selectDrill: useCallback((drill: string) => navigate({ drill }), [navigate]),
    closeDrill: useCallback(() => navigate({ drill: null }), [navigate]),
  };
}

/** Which collapsible sections are open, and the jump list's "open, scroll, focus the h2". */
function useSectionNav() {
  const [openSections, setOpenSections] = useState<ReadonlySet<FlowSectionId>>(new Set());
  const [pendingFocus, setPendingFocus] = useState<FlowSectionId | null>(null);

  const jumpTo = useCallback((id: FlowSectionId) => {
    setOpenSections((open) => new Set(open).add(id));
    setPendingFocus(id);
  }, []);
  const toggleSection = useCallback((id: FlowSectionId) => {
    setOpenSections((open) => {
      const next = new Set(open);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  }, []);

  // Focus after the render that opened the section, when its heading is in the DOM.
  useEffect(() => {
    if (pendingFocus === null) return;
    const heading = document.getElementById(pendingFocus);
    heading?.focus();
    heading?.scrollIntoView?.({ block: "start" });
    setPendingFocus(null);
  }, [pendingFocus]);

  return { openSections, jumpTo, toggleSection };
}

interface TimelineTarget {
  itemId: string;
  invoker: HTMLElement;
}

export function FlowPage() {
  const { announce } = useAnnounce();
  const now = useNow(CLOCK_TICK_MS);
  const { url, notices, selectWindow, selectCompare, selectDrill, closeDrill } = useFlowUrl();
  // The drawer is open from the URL alone; the clicked number is remembered only to return focus to it.
  const drillInvoker = useRef<HTMLElement | null>(null);
  const openDrill = useCallback(
    (key: string) => {
      drillInvoker.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      selectDrill(key);
    },
    [selectDrill],
  );
  const { openSections, jumpTo, toggleSection } = useSectionNav();
  const [timeline, setTimeline] = useState<TimelineTarget | null>(null);
  const viewEvents = useCallback((itemId: string, invoker: HTMLElement) => setTimeline({ itemId, invoker }), []);
  const closeTimeline = useCallback(() => setTimeline(null), []);

  const onSettled = useCallback(
    ({ kind, ok, asOf }: FlowSettled) => {
      // Background polls stay silent; the as-of header and stale banner carry their outcome.
      if (!ok || kind === "poll") return;
      announce(`Flow updated, as of ${formatClock(asOf ?? new Date())}`, "polite", "flow-updated");
    },
    [announce],
  );
  const flow = useBacklogFlow({ window: url.window, compare: url.compare === "prev", onSettled });
  const { summary, refresh } = flow;
  const reload = useCallback(() => void refresh(), [refresh]);

  const asOf = summary?.meta?.asOf ? timestampDate(summary.meta.asOf) : null;
  const unavailableSections = summary ? FLOW_SECTIONS.filter((def) => sectionStatus(summary, def)?.status === "failed").length : 0;

  return (
    <div className={styles.page}>
      <BacklogViewTabs active="flow" />
      <div className={styles.scroller}>
        <FlowHeader
          window={url.window}
          compare={url.compare}
          onWindowChange={selectWindow}
          onCompareChange={selectCompare}
          asOf={asOf}
          now={now}
          loading={flow.phase === "loading"}
          isRefreshing={flow.isRefreshing}
          onRefresh={reload}
          slowPath={summary?.meta?.slowPath ?? false}
          estimatedCount={summary?.meta?.estimated.length ?? 0}
          unavailableSections={unavailableSections}
          onJump={jumpTo}
        />
        {notices.map((notice) => (
          <p key={notice} className={styles.notice}>
            {NOTICE_TEXT[notice]}
          </p>
        ))}
        {flow.compareError && (
          <p className={styles.notice} role="status" data-testid="flow-compare-error">
            Previous-period comparison unavailable: {flow.compareError}
          </p>
        )}
        {flow.isStale && <StaleBanner since={asOf ?? new Date(flow.lastSuccessAt ?? now)} onRetry={reload} />}
        {flow.phase === "ready" && (
          <ThrashBanner anomalies={summary?.anomalies} archivedIds={NO_ARCHIVED_IDS} onView={() => jumpTo("anomalies")} onReload={reload} />
        )}
        {flow.phase === "error" && <HardError message={flow.error ?? "Unknown error"} onRetry={reload} />}
        {flow.phase === "too-large" && <WindowTooLarge window={url.window} tooLarge={flow.tooLarge} onPick={selectWindow} />}
        {(flow.phase === "loading" || flow.phase === "ready") && (
          <div className={`${styles.sections}${flow.isStale ? ` ${styles.dimmed}` : ""}`} data-stale={flow.isStale || undefined}>
            {FLOW_SECTIONS.map((def) => {
              const status = summary ? sectionStatus(summary, def) : undefined;
              const empty = def.id === "tiles" && status?.status === "empty";
              return (
                <FlowSection
                  key={def.id}
                  def={def}
                  status={status}
                  loading={summary === null}
                  open={openSections.has(def.id) || needsAttention(status)}
                  onToggle={() => toggleSection(def.id)}
                  summaryLine={summary ? sectionSummaryLine(def.id, summary) : null}
                  onReload={reload}
                >
                  {empty && <EmptyWindow window={url.window} onWiden={selectWindow} />}
                  {summary && !empty && <FlowSectionBody id={def.id} summary={summary} comparison={flow.comparison} onDrill={openDrill} onViewEvents={viewEvents} />}
                </FlowSection>
              );
            })}
          </div>
        )}
      </div>
      {url.drill !== null && <FlowDrilldownDrawer drillKey={url.drill} window={url.window} returnFocusTo={drillInvoker.current} onClose={closeDrill} />}
      {timeline && <FlowItemTimeline itemId={timeline.itemId} window={url.window} returnFocusTo={timeline.invoker} onClose={closeTimeline} />}
    </div>
  );
}
