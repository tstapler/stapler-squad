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
        flagList.map(({ name, enabled, description }) => {
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
