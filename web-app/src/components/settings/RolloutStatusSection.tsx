"use client";

import * as styles from "./StreamHubRolloutPanel.css";

export interface RolloutSessionOverride {
  sessionName: string;
  forced: boolean;
}

export interface RolloutStatusSectionProps {
  /** e.g. "stream-hub" or "tymux" — used to build every data-testid below. */
  testIdPrefix: string;
  /** e.g. "stream hub" or "tymux" — used in the global-override aria-labels. */
  subjectLabel: string;
  /** Label shown when no global override is set, e.g. "Not set (default: on)". */
  notSetLabel: string;
  error: string | null;
  globalOverride: boolean | undefined;
  onSetGlobalOverride: (forced: boolean | undefined) => void;
  busy: boolean;
  rehearsalCompletedAt: Date | null;
  onCompleteRehearsal: () => void;
  overrides: RolloutSessionOverride[];
  onRemoveOverride: (sessionName: string) => void;
}

/**
 * Shared status/controls block for the stream-hub and tymux rollout panels:
 * error banner, global-override toggle, rollback-rehearsal status, and the
 * per-session override list. Both panels are shaped identically here — they
 * differ only in wording, testid prefix, and which RPC client backs the
 * callbacks — so this is the deduplicated common body; each panel keeps its
 * own heading/description and the add-override input, which do differ.
 */
export function RolloutStatusSection({
  testIdPrefix,
  subjectLabel,
  notSetLabel,
  error,
  globalOverride,
  onSetGlobalOverride,
  busy,
  rehearsalCompletedAt,
  onCompleteRehearsal,
  overrides,
  onRemoveOverride,
}: RolloutStatusSectionProps) {
  return (
    <>
      {error && <p className={styles.errorMessage} role="alert">{error}</p>}

      <div className={styles.statusRow}>
        <span className={styles.statusLabel}>Global override</span>
        <span className={`${styles.badge} ${globalOverride === true ? styles.badgeEnabled : styles.badgeDisabled}`}>
          {globalOverride === undefined ? notSetLabel : globalOverride ? "Forced on" : "Forced off"}
        </span>
      </div>
      <div className={styles.addRow}>
        <button
          className={styles.actionButton}
          disabled={busy || globalOverride === true}
          onClick={() => onSetGlobalOverride(true)}
          data-testid={`${testIdPrefix}-global-override-on`}
          aria-label={`Force ${subjectLabel} on for all sessions`}
        >
          Force on for everything
        </button>
        <button
          className={styles.actionButton}
          disabled={busy || globalOverride === false}
          onClick={() => onSetGlobalOverride(false)}
          data-testid={`${testIdPrefix}-global-override-off`}
          aria-label={`Force ${subjectLabel} off for all sessions`}
        >
          Force off for everything
        </button>
        <button
          className={styles.removeButton}
          disabled={busy || globalOverride === undefined}
          onClick={() => onSetGlobalOverride(undefined)}
          data-testid={`${testIdPrefix}-global-override-clear`}
          aria-label="Clear global override, revert to the default"
        >
          Clear override
        </button>
      </div>

      <div className={styles.statusRow}>
        <span className={styles.statusLabel}>Rollback rehearsal</span>
        {rehearsalCompletedAt ? (
          <span className={`${styles.badge} ${styles.badgeEnabled}`}>
            Completed {rehearsalCompletedAt.toLocaleString()}
          </span>
        ) : (
          <button
            className={styles.actionButton}
            data-testid={`${testIdPrefix}-complete-rehearsal`}
            disabled={busy}
            onClick={onCompleteRehearsal}
          >
            Mark rehearsal complete
          </button>
        )}
      </div>
    </>
  );
}

export interface RolloutOverrideListProps {
  testIdPrefix: string;
  overrides: RolloutSessionOverride[];
  busy: boolean;
  onRemoveOverride: (sessionName: string) => void;
}

/** Per-session canary override list shared by both rollout panels. */
export function RolloutOverrideList({
  testIdPrefix,
  overrides,
  busy,
  onRemoveOverride,
}: RolloutOverrideListProps) {
  return (
    <>
      <h3 className={styles.subheading}>Per-session canary overrides</h3>
      {overrides.length === 0 ? (
        <p className={styles.description}>No sessions are currently overridden.</p>
      ) : (
        <ul className={styles.overrideList}>
          {overrides.map((o) => (
            <li key={o.sessionName} className={styles.overrideRow} data-testid={`${testIdPrefix}-override-row`}>
              <span className={styles.statusLabel}>{o.sessionName}</span>
              <span className={`${styles.badge} ${o.forced ? styles.badgeEnabled : styles.badgeDisabled}`}>
                {o.forced ? "Forced on" : "Forced off"}
              </span>
              <button
                className={styles.removeButton}
                data-testid={`${testIdPrefix}-remove-override`}
                disabled={busy}
                onClick={() => onRemoveOverride(o.sessionName)}
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
