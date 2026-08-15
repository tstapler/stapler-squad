"use client";

import { useState } from "react";
import Link from "next/link";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { routes } from "@/lib/routes";
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
   * Approves the item's plan (ApprovePlan RPC) — omitted disables the
   * approve control entirely. Rejects (throws) on failure so this component
   * can surface the actual backend message (e.g. "no plan artifacts found")
   * instead of a generic error.
   */
  onApprovePlan?: (itemId: string) => Promise<void>;
}

/** Read-only "Repo auto-merge: on/off/unknown" line (Story 4.1.4). `allowAutoMerge` is
 * unset ("not fetched / unknown", never treated as "disabled") whenever the
 * server's best-effort fetch hasn't populated it. */
function autoMergeLine(allowAutoMerge: boolean | undefined): string {
  if (allowAutoMerge === undefined) return "Repo auto-merge: unknown";
  return `Repo auto-merge: ${allowAutoMerge ? "on" : "off"} (allow_auto_merge: ${allowAutoMerge})`;
}

/**
 * Read-only "current rework cap override" line. `reworkCapOverride` is
 * tri-state — undefined means no override is set (global default applies),
 * 0 explicitly means unlimited retries for this item, and any other value is
 * this item's own cap — so this must use `=== undefined` / `=== 0` checks,
 * never `||`/truthy coercion (which would misreport an explicit 0 as unset).
 */
function formatReworkCapOverride(value: number | undefined): string {
  if (value === undefined) return "No override set (using global default)";
  if (value === 0) return "Unlimited";
  return `${value} rounds`;
}

/**
 * Expanded accordion detail panel for a StuckItem. Renders inline beneath the
 * card (no portal/modal), mirroring UnfinishedItemDetail.tsx.
 */
export function StuckItemDetail({
  item,
  onReworkCapOverride,
  currentReworkCapOverride,
  onApprovePlan,
}: StuckItemDetailProps) {
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
  const [overrideState, setOverrideState] = useState<"idle" | "pending" | "error">("idle");
  const [approveState, setApproveState] = useState<"idle" | "pending" | "error">("idle");
  const [approveError, setApproveError] = useState<string | null>(null);

  async function submitOverride(override: number) {
    if (!onReworkCapOverride) return;
    setOverrideState("pending");
    const ok = await onReworkCapOverride(item.itemId, override);
    setOverrideState(ok ? "idle" : "error");
  }

  async function submitApprovePlan() {
    if (!onApprovePlan) return;
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
              {formatReworkCapOverride(currentReworkCapOverride)}
            </span>
          </div>
          {onReworkCapOverride && (
            <div className={styles.overrideForm} data-testid="stuck-item-rework-cap-override-form">
              <input
                type="number"
                min={1}
                value={moreRounds}
                onChange={(e) => setMoreRounds(e.target.value)}
                className={styles.overrideInput}
                aria-label="This item's new rework cap"
                data-testid="stuck-item-rework-cap-rounds-input"
                disabled={overrideState === "pending"}
              />
              <button
                type="button"
                className={styles.overrideButton}
                disabled={overrideState === "pending" || !Number(moreRounds) || Number(moreRounds) <= 0}
                onClick={() => void submitOverride(Number(moreRounds))}
                data-testid="stuck-item-rework-cap-allow-rounds"
              >
                Set this item&apos;s cap to {moreRounds || 0} &amp; resume
              </button>
              <button
                type="button"
                className={styles.overrideUnlimitedButton}
                disabled={overrideState === "pending"}
                onClick={() => void submitOverride(0)}
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
                disabled={approveState === "pending"}
                onClick={() => void submitApprovePlan()}
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
          <span
            className={`${item.allowAutoMerge === undefined ? styles.unknownNote : styles.value}`}
            data-testid="stuck-item-auto-merge"
          >
            {autoMergeLine(item.allowAutoMerge)}
          </span>
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
