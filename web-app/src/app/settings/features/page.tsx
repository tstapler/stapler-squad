// +feature: settings-features
"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { useFeatureFlags, type FeatureFlagMeta } from "@/lib/contexts/FeatureFlagsContext";
import { usePageView } from "@/lib/analytics";
import { StreamHubRolloutPanel } from "@/components/settings/StreamHubRolloutPanel";
import { TymuxRolloutPanel } from "@/components/settings/TymuxRolloutPanel";
import { useEngineModeStatus } from "@/lib/hooks/useEngineModeStatus";
import {
  FEATURE_META,
  FLAG_EXTRAS,
  GROUP_HEADERS,
  enforcedCountAfter,
  flagRowId,
  isEngineEnforceFlag,
  isEngineModeFlag,
  layoutFlags,
  type FlagSetter,
} from "./flagExtras";
import { EnforceDialog } from "./EnforceReadinessEvidence";
import { useHashRowFocus } from "./useHashRowFocus";
import {
  container,
  title,
  subtitle,
  flagRow,
  flagInfo,
  flagName,
  flagDescription,
  toggle,
  toggleTrack,
  toggleThumb,
  badge,
  badgeEnabled,
  badgeDisabled,
  badgeInert,
  rowStatus,
  rowError,
  retryButton,
  group,
  groupHeading,
  nestedRow,
  skeletonRow,
  errorMessage,
  emptyMessage,
  hashTargetRow,
} from "./page.css";

type RowPhase = { phase: "saving" } | { phase: "done"; message: string } | { phase: "failed"; error: string };

const INACTIVE_PREFIX = "inactive:";
const SOURCE_PREFIXES = ["Default:", "Set explicitly:"];

/** Splits the server's "; "-joined status line into the inert marker, the value source and the rest. */
function parseStatusDetail(statusDetail: string) {
  const parts = statusDetail ? statusDetail.split("; ") : [];
  const inert = parts.some((p) => p.startsWith(INACTIVE_PREFIX));
  const source = parts.find((p) => SOURCE_PREFIXES.some((s) => p.startsWith(s)));
  const rest = parts.filter((p) => !p.startsWith(INACTIVE_PREFIX) && p !== source).join("; ");
  return { inert, source, rest };
}

interface FlagRowProps {
  flag: FeatureFlagMeta;
  nested?: boolean;
  status: RowPhase | undefined;
  /** `trigger` is the switch that was activated, when there is one (focus returns to it). */
  onToggle: (flag: FeatureFlagMeta, trigger?: HTMLElement) => void;
  onRevert: (flag: FeatureFlagMeta) => void;
  setFlag: FlagSetter;
}

/** Window in which a focus/visibility wake-up re-reads the list at most once. */
const FLAG_REFETCH_WINDOW_MS = 5000;

function FlagRow({ flag, nested, status, onToggle, onRevert, setFlag }: FlagRowProps) {
  const { name, enabled, description, statusDetail } = flag;
  const label = FEATURE_META[name]?.label ?? name;
  const extra = FLAG_EXTRAS[name];
  const { inert, source, rest } = parseStatusDetail(statusDetail);
  const pending = status?.phase === "saving";
  const ids = useId();
  const describedBy = [
    description && `${ids}-desc`,
    inert && `${ids}-inert`,
    source && `${ids}-source`,
  ].filter(Boolean).join(" ");

  const badgeClass = enabled ? (inert ? badgeInert : badgeEnabled) : badgeDisabled;
  const badgeText = enabled ? (inert ? "On (inactive)" : "On") : "Off";

  return (
    <div
      id={flagRowId(name)}
      tabIndex={-1}
      className={`${flagRow} ${hashTargetRow} ${nested ? nestedRow : ""}`}
      data-testid="feature-flag-row"
    >
      {/* First in DOM so Tab runs switch, Revert, Edit stages; CSS order keeps it on the right. */}
      <button
        type="button"
        role="switch"
        className={toggle}
        aria-checked={enabled}
        aria-label={label}
        aria-describedby={describedBy || undefined}
        aria-disabled={pending || undefined}
        data-testid="feature-flag-switch"
        onClick={(e) => onToggle(flag, e.currentTarget)}
      >
        <span className={toggleTrack}>
          <span className={toggleThumb} />
        </span>
      </button>
      <div className={flagInfo}>
        <div className={flagName} data-testid="feature-flag-name">
          {label}
          <span className={`${badge} ${badgeClass}`} data-testid="feature-flag-status">
            {badgeText}
          </span>
        </div>
        {description && (
          <div id={`${ids}-desc`} className={flagDescription}>{description}</div>
        )}
        {inert && (
          <div id={`${ids}-inert`} className={flagDescription} data-testid="feature-flag-inert-note">
            {extra?.inertNote ?? "Has no effect until a prerequisite flag is On."}
          </div>
        )}
        {extra?.liveNote && (
          <div className={flagDescription} data-testid="flag-live-note">{extra.liveNote}</div>
        )}
        {rest && <div className={flagDescription}>{rest}</div>}
        {source && (
          <div id={`${ids}-source`} className={flagDescription} data-testid="feature-flag-source">{source}</div>
        )}
        {extra?.renderExtras?.({ flag, pending, setFlag, revert: () => onRevert(flag) })}
        <div className={rowStatus} role="status" data-testid="feature-flag-row-status">
          {status?.phase === "saving" && `Saving ${label}...`}
          {status?.phase === "done" && status.message}
        </div>
        {status?.phase === "failed" && (
          <div className={rowError} role="alert" data-testid="feature-flag-row-error">
            <span>{`Couldn't change ${label}: ${status.error}`}</span>
            <button type="button" className={retryButton} onClick={() => onToggle(flag)}>
              Retry
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function FlagListSkeleton() {
  return (
    <div aria-busy="true" data-testid="feature-flags-loading">
      <p className={flagDescription} role="status">Loading feature flags…</p>
      {[0, 1, 2].map((i) => (
        <div key={i} className={skeletonRow} />
      ))}
    </div>
  );
}

const ENGINE_MODE_SETTLE_MS = 1500;

export default function FeaturesPage() {
  usePageView();
  const { flagList, isLoading, loadError, reload, refresh, setFlag } = useFeatureFlags();
  const { view: engineView, refetch: refetchEngine } = useEngineModeStatus();
  const [rowStatus, setRowStatus] = useState<Record<string, RowPhase | undefined>>({});
  const [confirming, setConfirming] = useState<string | null>(null);
  const [enforcing, setEnforcing] = useState<string | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const flagListRef = useRef(flagList);
  flagListRef.current = flagList;
  // A ref, not state: two activations in one tick must not both pass the guard.
  const inFlight = useRef(new Set<string>());

  const setStatus = useCallback((name: string, phase: RowPhase | undefined) => {
    setRowStatus((prev) => ({ ...prev, [name]: phase }));
  }, []);

  // The server's mode reader caches about 1s, so the first re-read can still show the old mode.
  const settleTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (settleTimer.current) clearTimeout(settleTimer.current);
  }, []);

  const apply = useCallback(
    async (flag: FeatureFlagMeta, next: boolean) => {
      const label = FEATURE_META[flag.name]?.label ?? flag.name;
      const change = FLAG_EXTRAS[flag.name]?.toggleChange?.(next) ?? next;
      const result = await setFlag(flag.name, change);
      inFlight.current.delete(flag.name);
      if (result && !result.ok) {
        setStatus(flag.name, { phase: "failed", error: result.error });
        return;
      }
      const isEngine = isEngineEnforceFlag(flag.name);
      // The row's confirmation carries the new count: the summary line is static (live-region policy).
      const count = isEngine
        ? ` ${enforcedCountAfter(flagListRef.current, flag.name, next)} of 3 caller classes enforced.`
        : "";
      setStatus(flag.name, { phase: "done", message: `${label} is now ${next ? "On" : "Off"}.${count}` });
      if (isEngineModeFlag(flag.name)) {
        void refetchEngine();
        if (settleTimer.current) clearTimeout(settleTimer.current);
        settleTimer.current = setTimeout(() => void refetchEngine(), ENGINE_MODE_SETTLE_MS);
      }
    },
    [setFlag, setStatus, refetchEngine],
  );

  // The displayed value is always the server's read-back, so a retry flips from it.
  const handleToggle = useCallback(
    async (flag: FeatureFlagMeta, trigger?: HTMLElement) => {
      if (inFlight.current.has(flag.name)) return;
      const next = !flag.enabled;
      if (next && FLAG_EXTRAS[flag.name]?.enforce) {
        // Safari and Firefox on macOS do not focus a clicked button, so name the return target.
        trigger?.focus();
        returnFocusRef.current = trigger ?? null;
        setStatus(flag.name, undefined);
        setEnforcing(flag.name);
        return;
      }
      inFlight.current.add(flag.name);
      setStatus(flag.name, { phase: "saving" });
      const confirmDisable = FLAG_EXTRAS[flag.name]?.confirmDisable;
      if (!next && confirmDisable && (await confirmDisable.needsConfirm())) {
        inFlight.current.delete(flag.name);
        setStatus(flag.name, undefined);
        setConfirming(flag.name);
        return;
      }
      await apply(flag, next);
    },
    [apply, setStatus],
  );

  const handleRevert = useCallback(
    (flag: FeatureFlagMeta) => {
      if (inFlight.current.has(flag.name)) return;
      inFlight.current.add(flag.name);
      setStatus(flag.name, { phase: "saving" });
      void apply(flag, false);
    },
    [apply, setStatus],
  );

  const listState = isLoading ? "loading" : loadError ? "failed" : "loaded";
  useHashRowFocus(listState);

  // A flag flipped in another tab or device shows up when this tab is looked at again (UX-36).
  const lastRefetchRef = useRef(0);
  useEffect(() => {
    if (!refresh) return;
    const onWake = () => {
      if (document.visibilityState === "hidden") return;
      if (Date.now() - lastRefetchRef.current < FLAG_REFETCH_WINDOW_MS) return;
      lastRefetchRef.current = Date.now();
      void refresh({ quiet: true });
    };
    lastRefetchRef.current = Date.now();
    document.addEventListener("visibilitychange", onWake);
    window.addEventListener("focus", onWake);
    return () => {
      document.removeEventListener("visibilitychange", onWake);
      window.removeEventListener("focus", onWake);
    };
  }, [refresh]);

  const enforcingFlag = enforcing ? flagList.find((f) => f.name === enforcing) : undefined;
  const enforcingExtra = enforcing ? FLAG_EXTRAS[enforcing]?.enforce : undefined;
  const confirmingFlag = confirming ? flagList.find((f) => f.name === confirming) : undefined;
  const ConfirmDialog = confirming ? FLAG_EXTRAS[confirming]?.confirmDisable?.Dialog : undefined;

  const orderAdvisory = (predecessor: { flag: string; advisory: string } | undefined) =>
    predecessor && !flagList.find((f) => f.name === predecessor.flag)?.enabled ? predecessor.advisory : undefined;

  const renderRow = (flag: FeatureFlagMeta, nested = false) => (
    <FlagRow
      key={flag.name}
      flag={flag}
      nested={nested}
      status={rowStatus[flag.name]}
      onToggle={handleToggle}
      onRevert={handleRevert}
      setFlag={setFlag}
    />
  );

  const renderGroupHeader = (heading: string) => {
    const Header = GROUP_HEADERS[heading];
    return Header ? <Header view={engineView} flagList={flagList} /> : null;
  };

  const renderList = () =>
    layoutFlags(flagList).map((item) => {
      if (item.kind === "flag") {
        return [renderRow(item.flag), ...item.children.map((c) => renderRow(c, true))];
      }
      return (
        <section key={item.heading} className={group} aria-label={item.heading} data-testid="feature-flag-group">
          <h2 className={groupHeading}>{item.heading}</h2>
          {renderGroupHeader(item.heading)}
          {item.members.flatMap((m) => [renderRow(m.flag), ...m.children.map((c) => renderRow(c, true))])}
        </section>
      );
    });

  return (
    <main id="main-content" className={container}>
      <h1 className={title}>Feature Flags</h1>
      <p className={subtitle}>
        Toggle experimental or optional features. Changes take effect immediately — no restart needed.
      </p>

      {isLoading ? (
        <FlagListSkeleton />
      ) : loadError ? (
        <div className={errorMessage} role="alert" data-testid="feature-flags-load-error">
          <span>{`Couldn't load feature flags: ${loadError}`}</span>{" "}
          <button type="button" className={retryButton} onClick={() => void reload()}>
            Retry
          </button>
        </div>
      ) : flagList.length === 0 ? (
        <p className={emptyMessage}>No feature flags configured.</p>
      ) : (
        renderList()
      )}

      <StreamHubRolloutPanel />
      <TymuxRolloutPanel />

      {enforcingFlag && enforcingExtra && (
        <EnforceDialog
          cls={enforcingExtra.cls}
          flagLabel={FEATURE_META[enforcingFlag.name]?.label ?? enforcingFlag.name}
          classNote={enforcingExtra.classNote}
          orderAdvisory={orderAdvisory(enforcingExtra.predecessor)}
          returnFocusRef={returnFocusRef}
          onCancel={() => setEnforcing(null)}
          onConfirm={() => {
            setEnforcing(null);
            inFlight.current.add(enforcingFlag.name);
            setStatus(enforcingFlag.name, { phase: "saving" });
            void apply(enforcingFlag, true);
          }}
        />
      )}

      {confirmingFlag && ConfirmDialog && (
        <ConfirmDialog
          onAcknowledge={() => {
            setConfirming(null);
            inFlight.current.add(confirmingFlag.name);
            setStatus(confirmingFlag.name, { phase: "saving" });
            void apply(confirmingFlag, false);
          }}
          onCancel={() => setConfirming(null)}
        />
      )}
    </main>
  );
}
