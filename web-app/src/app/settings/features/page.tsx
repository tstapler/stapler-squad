// +feature: settings-features
"use client";

import { useCallback, useState } from "react";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { usePageView } from "@/lib/analytics";
import { vars } from "@/styles/theme.css";
import { StreamHubRolloutPanel } from "@/components/settings/StreamHubRolloutPanel";
import { PiDisableWarningDialog } from "@/components/settings/PiDisableWarningDialog";
import { PI_SUPPORT_FLAG_NAME } from "@/lib/constants/programs";
import {
  container,
  title,
  subtitle,
  flagRow,
  flagInfo,
  flagName,
  flagDescription,
  toggle,
  toggleThumb,
  badge,
  badgeEnabled,
  badgeDisabled,
  errorMessage,
  emptyMessage,
} from "./page.css";

const FEATURE_META: Record<string, { label: string }> = {
  backlog: { label: "Backlog" },
  "pi-support": { label: "pi coding agent" },
  "backlog:sdd-default-pipeline": { label: "Backlog: default new items to SDD pipeline" },
  "review:block-approval-on-ci-failure": { label: "Block Approve when CI is failing" },
  "terminal:resync-visibility-scope": { label: "Terminal resync: scope to visible terminal" },
  "terminal:resync-correlation-id": { label: "Terminal resync: correlation IDs" },
  "terminal:resync-skip-stale-dimension-slowpath": {
    label: "Terminal resync: skip slow path for backgrounded terminals",
  },
  "terminal:resync-exec-gate-fast-lane": { label: "Terminal resync: exec-gate fast lane" },
  "terminal:resync-stagger": { label: "Terminal resync: stagger bursts" },
  "terminal:resync-compression": { label: "Terminal resync: wire compression" },
  "terminal:resync-batching": { label: "Terminal resync: batch requests" },
};

export default function FeaturesPage() {
  usePageView();
  const { flagList, isLoading, error, setFlag } = useFeatureFlags();
  const [pendingPiDisable, setPendingPiDisable] = useState(false);

  // Story 2.1.2: toggling pi-support off is gated on a mandatory warning ONLY
  // when the global pi approval extension is actually installed on disk —
  // otherwise this behaves exactly like toggling any other flag. The check
  // runs server-side (~/.pi is on the server's filesystem, not the browser's)
  // via GET /api/pi-extension-status.
  const handleToggle = useCallback(
    async (name: string, currentEnabled: boolean) => {
      const disablingPiSupport = name === PI_SUPPORT_FLAG_NAME && currentEnabled;
      if (!disablingPiSupport) {
        setFlag(name, !currentEnabled);
        return;
      }
      try {
        const res = await fetch("/api/pi-extension-status");
        if (!res.ok) {
          // Fail closed: a non-2xx response (e.g. a 500) means we can't trust
          // the body's `installed` field — show the warning rather than
          // silently falling through to a disable. Matches ADR-003's
          // fail-closed posture elsewhere in this feature.
          setPendingPiDisable(true);
          return;
        }
        const data: { installed?: boolean } = await res.json();
        if (data.installed) {
          setPendingPiDisable(true);
          return;
        }
      } catch {
        // Fail closed: if the status check itself fails, show the warning
        // rather than silently disabling — matches this feature's fail-closed
        // posture elsewhere (ADR-003) rather than assuming "not installed".
        setPendingPiDisable(true);
        return;
      }
      setFlag(name, false);
    },
    [setFlag],
  );

  const confirmPiDisable = useCallback(() => {
    setPendingPiDisable(false);
    setFlag(PI_SUPPORT_FLAG_NAME, false);
  }, [setFlag]);

  const cancelPiDisable = useCallback(() => {
    setPendingPiDisable(false);
  }, []);

  return (
    <main id="main-content" className={container}>
      <h1 className={title}>Feature Flags</h1>
      <p className={subtitle}>
        Toggle experimental or optional features. Changes take effect immediately — no restart needed.
      </p>

      {error && (
        <p className={errorMessage} role="alert">{error}. Please refresh.</p>
      )}

      {isLoading ? (
        <p className={flagDescription}>Loading…</p>
      ) : !error && flagList.length === 0 ? (
        <p className={emptyMessage}>No feature flags configured.</p>
      ) : (
        flagList.map(({ name, enabled, description, statusDetail }) => {
          const meta = FEATURE_META[name];
          const label = meta?.label ?? name;
          return (
            <div key={name} className={flagRow} data-testid="feature-flag-row">
              <div className={flagInfo}>
                <div className={flagName} data-testid="feature-flag-name">
                  {label}
                  <span
                    className={`${badge} ${enabled ? badgeEnabled : badgeDisabled}`}
                    data-testid="feature-flag-status"
                  >
                    {enabled ? "On" : "Off"}
                  </span>
                </div>
                {description && (
                  <div className={flagDescription}>{description}</div>
                )}
                {statusDetail && (
                  <div className={flagDescription}>{statusDetail}</div>
                )}
              </div>
              <button
                className={toggle}
                style={{
                  background: enabled ? vars.color.primary : vars.color.borderColor,
                }}
                aria-label={`${enabled ? "Disable" : "Enable"} ${label}`}
                aria-pressed={enabled}
                onClick={() => handleToggle(name, enabled)}
              >
                <span
                  className={toggleThumb}
                  style={{ left: enabled ? "1.375rem" : "0.1875rem" }}
                />
              </button>
            </div>
          );
        })
      )}

      <StreamHubRolloutPanel />

      {pendingPiDisable && (
        <PiDisableWarningDialog onAcknowledge={confirmPiDisable} onCancel={cancelPiDisable} />
      )}
    </main>
  );
}
