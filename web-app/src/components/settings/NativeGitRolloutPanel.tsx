"use client";
// +feature: native-git-rollout

import { useState, useEffect, useCallback, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import {
  NativeGitRolloutService,
  type NativeGitRolloutStatus,
  type NativeWorktreeSessionOverrideEntry,
  type NativeMergeWorktreeOverrideEntry,
} from "@/gen/session/v1/native_git_rollout_pb";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useAnalytics } from "@/lib/analytics";
import { vars } from "@/styles/theme.css";
// ponytail: reuses StreamHubRolloutPanel's stylesheet — the class names
// (panel, heading, statusRow, ...) are generic, not stream-hub-specific, and
// this panel's markup is deliberately shaped the same way.
import * as styles from "./StreamHubRolloutPanel.css";

/** `"unknown"` is a distinct load-failure state — see Task 4.3.1d — never
 * conflated with `undefined` (explicit "not set, default off"). */
type GlobalOverrideState = boolean | undefined | "unknown";

interface NormalizedOverride {
  key: string;
  forced: boolean;
}

interface SectionAriaLabels {
  forceOn: string;
  forceOff: string;
  clear: string;
  add: string;
  remove: (key: string) => string;
}

// Full-sentence, section-disambiguating labels (Task 4.3.1f / design/ux.md
// §3 AC10) — the visible button text is identical between sections
// ("Force on for everything", etc.), so the accessible name is the only
// thing telling a screen-reader user which section they're in.
const WORKTREE_ARIA_LABELS: SectionAriaLabels = {
  forceOn: "Force native worktree management on for all sessions",
  forceOff: "Force native worktree management off for all sessions",
  clear: "Clear the native worktree management global override, revert to the default",
  add: "Force native worktree management on for this session",
  remove: (key) => `Remove the native worktree management override for ${key}`,
};

const MERGE_ARIA_LABELS: SectionAriaLabels = {
  forceOn: "Force native merge on for all worktree paths",
  forceOff: "Force native merge off for all worktree paths",
  clear: "Clear the native merge global override, revert to the default",
  add: "Force native merge on for this worktree path",
  remove: (key) => `Remove the native merge override for ${key}`,
};

function badgeText(state: GlobalOverrideState): string {
  if (state === "unknown") return "Unknown — reload failed";
  if (state === undefined) return "Not set (default: off)";
  return state ? "Forced on" : "Forced off";
}

function badgeClassName(state: GlobalOverrideState): string {
  return state === true ? styles.badgeEnabled : styles.badgeDisabled;
}

// Builds the exact copy from design/ux.md §1.3's "Emergency global kill…"
// row, generalized across the worktree (session-keyed) and merge
// (worktree-path-keyed) sections and across singular/plural counts.
function buildPrecedenceMessage(
  nounSingular: string,
  nounPlural: string,
  label: string,
  subject: string,
  conflictingKeys: string[],
  newGlobalValue: boolean,
): string {
  const count = conflictingKeys.length;
  const forcedWord = newGlobalValue ? "off" : "on";
  const noun = count === 1 ? nounSingular : nounPlural;
  const verb = count === 1 ? "still forces" : "still force";
  const pronoun = count === 1 ? "it" : "them";
  return (
    `${count} ${noun} ${verb} ${label} ${forcedWord} for ${subject}, which takes ` +
    `precedence over the global setting above: ${conflictingKeys.join(", ")}. ` +
    `Remove ${pronoun} below if this is part of the incident.`
  );
}

interface SectionCopy {
  sectionTitle: string;
  sectionDescription: string;
  overridesHeading: string;
  emptyOverridesText: string;
  inputPlaceholder: string;
  addButtonLabel: string;
}

interface SectionStatus {
  globalState: GlobalOverrideState;
  busy: boolean;
  error: string | null;
}

interface SectionConflict {
  keys: Set<string>;
  message: string | null;
  onDismiss: () => void;
}

interface SectionInput {
  value: string;
  onChange: (value: string) => void;
  ariaLabel: string;
  datalistOptions: string[];
}

interface SectionActions {
  onGlobalSet: (value: boolean | undefined) => void;
  onAddOverride: () => void;
  onRemoveOverride: (key: string) => void;
}

interface RolloutSectionProps {
  testidPrefix: string;
  copy: SectionCopy;
  status: SectionStatus;
  overrides: NormalizedOverride[];
  conflict: SectionConflict;
  input: SectionInput;
  ariaLabels: SectionAriaLabels;
  actions: SectionActions;
}

// Shared rendering for both sections — status row (badge + 3 buttons),
// override list (or empty-state line), and add-row — per design/ux.md §1.1's
// "one visual template, rendered twice" note. Tracking lives here (not in
// the `actions` callbacks NativeGitRolloutPanel passes in) so every click is
// tracked at the exact point of interaction, once, with no duplication.
function RolloutSection({ testidPrefix, copy, status, overrides, conflict, input, ariaLabels, actions }: RolloutSectionProps) {
  const { track } = useAnalytics();
  const datalistId = `${testidPrefix}-datalist`;
  const isUnknown = status.globalState === "unknown";
  const eventPrefix = testidPrefix.replace(/-/g, "_");

  const handleGlobalSet = (value: boolean | undefined) => {
    track({
      name: `${eventPrefix}_global_override_changed`,
      category: "user_action",
      component: "NativeGitRolloutPanel",
      labels: { forceNative: value === undefined ? "clear" : String(value) },
    });
    actions.onGlobalSet(value);
  };
  const handleAddOverride = () => {
    track({ name: `${eventPrefix}_override_added`, category: "user_action", component: "NativeGitRolloutPanel" });
    actions.onAddOverride();
  };

  return (
    <div data-testid={`${testidPrefix}-section`}>
      <h3 className={styles.subheading}>{copy.sectionTitle}</h3>
      <p className={styles.description}>{copy.sectionDescription}</p>

      {status.error && (
        <p className={styles.errorMessage} role="alert" data-testid={`${testidPrefix}-error`}>
          {status.error}
        </p>
      )}

      <div className={styles.statusRow}>
        <span className={styles.statusLabel}>Global override</span>
        <span
          className={`${styles.badge} ${badgeClassName(status.globalState)}`}
          style={isUnknown ? { background: vars.color.warningBg, color: vars.color.warningText } : undefined}
          data-testid={`${testidPrefix}-global-badge`}
        >
          {badgeText(status.globalState)}
        </span>
      </div>
      <div className={styles.addRow}>
        <button
          className={styles.actionButton}
          disabled={status.busy || isUnknown || status.globalState === true}
          onClick={() => handleGlobalSet(true)}
          data-testid={`${testidPrefix}-global-override-on`}
          aria-label={ariaLabels.forceOn}
        >
          Force on for everything
        </button>
        <button
          className={styles.actionButton}
          disabled={status.busy || isUnknown || status.globalState === false}
          onClick={() => handleGlobalSet(false)}
          data-testid={`${testidPrefix}-global-override-off`}
          aria-label={ariaLabels.forceOff}
        >
          Force off for everything
        </button>
        <button
          className={styles.removeButton}
          disabled={status.busy || isUnknown || status.globalState === undefined}
          onClick={() => handleGlobalSet(undefined)}
          data-testid={`${testidPrefix}-global-override-clear`}
          aria-label={ariaLabels.clear}
        >
          Clear override
        </button>
      </div>

      <h3 className={styles.subheading}>{copy.overridesHeading}</h3>
      {overrides.length === 0 ? (
        <p className={styles.description}>{copy.emptyOverridesText}</p>
      ) : (
        <ul className={styles.overrideList}>
          {overrides.map((o) => {
            const conflicted = conflict.keys.has(o.key);
            return (
              <li
                key={o.key}
                className={styles.overrideRow}
                data-testid={`${testidPrefix}-override-row`}
                style={conflicted ? { background: vars.color.warningBg } : undefined}
              >
                <span className={styles.statusLabel}>{o.key}</span>
                <span className={`${styles.badge} ${o.forced ? styles.badgeEnabled : styles.badgeDisabled}`}>
                  {o.forced ? "Forced on" : "Forced off"}
                </span>
                <button
                  className={styles.removeButton}
                  data-testid={`${testidPrefix}-remove-override`}
                  disabled={status.busy}
                  onClick={() => {
                    // Inlined (not handleRemoveOverride) so the analytics-lint
                    // check, which only looks within this element's own
                    // enclosing function (the .map() callback), can see it.
                    track({ name: `${eventPrefix}_override_removed`, category: "user_action", component: "NativeGitRolloutPanel" });
                    actions.onRemoveOverride(o.key);
                  }}
                  aria-label={ariaLabels.remove(o.key)}
                  style={conflicted ? { borderColor: vars.color.warning, borderWidth: "2px" } : undefined}
                >
                  Remove
                </button>
              </li>
            );
          })}
        </ul>
      )}

      {conflict.message && (
        <p
          className={styles.hint}
          role="status"
          data-testid={`${testidPrefix}-precedence-note`}
          style={{ color: vars.color.warningText }}
        >
          {conflict.message}{" "}
          <button
            className={styles.removeButton}
            onClick={conflict.onDismiss}
            data-testid={`${testidPrefix}-precedence-dismiss`}
            aria-label="Dismiss this precedence note"
          >
            Dismiss
          </button>
        </p>
      )}

      <div className={styles.addRow}>
        <input
          className={styles.input}
          list={datalistId}
          placeholder={copy.inputPlaceholder}
          aria-label={input.ariaLabel}
          value={input.value}
          onChange={(e) => input.onChange(e.target.value)}
          data-testid={`${testidPrefix}-override-input`}
        />
        <datalist id={datalistId}>
          {input.datalistOptions.map((opt) => (
            <option key={opt} value={opt} />
          ))}
        </datalist>
        <button
          className={styles.actionButton}
          disabled={status.busy || !input.value.trim()}
          onClick={handleAddOverride}
          data-testid={`${testidPrefix}-add-override`}
          aria-label={ariaLabels.add}
        >
          {copy.addButtonLabel}
        </button>
      </div>
    </div>
  );
}

/**
 * Operator controls for the go-git worktree/merge rollout: two independent
 * feature flags (native worktree management, native merge), each with a
 * live global override plus per-session/per-worktree-path canary overrides.
 * Both flags default off (corruption-blast-radius code, unlike stream_hub's
 * default-on) — config.json-backed, no process restart required. Mirrors
 * StreamHubRolloutPanel/TymuxRolloutPanel's shape, consolidated into one
 * component since both flags share one NativeGitRolloutService RPC surface
 * (ADR-002).
 */
export function NativeGitRolloutPanel() {
  const { track } = useAnalytics();
  const transport = useMemo(() => getConnectTransport(), []);
  const client = useMemo(() => createClient(NativeGitRolloutService, transport), [transport]);
  const sessionClient = useMemo(() => createClient(SessionService, transport), [transport]);

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);

  const [worktreeGlobalOverride, setWorktreeGlobalOverride] = useState<GlobalOverrideState>(undefined);
  const [worktreeOverrides, setWorktreeOverrides] = useState<NativeWorktreeSessionOverrideEntry[]>([]);
  const [worktreeError, setWorktreeError] = useState<string | null>(null);
  const [worktreeBusy, setWorktreeBusy] = useState(false);
  const [newWorktreeOverrideName, setNewWorktreeOverrideName] = useState("");
  const [worktreeNoteDismissed, setWorktreeNoteDismissed] = useState(false);

  const [mergeGlobalOverride, setMergeGlobalOverride] = useState<GlobalOverrideState>(undefined);
  const [mergeOverrides, setMergeOverrides] = useState<NativeMergeWorktreeOverrideEntry[]>([]);
  const [mergeError, setMergeError] = useState<string | null>(null);
  const [mergeBusy, setMergeBusy] = useState(false);
  const [newMergeOverridePath, setNewMergeOverridePath] = useState("");
  const [mergeNoteDismissed, setMergeNoteDismissed] = useState(false);

  const [sessionTitles, setSessionTitles] = useState<string[]>([]);
  const [sessionPaths, setSessionPaths] = useState<string[]>([]);

  const applyStatus = useCallback((status: NativeGitRolloutStatus) => {
    setWorktreeGlobalOverride(status.worktreeGlobalOverride ?? undefined);
    setWorktreeOverrides(status.worktreeSessionOverrides);
    setMergeGlobalOverride(status.mergeGlobalOverride ?? undefined);
    setMergeOverrides(status.mergeWorktreeOverrides);
  }, []);

  const load = useCallback(async () => {
    try {
      const status = await client.getNativeGitRolloutStatus({});
      applyStatus(status);
      setLoadError(false);
    } catch {
      // Task 4.3.1d: a load failure is a distinct "unknown" state, never
      // conflated with the explicit "not set, default off" (undefined) one.
      setWorktreeGlobalOverride("unknown");
      setMergeGlobalOverride("unknown");
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [client, applyStatus]);

  useEffect(() => {
    let cancelled = false;
    void load();
    sessionClient
      .listSessions({})
      .then((res) => {
        if (cancelled) return;
        setSessionTitles(res.sessions.map((s) => s.title).filter(Boolean));
        setSessionPaths(res.sessions.map((s) => s.path).filter(Boolean));
      })
      .catch(() => {
        // Non-fatal — both inputs still work as free text.
      });
    return () => {
      cancelled = true;
    };
  }, [load, sessionClient]);

  // Tracking for these mutations lives in RolloutSection, at the actual
  // click site — see its handleGlobalSet/handleAddOverride/handleRemoveOverride.
  const setWorktreeGlobalOverrideValue = useCallback(
    async (forceNative: boolean | undefined) => {
      setWorktreeBusy(true);
      setWorktreeError(null);
      try {
        const status = await client.setNativeWorktreeGlobalOverride({ forceNative });
        applyStatus(status);
      } catch {
        setWorktreeError("Failed to update the global override — try again");
      } finally {
        setWorktreeBusy(false);
      }
    },
    [client, applyStatus],
  );

  const addWorktreeOverride = useCallback(async () => {
    const sessionName = newWorktreeOverrideName.trim();
    if (!sessionName) return;
    setWorktreeBusy(true);
    setWorktreeError(null);
    try {
      const status = await client.setNativeWorktreeSessionOverride({ sessionName, forceNative: true });
      applyStatus(status);
      setNewWorktreeOverrideName("");
    } catch {
      setWorktreeError("Failed to set session override — try again");
    } finally {
      setWorktreeBusy(false);
    }
  }, [client, applyStatus, newWorktreeOverrideName]);

  const removeWorktreeOverride = useCallback(
    async (sessionName: string) => {
      setWorktreeBusy(true);
      setWorktreeError(null);
      try {
        const status = await client.setNativeWorktreeSessionOverride({ sessionName });
        applyStatus(status);
      } catch {
        setWorktreeError("Failed to clear override — try again");
      } finally {
        setWorktreeBusy(false);
      }
    },
    [client, applyStatus],
  );

  const setMergeGlobalOverrideValue = useCallback(
    async (forceNative: boolean | undefined) => {
      setMergeBusy(true);
      setMergeError(null);
      try {
        const status = await client.setNativeMergeGlobalOverride({ forceNative });
        applyStatus(status);
      } catch {
        setMergeError("Failed to update the global override — try again");
      } finally {
        setMergeBusy(false);
      }
    },
    [client, applyStatus],
  );

  const addMergeOverride = useCallback(async () => {
    const worktreePath = newMergeOverridePath.trim();
    if (!worktreePath) return;
    setMergeBusy(true);
    setMergeError(null);
    try {
      const status = await client.setNativeMergeWorktreeOverride({ worktreePath, forceNative: true });
      applyStatus(status);
      setNewMergeOverridePath("");
    } catch {
      setMergeError("Failed to set worktree-path override — try again");
    } finally {
      setMergeBusy(false);
    }
  }, [client, applyStatus, newMergeOverridePath]);

  const removeMergeOverride = useCallback(
    async (worktreePath: string) => {
      setMergeBusy(true);
      setMergeError(null);
      try {
        const status = await client.setNativeMergeWorktreeOverride({ worktreePath });
        applyStatus(status);
      } catch {
        setMergeError("Failed to clear override — try again");
      } finally {
        setMergeBusy(false);
      }
    },
    [client, applyStatus],
  );

  // Precedence-conflict detection (Task 4.3.1e): derived from current state
  // rather than tracked imperatively, so it's automatically correct after
  // load, a global toggle, or an add/remove — not just the toggle path.
  const worktreeConflictKeys = useMemo(() => {
    if (typeof worktreeGlobalOverride !== "boolean") return [];
    return worktreeOverrides.filter((o) => o.forceNative !== worktreeGlobalOverride).map((o) => o.sessionName);
  }, [worktreeGlobalOverride, worktreeOverrides]);
  const mergeConflictKeys = useMemo(() => {
    if (typeof mergeGlobalOverride !== "boolean") return [];
    return mergeOverrides.filter((o) => o.forceNative !== mergeGlobalOverride).map((o) => o.worktreePath);
  }, [mergeGlobalOverride, mergeOverrides]);

  const worktreeConflictSignature = worktreeConflictKeys.join(" ");
  useEffect(() => {
    setWorktreeNoteDismissed(false);
  }, [worktreeConflictSignature]);
  const mergeConflictSignature = mergeConflictKeys.join(" ");
  useEffect(() => {
    setMergeNoteDismissed(false);
  }, [mergeConflictSignature]);

  const worktreePrecedenceMessage =
    worktreeConflictKeys.length > 0 && !worktreeNoteDismissed && typeof worktreeGlobalOverride === "boolean"
      ? buildPrecedenceMessage(
          "session override",
          "session overrides",
          "native worktree management",
          "this session",
          worktreeConflictKeys,
          worktreeGlobalOverride,
        )
      : null;
  const mergePrecedenceMessage =
    mergeConflictKeys.length > 0 && !mergeNoteDismissed && typeof mergeGlobalOverride === "boolean"
      ? buildPrecedenceMessage(
          "worktree-path override",
          "worktree-path overrides",
          "native merge",
          "this worktree path",
          mergeConflictKeys,
          mergeGlobalOverride,
        )
      : null;

  const normalizedWorktreeOverrides = useMemo(
    () => worktreeOverrides.map((o) => ({ key: o.sessionName, forced: o.forceNative })),
    [worktreeOverrides],
  );
  const normalizedMergeOverrides = useMemo(
    () => mergeOverrides.map((o) => ({ key: o.worktreePath, forced: o.forceNative })),
    [mergeOverrides],
  );
  const worktreeConflictSet = useMemo(() => new Set(worktreeConflictKeys), [worktreeConflictKeys]);
  const mergeConflictSet = useMemo(() => new Set(mergeConflictKeys), [mergeConflictKeys]);

  if (loading) {
    return <p className={styles.description}>Loading…</p>;
  }

  return (
    <section className={styles.panel} data-testid="native-git-rollout-panel">
      <h2 className={styles.heading}>Native Git Rollout</h2>
      <p className={styles.description}>
        Pure-Go worktree management and merge, replacing subprocess git calls. Both flags
        default off. A change here takes effect on the next operation — no restart. A
        session/path override below always wins over the global setting in its own section.
      </p>

      {loadError && (
        <p className={styles.errorMessage} role="alert" data-testid="native-git-load-error-banner">
          Couldn&apos;t load rollout status — controls below may be stale.{" "}
          <button
            className={styles.actionButton}
            onClick={() => {
              track({ name: "native_git_rollout_load_retried", category: "user_action", component: "NativeGitRolloutPanel" });
              void load();
            }}
            data-testid="native-git-retry"
          >
            Retry
          </button>
        </p>
      )}

      <RolloutSection
        testidPrefix="native-git-worktree"
        copy={{
          sectionTitle: "Native Worktree Management",
          sectionDescription: 'The "native_git_worktree" feature flag defaults to off.',
          overridesHeading: "Per-session canary overrides",
          emptyOverridesText: "No sessions are currently overridden.",
          inputPlaceholder: "Session title (existing or new)",
          addButtonLabel: "Force worktree on for session",
        }}
        status={{ globalState: worktreeGlobalOverride, busy: worktreeBusy, error: worktreeError }}
        overrides={normalizedWorktreeOverrides}
        conflict={{
          keys: worktreeConflictSet,
          message: worktreePrecedenceMessage,
          onDismiss: () => setWorktreeNoteDismissed(true),
        }}
        input={{
          value: newWorktreeOverrideName,
          onChange: setNewWorktreeOverrideName,
          ariaLabel: "Session title",
          datalistOptions: sessionTitles,
        }}
        ariaLabels={WORKTREE_ARIA_LABELS}
        actions={{
          onGlobalSet: setWorktreeGlobalOverrideValue,
          onAddOverride: addWorktreeOverride,
          onRemoveOverride: removeWorktreeOverride,
        }}
      />

      <RolloutSection
        testidPrefix="native-git-merge"
        copy={{
          sectionTitle: "Native Merge",
          sectionDescription: 'The "native_git_merge" feature flag defaults to off.',
          overridesHeading: "Per-worktree-path overrides",
          emptyOverridesText: "No worktree paths are currently overridden.",
          inputPlaceholder: "Worktree path",
          addButtonLabel: "Force merge on for path",
        }}
        status={{ globalState: mergeGlobalOverride, busy: mergeBusy, error: mergeError }}
        overrides={normalizedMergeOverrides}
        conflict={{
          keys: mergeConflictSet,
          message: mergePrecedenceMessage,
          onDismiss: () => setMergeNoteDismissed(true),
        }}
        input={{
          value: newMergeOverridePath,
          onChange: setNewMergeOverridePath,
          ariaLabel: "Worktree path",
          datalistOptions: sessionPaths,
        }}
        ariaLabels={MERGE_ARIA_LABELS}
        actions={{
          onGlobalSet: setMergeGlobalOverrideValue,
          onAddOverride: addMergeOverride,
          onRemoveOverride: removeMergeOverride,
        }}
      />
    </section>
  );
}
