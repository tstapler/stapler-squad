// +feature: settings-features
"use client";

import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { usePageView } from "@/lib/analytics";
import { vars } from "@/styles/theme.css";
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
            <div key={name} className={flagRow}>
              <div className={flagInfo}>
                <div className={flagName}>
                  {label}
                  <span className={`${badge} ${enabled ? badgeEnabled : badgeDisabled}`}>
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
                onClick={() => setFlag(name, !enabled)}
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
    </main>
  );
}
