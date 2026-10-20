// +feature: backlog-diagnose-nudge
"use client";

import React, { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { timestampMs } from "@bufbuild/protobuf/wkt";
import { AutoMergeLookupStatus, StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { routes } from "@/lib/routes";
import { useAnalytics } from "@/lib/analytics";
import { resolveReworkCapOverride } from "@/lib/backlog/formatReworkCapOverride";
import { formatAgo, formatSinceUTC, isPrStatusUnknown } from "./stuckReason";
import * as styles from "./StuckItemDetail.css";

interface StuckItemDetailProps {
  item: StuckBacklogItem;
  /** Sets a per-item rework-cap override and immediately reopens the item — omitted disables the rework_cap override control. */
  onReworkCapOverride?: (itemId: string, override: number) => Promise<boolean>;
  /**
   * This item's current reworkCapOverride value (tri-state: undefined = no
   * override set / using global default, 0 = unlimited, >0 = this item's own
   * cap), fetched by the parent via getBacklogItem since StuckBacklogItem
   * (this component's `item` prop type) doesn't carry the field.
   */
  currentReworkCapOverride?: number;
  /**
   * True once the parent (StuckItemsSection) has resolved
   * `currentReworkCapOverride` for this item — either to a value or to
   * confirmed-unset. Before this is true, `currentReworkCapOverride` being
   * `undefined` is indistinguishable from "confirmed no override", so the
   * read-only display line below must not render "No override set" until
   * this flips true (see `formatReworkCapOverride`'s doc comment).
   */
  reworkCapOverrideLoaded?: boolean;
  /**
   * Approves the item's plan (ApprovePlan RPC) — omitted disables the
   * approve control entirely. Rejects (throws) on failure so this component
   * can surface the actual backend message (e.g. "no plan artifacts found")
   * instead of a generic error.
   */
  onApprovePlan?: (itemId: string) => Promise<void>;
  /**
   * Dispatches a "Diagnose & Nudge" agent for this item (backlog item
   * 68964304), scoped to this card's own StuckReason so the nudge
   * cap/cooldown gate (AC4) and the dispatched agent's action space key off
   * the right BacklogStuckState row. Omitted disables the control entirely.
   * Resolves with the dispatched diagnostic session's UUID, which this
   * component links to directly once dispatched — the session runs hidden
   * from the main list, so that link is the only way to open its transcript
   * and see what it actually did. Rejects (throws) on failure, mirroring
   * onApprovePlan above.
   */
  onDiagnose?: (itemId: string, reason: StuckReason) => Promise<string>;
  /**
   * Re-checks the repo auto-merge setting bypassing the server cache
   * (ListStuckBacklogItems with refreshAutoMerge) and resolves with the
   * refreshed item. Omitted hides the Retry button.
   */
  onRefreshAutoMerge?: (itemId: string) => Promise<StuckBacklogItem | undefined>;
}

const AUTO_MERGE_TTL_MS = 10 * 60 * 1000; // mirrors github.repoSettingsTTL
const TICK_MS = 30_000;

const FAILURE_REASON: Partial<Record<AutoMergeLookupStatus, string>> = {
  [AutoMergeLookupStatus.RATE_LIMITED]: "rate limited",
  [AutoMergeLookupStatus.NO_PERMISSION]: "no permission",
  [AutoMergeLookupStatus.NOT_FOUND]: "repo not found",
  [AutoMergeLookupStatus.ERROR]: "lookup error",
};

/** Repo settings URL (host, owner, repo) from the PR URL, or null when not derivable. */
function repoSettingsUrl(prUrl: string): string | null {
  try {
    const u = new URL(prUrl);
    const [owner, repo] = u.pathname.split("/").filter(Boolean);
    if (!/^https?:$/.test(u.protocol) || !owner || !repo) return null;
    return `${u.origin}/${owner}/${repo}/settings`;
  } catch {
    return null;
  }
}

function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

interface AutoMergeLineProps {
  item: StuckBacklogItem;
  /** Re-checks bypassing the cache; resolves with the refreshed item. Omitted hides Retry. */
  onRefresh?: (itemId: string) => Promise<StuckBacklogItem | undefined>;
}

/** Repo auto-merge status line (Story 3.1.3): one honest state per lookup status. */
function AutoMergeLine({ item, onRefresh }: AutoMergeLineProps) {
  const now = useNow(TICK_MS);
  const containerRef = useRef<HTMLDivElement>(null);
  const [fresh, setFresh] = useState<StuckBacklogItem | null>(null);
  const [checking, setChecking] = useState(false);
  const [announce, setAnnounce] = useState("");
  const focusContainerNext = useRef(false);
  useEffect(() => setFresh(null), [item]);

  const current = fresh ?? item;
  const status = current.autoMergeLookupStatus;
  const failed = status in FAILURE_REASON;
  const checkedMs = current.autoMergeCheckedAt ? timestampMs(current.autoMergeCheckedAt) : null;
  const ago = checkedMs === null ? null : formatAgo(current.autoMergeCheckedAt, now);
  const stale =
    status === AutoMergeLookupStatus.OK && checkedMs !== null && now - checkedMs > AUTO_MERGE_TTL_MS;
  const showRetry = !!onRefresh && (failed || stale || checking);

  // Success unmounts Retry; hand focus to the line container so it never falls to body.
  useEffect(() => {
    if (focusContainerNext.current && !showRetry) {
      focusContainerNext.current = false;
      containerRef.current?.focus();
    }
  });

  if (status === AutoMergeLookupStatus.UNSPECIFIED) return null;

  async function retry() {
    if (!onRefresh || checking) return;
    setChecking(true);
    setAnnounce("Checking repo auto-merge");
    try {
      const next = await onRefresh(item.itemId);
      const ok = next?.autoMergeLookupStatus === AutoMergeLookupStatus.OK;
      focusContainerNext.current = ok;
      if (next) setFresh(next);
      setAnnounce(
        !next
          ? "Repo auto-merge check skipped: item no longer listed"
          : ok
            ? "Repo auto-merge check succeeded"
            : "Repo auto-merge check failed again",
      );
    } catch {
      setAnnounce("Repo auto-merge check failed again");
    } finally {
      setChecking(false);
    }
  }

  const settingsUrl = repoSettingsUrl(item.prUrl);

  let line: string;
  let secondary: React.ReactNode = null;
  if (checking) {
    line = "Repo auto-merge: checking...";
  } else if (failed) {
    line = `Repo auto-merge: couldn't check (${FAILURE_REASON[status]})`;
    if (ago) secondary = `Last tried ${ago}.`;
  } else if (current.allowAutoMerge === false) {
    line = "Repo auto-merge: not allowed on this repo";
    secondary = (
      <>
        Merge this PR by hand, or enable Allow auto-merge in the repo settings.
        {settingsUrl && (
          <>
            {" "}
            <a
              className={`${styles.prLink} ${styles.touchLink}`}
              href={settingsUrl}
              target="_blank"
              rel="noopener"
              data-testid="stuck-item-auto-merge-settings-link"
            >
              Open repo settings (opens in new tab)
            </a>
          </>
        )}
      </>
    );
  } else if (current.allowAutoMerge === true) {
    line = "Repo auto-merge: allowed";
    secondary = "GitHub setting only. Branch protection or required checks can still block it.";
  } else {
    return null;
  }
  if (stale && !checking && ago) line += ` (as of ${ago}, may be out of date)`;

  return (
    <div
      ref={containerRef}
      tabIndex={-1}
      className={styles.autoMerge}
      aria-busy={checking}
      data-testid="stuck-item-auto-merge"
    >
      <span className={styles.value}>{line}</span>
      {secondary && <span className={styles.unknownNote}>{secondary}</span>}
      {showRetry && (
        <button
          key="retry"
          type="button"
          className={styles.overrideButton}
          aria-disabled={checking}
          onClick={() => void retry()}
          data-testid="stuck-item-auto-merge-retry"
        >
          {checking ? "Checking..." : "Retry"}
        </button>
      )}
      <span role="status" className={styles.srOnly} data-testid="stuck-item-auto-merge-status">
        {announce}
      </span>
    </div>
  );
}

/**
 * Read-only "current rework cap override" line. `currentReworkCapOverride`
 * is `undefined` both before the parent's fetch-on-expand has resolved and
 * when it resolves to a genuinely-unset override — those two states are
 * otherwise visually indistinguishable, so `loaded` (StuckItemsSection's
 * `reworkCapOverrides.has(item.itemId)`) must gate the "No override set"
 * copy specifically, rather than being inferred from `value`.
 */
function formatReworkCapOverride(value: number | undefined, loaded: boolean): string {
  if (!loaded) return "Checking current override…";
  const resolved = resolveReworkCapOverride(value);
  if (resolved.kind === "unset") return "No override set (using global default)";
  if (resolved.kind === "unlimited") return "Unlimited";
  return `${resolved.rounds} rounds`;
}

/**
 * Expanded accordion detail panel for a StuckItem. Renders inline beneath the
 * card (no portal/modal), mirroring UnfinishedItemDetail.tsx.
 */
export function StuckItemDetail({
  item,
  onReworkCapOverride,
  currentReworkCapOverride,
  reworkCapOverrideLoaded = false,
  onApprovePlan,
  onDiagnose,
  onRefreshAutoMerge,
}: StuckItemDetailProps) {
  const { track } = useAnalytics();
  const unknown = isPrStatusUnknown(item);
  const isPrReady = item.reason === StuckReason.PR_READY_UNMERGED;
  const isReworkCap = item.reason === StuckReason.REWORK_CAP;
  const isAutonomousStuck = item.reason === StuckReason.AUTONOMOUS_STUCK;
  const isPlanNotApprovedReason = item.reason === StuckReason.PLAN_NOT_APPROVED;
  // hasPlan gates the actionable "Approve Plan" affordance on a real plan
  // existing — `reason` alone lags actual PlanArtifactsPath state until the
  // next ReconcileStuck tick (see research/pitfalls.md #1), so without this
  // check the button could be shown with nothing behind it.
  const hasPlan = item.planArtifactsPath !== "";
  const isPlanNotApproved = isPlanNotApprovedReason && hasPlan;
  const why = item.context?.trim() ? item.context : "No additional context recorded";

  // Pre-fill with this item's existing explicit cap (>0) so the input
  // reflects reality instead of a generic default; 0 (unlimited) and
  // undefined (no override) have no positive number to show in this
  // min=1 numeric field, so both fall back to the "3" starting suggestion.
  const [moreRounds, setMoreRounds] = useState(
    currentReworkCapOverride !== undefined && currentReworkCapOverride > 0
      ? String(currentReworkCapOverride)
      : "3"
  );
  // currentReworkCapOverride is fetched by the parent (StuckItemsSection)
  // *after* this component has already mounted with it `undefined` (the
  // panel expands, then the getBacklogItem fetch resolves later) — so the
  // useState initializer above almost always runs against the unresolved
  // value and the input is stuck showing "3". This ref-guarded one-time sync
  // re-applies the real value once it arrives.
  //
  // hasSyncedOverride alone only tracks "has the sync fired once" — it does
  // NOT track "has the user already edited the field since mount". Without
  // userEditedRounds, a user who types into the field before the parent's
  // fetch resolves would have their input silently clobbered the moment the
  // fetch lands (the effect's guard was still false, so it fires and
  // overwrites whatever they typed). userEditedRounds, set in the input's
  // onChange handler, makes the sync a no-op once the user has started
  // typing, regardless of fetch timing.
  const hasSyncedOverride = useRef(false);
  const userEditedRounds = useRef(false);
  useEffect(() => {
    if (
      !hasSyncedOverride.current &&
      !userEditedRounds.current &&
      currentReworkCapOverride !== undefined
    ) {
      hasSyncedOverride.current = true;
      if (currentReworkCapOverride > 0) {
        setMoreRounds(String(currentReworkCapOverride));
      }
    }
  }, [currentReworkCapOverride]);
  const [overrideState, setOverrideState] = useState<"idle" | "pending" | "error">("idle");
  const [approveState, setApproveState] = useState<"idle" | "pending" | "error">("idle");
  const [approveError, setApproveError] = useState<string | null>(null);
  const [diagnoseState, setDiagnoseState] = useState<"idle" | "pending" | "dispatched" | "error">(
    "idle"
  );
  const [diagnoseError, setDiagnoseError] = useState<string | null>(null);
  // Each computed once and reused for both a button's aria-disabled and its
  // click guard below, so the two conditions can't drift apart.
  const isOverridePending = overrideState === "pending";
  const isSetCapDisabled = isOverridePending || !Number(moreRounds) || Number(moreRounds) <= 0;
  const isApprovePending = approveState === "pending";
  const isDiagnosePending = diagnoseState === "pending" || diagnoseState === "dispatched";
  // The dispatched diagnostic session's UUID (onDiagnose's resolved value),
  // used to render a direct "View diagnostic session" link below — see
  // onDiagnose's doc comment for why this link matters (the session is
  // hidden from the main session list).
  const [dispatchedSessionUuid, setDispatchedSessionUuid] = useState<string | null>(null);

  async function submitOverride(override: number) {
    // These buttons use aria-disabled instead of disabled (so they stay
    // focusable while busy), which doesn't block clicks on its own — guard
    // here against a rapid double click/Enter re-dispatching before the
    // first request settles.
    if (!onReworkCapOverride || isOverridePending) return;
    setOverrideState("pending");
    const ok = await onReworkCapOverride(item.itemId, override);
    setOverrideState(ok ? "idle" : "error");
  }

  async function submitApprovePlan() {
    if (!onApprovePlan || isApprovePending) return;
    setApproveState("pending");
    setApproveError(null);
    try {
      await onApprovePlan(item.itemId);
      setApproveState("idle");
    } catch (err) {
      setApproveState("error");
      setApproveError(err instanceof Error ? err.message : "Failed to approve — try again.");
    }
  }

  async function submitDiagnose() {
    if (!onDiagnose || isDiagnosePending) return;
    setDiagnoseState("pending");
    setDiagnoseError(null);
    try {
      const uuid = await onDiagnose(item.itemId, item.reason);
      setDispatchedSessionUuid(uuid);
      setDiagnoseState("dispatched");
    } catch (err) {
      setDiagnoseState("error");
      setDiagnoseError(err instanceof Error ? err.message : "Failed to dispatch — try again.");
    }
  }

  return (
    <div className={styles.detail} data-testid="stuck-item-detail">
      <div className={styles.row}>
        <span className={styles.label}>Why:</span>
        <span className={styles.value} data-testid="stuck-item-why">
          {why}
        </span>
      </div>

      {isPrReady && !unknown && (
        <p className={styles.actionCopy} data-testid="stuck-item-action-copy">
          This PR is ready — merge it on GitHub when you&apos;re ready.
        </p>
      )}

      {unknown && (
        <p className={styles.actionCopy} data-testid="stuck-item-no-action-copy">
          Couldn&apos;t check this PR&apos;s status — no action available.
        </p>
      )}

      {isReworkCap && (
        <>
          <p className={styles.actionCopy} data-testid="stuck-item-rework-cap-copy">
            Hit the auto-rework cap after repeated failed reviews. Raise this item&apos;s
            own cap below to let it keep retrying automatically, or click &quot;Reopen for
            Revision&quot; on the item to try one more round manually.
          </p>
          <div className={styles.row}>
            <span className={styles.label}>Current override:</span>
            <span className={styles.value} data-testid="stuck-item-rework-cap-current">
              {formatReworkCapOverride(currentReworkCapOverride, reworkCapOverrideLoaded)}
            </span>
          </div>
          {onReworkCapOverride && (
            <div className={styles.overrideForm} data-testid="stuck-item-rework-cap-override-form">
              <input
                type="number"
                min={1}
                value={moreRounds}
                onChange={(e) => {
                  userEditedRounds.current = true;
                  setMoreRounds(e.target.value);
                }}
                className={styles.overrideInput}
                aria-label="This item's new rework cap"
                data-testid="stuck-item-rework-cap-rounds-input"
                disabled={isOverridePending}
              />
              <button
                type="button"
                className={styles.overrideButton}
                aria-disabled={isSetCapDisabled}
                onClick={() => {
                  if (isSetCapDisabled) return;
                  track({
                    name: "stuck_item_rework_cap_override",
                    category: "user_action",
                    component: "StuckItemDetail",
                    labels: { unlimited: "false" },
                  });
                  void submitOverride(Number(moreRounds));
                }}
                data-testid="stuck-item-rework-cap-allow-rounds"
              >
                Set this item&apos;s cap to {moreRounds || 0} &amp; resume
              </button>
              <button
                type="button"
                className={styles.overrideUnlimitedButton}
                aria-disabled={isOverridePending}
                onClick={() => {
                  if (isOverridePending) return;
                  track({
                    name: "stuck_item_rework_cap_override",
                    category: "user_action",
                    component: "StuckItemDetail",
                    labels: { unlimited: "true" },
                  });
                  void submitOverride(0);
                }}
                data-testid="stuck-item-rework-cap-unlimited"
              >
                Remove cap for this item &amp; resume
              </button>
              {overrideState === "error" && (
                <span className={styles.overrideStatus} role="alert">
                  Failed to apply — try again.
                </span>
              )}
            </div>
          )}
        </>
      )}

      {isAutonomousStuck && (
        <p className={styles.actionCopy} data-testid="stuck-item-autonomous-stuck-copy">
          Autonomous mode stopped without a completion signal. Open the session to see
          what it accomplished, then either give it a manual instruction or use &quot;Reopen
          for Revision&quot; / re-trigger triage to let it try again.
        </p>
      )}

      {isPlanNotApprovedReason && !hasPlan && (
        <p className={styles.actionCopy} data-testid="stuck-item-no-action-copy">
          This item is flagged as waiting on plan approval, but no plan has been
          generated yet — run triage first. This will clear on the next check once a
          plan exists.
        </p>
      )}

      {isPlanNotApproved && (
        <>
          <p className={styles.actionCopy} data-testid="stuck-item-plan-not-approved-copy">
            This item is queued but can&apos;t be dequeued until its plan is approved (or
            skip_planning is set). Approve the plan below to unblock it.
          </p>
          {onApprovePlan && (
            <div className={styles.overrideForm} data-testid="stuck-item-approve-plan-form">
              <button
                type="button"
                className={styles.overrideButton}
                aria-disabled={isApprovePending}
                onClick={() => {
                  if (isApprovePending) return;
                  track({ name: "stuck_item_approve_plan", category: "user_action", component: "StuckItemDetail" });
                  void submitApprovePlan();
                }}
                data-testid="stuck-item-approve-plan"
              >
                {approveState === "pending" ? "Approving…" : "Approve Plan"}
              </button>
              {approveState === "error" && (
                <span className={styles.overrideStatus} role="alert" data-testid="stuck-item-approve-plan-error">
                  {approveError}
                </span>
              )}
            </div>
          )}
        </>
      )}

      <div className={styles.row}>
        <span className={styles.label}>Since:</span>
        <span className={styles.value}>{formatSinceUTC(item.firstDetectedAt)} (first detected)</span>
      </div>

      <div className={styles.row}>
        <span className={styles.label}>Last check:</span>
        <span className={styles.value} data-testid="stuck-item-last-check">
          {formatAgo(item.lastCheckedAt)}
        </span>
      </div>

      {isPrReady && (
        <div className={styles.row}>
          <AutoMergeLine item={item} onRefresh={onRefreshAutoMerge} />
        </div>
      )}

      {item.prNumber > 0 && (
        <div className={styles.row}>
          <span className={styles.label}>PR:</span>
          <a
            className={styles.prLink}
            href={item.prUrl}
            target="_blank"
            rel="noreferrer"
            data-testid="stuck-item-pr-link"
            aria-label={`View PR #${item.prNumber} on GitHub`}
          >
            🔗 View PR #{item.prNumber} on GitHub
          </a>
        </div>
      )}

      {onDiagnose && (
        <div className={styles.overrideForm} data-testid="stuck-item-diagnose-form">
          <button
            type="button"
            className={styles.overrideButton}
            aria-disabled={isDiagnosePending}
            onClick={() => {
              if (isDiagnosePending) return;
              track({
                name: "stuck_item_diagnose",
                category: "user_action",
                component: "StuckItemDetail",
                labels: { reason: String(item.reason) },
              });
              void submitDiagnose();
            }}
            data-testid="stuck-item-diagnose"
          >
            {diagnoseState === "pending"
              ? "Dispatching…"
              : diagnoseState === "dispatched"
                ? "Diagnostic session dispatched"
                : "Diagnose & Nudge"}
          </button>
          {diagnoseState === "error" && (
            <span className={styles.overrideStatus} role="alert" data-testid="stuck-item-diagnose-error">
              {diagnoseError}
            </span>
          )}
          {diagnoseState === "dispatched" && dispatchedSessionUuid && (
            <span className={styles.overrideStatus} data-testid="stuck-item-diagnose-dispatched-detail">
              <Link
                className={styles.prLink}
                href={routes.sessionDetail(dispatchedSessionUuid)}
                data-testid="stuck-item-diagnose-session-link"
                aria-label="Open the dispatched diagnostic session's transcript"
              >
                View diagnostic session →
              </Link>{" "}
              for its live log. Its conclusion (bug filed, note, or nudge) posts to this
              item&apos;s Activity Log — see &quot;Open item detail&quot; below.
            </span>
          )}
        </div>
      )}

      {/*
        review-gate-stale-session-rework Story 2.2.2: closes a pre-existing gap
        found during implementation (not new for this feature — applies to
        every StuckReason, including the 11 that predate it) where this panel
        never actually linked to the backlog item's own detail page, even
        though several reasons' actionCopy above tells the user to click
        "Reopen for Revision" — an action that only exists on that page
        (GateVerdictBox, web-app/src/components/backlog/BacklogItemDetail.tsx).
        Reuses the existing ?item= query-param navigation
        (BacklogQueueSection.tsx's identical `${routes.backlog}?item=...`
        pattern, handled by web-app/src/app/backlog/page.tsx) rather than
        inventing a new navigation mechanism.
      */}
      <div className={styles.row}>
        <Link
          className={styles.prLink}
          href={`${routes.backlog}?item=${encodeURIComponent(item.itemId)}`}
          data-testid="stuck-item-open-detail-link"
          aria-label="Open this item's full detail page"
        >
          Open item detail →
        </Link>
      </div>
    </div>
  );
}
