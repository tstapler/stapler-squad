"use client";

import { useRef, useState } from "react";
import type { PrHold, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { ConfirmPrDialog, TrackPrDialog } from "@/components/shared/PullRequestDialogs";
import { PrStateGlyph } from "@/components/shared/PrStateGlyph";
import * as shared from "@/components/shared/PullRequestList.css";
import { PullRequestUndoToast } from "@/components/shared/PullRequestUndoToast";
import type { ItemPullRequestActions } from "@/lib/hooks/useItemPullRequestActions";
import { canStopTracking, itemPrSummary } from "@/lib/pr/itemPullRequests";
import { holdsFor, prKey, prRowState, prTarget, prUrl, sortPullRequests, toRef } from "@/lib/pr/pullRequests";
import { holdLine, isWarningHold, pollErrorText, stateText, unverifiedLine } from "@/lib/pr/rowCopy";
import * as s from "./ItemPullRequestList.css";

interface Props {
  pullRequests: readonly SessionPullRequest[];
  holds: readonly PrHold[];
  /** Absent when the surface cannot change the item (rows then offer no exit). */
  actions?: ItemPullRequestActions;
  readOnly?: boolean;
  nowMs?: number;
}

type Toast = { id: number; message: string; undo: () => void };

function ItemRow({ pr, nowMs, onStop }: { pr: SessionPullRequest; nowMs: number; onStop?: (pr: SessionPullRequest) => void }) {
  const state = prRowState(pr);
  const error = pollErrorText(pr, nowMs);
  return (
    <li className={`${shared.row} ${canStopTracking(pr) ? "" : shared.rowTerminal}`} data-testid={`item-pr-row-${pr.number}`}>
      <div className={shared.rowMain}>
        <div className={shared.rowLine}>
          <PrStateGlyph state={state} className={shared.glyph} />
          <a className={shared.prLink} href={prUrl(pr)} target="_blank" rel="noopener noreferrer" aria-label={`Pull request ${prTarget(pr)}${pr.title ? `, ${pr.title}` : ""}, opens in new tab`}>
            #{pr.number}
          </a>
          {pr.title && <span className={shared.prTitleText} data-testid={`item-pr-title-${pr.number}`}>{pr.title}</span>}
          <span>{stateText(pr, nowMs)}</span>
        </div>
        {pr.headBranch && <div className={shared.rowMeta}><span>{pr.headBranch}</span></div>}
        {onStop && !pr.itemDirect && canStopTracking(pr) && <div className={shared.rowMeta} data-testid={`item-pr-via-session-${pr.number}`}><span>Tracked by the item&apos;s session; stop it there.</span></div>}
        {state === "unverified" && <span className={shared.rowMeta}>{unverifiedLine(pr)}</span>}
        {error && <span className={shared.errorChip}>{error}</span>}
      </div>
      {onStop && pr.itemDirect && canStopTracking(pr) && (
        <div className={shared.rowActions}>
          <button type="button" className={shared.button} aria-label={`Stop tracking ${prTarget(pr)}`} data-testid={`item-pr-stop-${pr.number}`} onClick={() => onStop(pr)}>{/* analytics-exempt */}
            Stop tracking
          </button>
        </div>
      )}
    </li>
  );
}

/** The backlog item's PRs: "k of N merged", the done rule, held lines, one row per PR (ux.md S5). */
export function ItemPullRequestList({ pullRequests, holds, actions, readOnly = false, nowMs = Date.now() }: Props) {
  const [dialog, setDialog] = useState<{ kind: "stop"; pr: SessionPullRequest } | { kind: "track" } | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const toastSeq = useRef(0);
  const summary = itemPrSummary(pullRequests);
  const canAct = !!actions && !readOnly;
  const byKey = new Map(pullRequests.map((pr) => [prKey(pr), pr]));

  const closeDialog = () => {
    setDialog(null);
    setTimeout(() => headingRef.current?.focus(), 0);
  };

  return (
    <div className={s.stack} data-testid="item-pr-list">
      <div className={shared.header}>
        <h4 ref={headingRef} tabIndex={-1} className={shared.heading} id="item-pull-requests-heading">Pull requests ({summary.total})</h4>
        {canAct && (
          <button type="button" className={shared.button} data-testid="item-pr-track" onClick={() => setDialog({ kind: "track" })}>{/* analytics-exempt */}
            Track a PR by URL
          </button>
        )}
      </div>
      <div className={s.summaryLine}>
        <p className={s.summaryText} data-testid="item-pr-summary">{summary.text}</p>
      </div>
      {summary.showBar && (
        <progress className={s.progress} value={summary.merged} max={summary.total} aria-label="Pull requests merged" aria-valuetext={summary.valueText} data-testid="item-pr-progress" />
      )}
      <p className={s.ruleLine} data-testid="item-pr-rule">{summary.ruleLine}</p>
      {holds.length > 0 && (
        <ul className={s.holds} aria-label="Why cleanup is held">
          {holds.map((h) => (
            <li key={`${h.kind}-${h.pr ? prKey(h.pr) : ""}`} className={`${shared.holdLine} ${isWarningHold(h) ? shared.holdWarning : ""}`} data-testid="item-pr-hold">
              {holdLine(h, h.pr ? byKey.get(prKey(h.pr)) : undefined, "item", nowMs)}
            </li>
          ))}
        </ul>
      )}
      <ul className={shared.list}>
        {sortPullRequests(pullRequests).map((pr) => (
          <ItemRow key={prKey(pr)} pr={pr} nowMs={nowMs} onStop={canAct ? (p) => setDialog({ kind: "stop", pr: p }) : undefined} />
        ))}
      </ul>

      {dialog?.kind === "stop" && actions && (
        <ConfirmPrDialog
          testId="item-pr-stop-dialog"
          title={`Stop tracking ${prTarget(dialog.pr)} for this item?`}
          body="The PR stays on GitHub. It won't be added back automatically; you can track it again from Track a PR by URL. A session that still holds it keeps its own local files until that PR is merged or closed."
          confirmLabel="Stop tracking"
          busyLabel="Removing..."
          onConfirm={async () => {
            const result = await actions.stopTracking(toRef(dialog.pr));
            if (result.ok) {
              toastSeq.current += 1;
              setToast({ id: toastSeq.current, message: `Stopped tracking #${dialog.pr.number}.`, undo: () => void actions.trackByUrl(prUrl(dialog.pr)) });
            }
            return result;
          }}
          onClose={() => setDialog(null)}
          onDone={closeDialog}
        />
      )}
      {dialog?.kind === "track" && actions && (
        <TrackPrDialog onAttach={actions.trackByUrl} onClose={() => setDialog(null)} onDone={closeDialog} />
      )}
      {toast && (
        <PullRequestUndoToast
          key={toast.id}
          message={toast.message}
          hint="Undo, or use Track a PR by URL."
          focusUndoOnAppear={false}
          onUndo={toast.undo}
          onDismiss={() => setToast(null)}
        />
      )}
    </div>
  );
}
