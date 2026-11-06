"use client";

import { forwardRef, useEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from "react";
import type { PrHold, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { PollError } from "@/gen/session/v1/types_pb";
import { isTerminal, prRowState, prTarget } from "@/lib/pr/pullRequests";
import { holdLine, isWarningHold, pollErrorText, reviewSummary, stateText, unverifiedLine } from "@/lib/pr/rowCopy";
import { PrStateGlyph } from "./PrStateGlyph";
import { usePopupMenu } from "./usePopupMenu";
import * as s from "./PullRequestList.css";

export type RowCommand = "merge" | "close" | "askToFix" | "pin" | "unpin" | "refresh" | "detach" | "copyLink";

interface RowProps {
  pr: SessionPullRequest;
  holds: readonly PrHold[];
  nowMs: number;
  canAskToFix: boolean;
  canAct: boolean;
  onCommand: (command: RowCommand, pr: SessionPullRequest, e: MouseEvent | KeyboardEvent) => void;
}

interface MenuItem { command: RowCommand; label: string }

function menuItems(pr: SessionPullRequest, canAskToFix: boolean, canAct: boolean): MenuItem[] {
  const state = prRowState(pr);
  const live = state === "open" || state === "draft";
  const items: MenuItem[] = [];
  if (canAct && state === "open") items.push({ command: "merge", label: "Merge" });
  if (canAct && live) items.push({ command: "close", label: "Close" });
  if (canAct && live && canAskToFix) items.push({ command: "askToFix", label: "Ask to fix" });
  if (canAct && !isTerminal(pr)) items.push(pr.pinned ? { command: "unpin", label: "Unpin" } : { command: "pin", label: "Pin as current" });
  if (!live || pr.pollError !== PollError.NONE) {
    if (state === "unknown" || state === "unverified" || pr.pollError !== PollError.NONE) {
      items.push({ command: "refresh", label: pr.pollError !== PollError.NONE ? "Retry" : "Refresh" });
    }
  }
  items.push({ command: "copyLink", label: "Copy link" });
  if (canAct) items.push({ command: "detach", label: "Not mine" });
  return items;
}

function RowMenu({ pr, items, onCommand }: { pr: SessionPullRequest; items: MenuItem[]; onCommand: RowProps["onCommand"] }) {
  const [open, setOpen] = useState(false);
  const { triggerRef, menuRef, onKeyDown } = usePopupMenu(open, setOpen);
  const target = prTarget(pr);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className={s.button}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`Actions for pull request ${target}`}
        data-testid={`pr-menu-${pr.number}`}
        onClick={() => setOpen((v) => !v)}
      >{/* analytics-exempt */}
        <span aria-hidden="true">...</span>
      </button>
      {open && (
        <div ref={menuRef} role="menu" aria-label={`Actions for ${target}`} className={s.menu} onKeyDown={onKeyDown}>
          {items.map((item) => (
            <button
              key={item.command}
              type="button"
              role="menuitem"
              className={s.menuItem}
              data-testid={`pr-menu-${item.command}-${pr.number}`}
              onClick={(e) => { setOpen(false); onCommand(item.command, pr, e); }}
            >{/* analytics-exempt */}
              {item.label}
            </button>
          ))}
          <a role="menuitem" className={s.menuItem} href={pr.url} target="_blank" rel="noopener noreferrer" onClick={() => setOpen(false)}>{/* analytics-exempt */}
            Open on GitHub
          </a>
        </div>
      )}
    </>
  );
}

/** One PR: link, state text, role chip, review summary, error chip, held line and its actions. */
export const PullRequestRow = forwardRef<HTMLLIElement, RowProps>(function PullRequestRow(
  { pr, holds, nowMs, canAskToFix, canAct, onCommand }, ref,
) {
  const state = prRowState(pr);
  const terminal = isTerminal(pr);
  const error = pollErrorText(pr, nowMs);
  const chip = pr.role === "followup" || pr.role === "stacked" ? pr.role : null;
  const summary = terminal ? [] : reviewSummary(pr);
  const named = `${pr.title ? `, ${pr.title}` : ""}${pr.headBranch ? `, branch ${pr.headBranch}` : ""}`;
  const items = menuItems(pr, canAskToFix, canAct);

  return (
    <li
      ref={ref}
      className={`${s.row} ${pr.isActive ? s.rowActive : ""} ${terminal ? s.rowTerminal : ""}`}
      aria-current={pr.isActive ? "true" : undefined}
      data-testid={`pr-row-${pr.number}`}
    >
      <div className={s.rowMain}>
        <div className={s.rowLine}>
          <PrStateGlyph state={state} className={`${s.glyph}`} />
          <a
            className={s.prLink}
            href={pr.url}
            target="_blank"
            rel="noopener noreferrer"
            data-testid={`pr-link-${pr.number}`}
            aria-label={`Pull request ${prTarget(pr)}${named}, opens in new tab`}
          >
            #{pr.number}
          </a>
          {pr.title && <span className={s.prTitleText} data-testid={`pr-title-${pr.number}`}>{pr.title}</span>}
          <span>{stateText(pr, nowMs)}</span>
          {pr.isActive && <span className={s.activeChip}>Active</span>}
          {chip && <span className={s.roleChip}>{chip}</span>}
        </div>
        {(pr.headBranch || summary.length > 0) && (
          <div className={s.rowMeta}>
            {pr.headBranch && <span>{pr.headBranch}</span>}
            {summary.map((t) => <span key={t}>{t}</span>)}
          </div>
        )}
        {state === "unverified" && <span className={s.rowMeta} aria-busy={!pr.verifyExhausted}>{unverifiedLine(pr)}</span>}
        {error && <span className={s.errorChip} data-testid={`pr-error-${pr.number}`}>{error}</span>}
        {holds.map((h) => (
          <p key={h.kind} className={`${s.holdLine} ${isWarningHold(h) ? s.holdWarning : ""}`} data-testid={`pr-hold-${pr.number}`}>
            {holdLine(h, pr, "session", nowMs)}
          </p>
        ))}
      </div>
      <div className={s.rowActions}>
        {pr.isActive && state === "open" && canAct && (
          <span className={s.inlineActions}>
            <button type="button" className={s.button} data-testid={`pr-merge-${pr.number}`} aria-label={`Merge ${prTarget(pr)}`} onClick={(e) => onCommand("merge", pr, e)}>{/* analytics-exempt */}Merge</button>
            {canAskToFix && (
              <button type="button" className={s.button} aria-label={`Ask to fix ${prTarget(pr)}`} onClick={(e) => onCommand("askToFix", pr, e)}>{/* analytics-exempt */}Ask to fix</button>
            )}
          </span>
        )}
        <RowMenu pr={pr} items={items} onCommand={onCommand} />
      </div>
    </li>
  );
});
