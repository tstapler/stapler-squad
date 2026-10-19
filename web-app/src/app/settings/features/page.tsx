// +feature: settings-features
"use client";

import { useCallback, useId, useRef, useState } from "react";
import { useFeatureFlags, type FeatureFlagMeta } from "@/lib/contexts/FeatureFlagsContext";
import { usePageView } from "@/lib/analytics";
import { StreamHubRolloutPanel } from "@/components/settings/StreamHubRolloutPanel";
import { TymuxRolloutPanel } from "@/components/settings/TymuxRolloutPanel";
import { FEATURE_META, FLAG_EXTRAS, layoutFlags, type FlagSetter } from "./flagExtras";
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
  onToggle: (flag: FeatureFlagMeta) => void;
  setFlag: FlagSetter;
}

function FlagRow({ flag, nested, status, onToggle, setFlag }: FlagRowProps) {
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
    <div className={`${flagRow} ${nested ? nestedRow : ""}`} data-testid="feature-flag-row">
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
        {extra?.renderExtras?.({ flag, pending, setFlag })}
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
      <button
        type="button"
        role="switch"
        className={toggle}
        aria-checked={enabled}
        aria-label={label}
        aria-describedby={describedBy || undefined}
        aria-disabled={pending || undefined}
        data-testid="feature-flag-switch"
        onClick={() => onToggle(flag)}
      >
        <span className={toggleTrack}>
          <span className={toggleThumb} />
        </span>
      </button>
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

export default function FeaturesPage() {
  usePageView();
  const { flagList, isLoading, loadError, reload, setFlag } = useFeatureFlags();
  const [rowStatus, setRowStatus] = useState<Record<string, RowPhase | undefined>>({});
  const [confirming, setConfirming] = useState<string | null>(null);
  // A ref, not state: two activations in one tick must not both pass the guard.
  const inFlight = useRef(new Set<string>());

  const setStatus = useCallback((name: string, phase: RowPhase | undefined) => {
    setRowStatus((prev) => ({ ...prev, [name]: phase }));
  }, []);

  const apply = useCallback(
    async (flag: FeatureFlagMeta, next: boolean) => {
      const label = FEATURE_META[flag.name]?.label ?? flag.name;
      const change = FLAG_EXTRAS[flag.name]?.toggleChange?.(next) ?? next;
      const result = await setFlag(flag.name, change);
      inFlight.current.delete(flag.name);
      if (result && !result.ok) {
        setStatus(flag.name, { phase: "failed", error: result.error });
      } else {
        setStatus(flag.name, { phase: "done", message: `${label} is now ${next ? "On" : "Off"}.` });
      }
    },
    [setFlag, setStatus],
  );

  // The displayed value is always the server's read-back, so a retry flips from it.
  const handleToggle = useCallback(
    async (flag: FeatureFlagMeta) => {
      if (inFlight.current.has(flag.name)) return;
      inFlight.current.add(flag.name);
      setStatus(flag.name, { phase: "saving" });
      const next = !flag.enabled;
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

  const confirmingFlag = confirming ? flagList.find((f) => f.name === confirming) : undefined;
  const ConfirmDialog = confirming ? FLAG_EXTRAS[confirming]?.confirmDisable?.Dialog : undefined;

  const renderRow = (flag: FeatureFlagMeta, nested = false) => (
    <FlagRow
      key={flag.name}
      flag={flag}
      nested={nested}
      status={rowStatus[flag.name]}
      onToggle={handleToggle}
      setFlag={setFlag}
    />
  );

  const renderList = () =>
    layoutFlags(flagList).map((item) => {
      if (item.kind === "flag") {
        return [renderRow(item.flag), ...item.children.map((c) => renderRow(c, true))];
      }
      return (
        <section key={item.heading} className={group} aria-label={item.heading} data-testid="feature-flag-group">
          <h2 className={groupHeading}>{item.heading}</h2>
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
