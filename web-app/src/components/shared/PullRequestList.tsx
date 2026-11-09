"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PrHold, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { useMediaQuery } from "@/lib/hooks/useMediaQuery";
import type { PrActionResult, PrTarget, PullRequestActions } from "@/lib/pr/actionResult";
import { PULL_REQUESTS_ANCHOR, usePullRequestsAnchor } from "@/lib/pr/anchor";
import { holdsFor, planRows, prKey, prTarget, prUrl, toRef } from "@/lib/pr/pullRequests";
import { holdLine } from "@/lib/pr/rowCopy";
import { usePullRequestAnnouncements } from "@/lib/pr/usePullRequestAnnouncements";
import { ConfirmPrDialog, MergeDialog, TrackPrDialog } from "./PullRequestDialogs";
import * as s from "./PullRequestList.css";
import { PullRequestRow, type RowCommand } from "./PullRequestRow";
import { PullRequestUndoToast } from "./PullRequestUndoToast";
import { usePopupMenu } from "./usePopupMenu";

const PHONE = "(max-width: 480px)";
const LOAD_TIMEOUT_MS = 10_000;

export interface PullRequestListProps {
  pullRequests: readonly SessionPullRequest[];
  holds?: readonly PrHold[];
  actions: PullRequestActions;
  /** True until the first list arrives; after 10 s the skeleton becomes the error state. */
  loading?: boolean;
  loadError?: boolean;
  onRetryLoad?: () => void;
  /** Legacy single-PR link kept visible when the list fails to load. */
  fallbackUrl?: string;
  onCreatePullRequest?: () => void;
  /** Injectable clock for relative times in tests. */
  nowMs?: number;
}

type Dialog = { kind: "merge" | "close" | "detach"; pr: SessionPullRequest } | { kind: "track" } | null;
type Toast = { id: number; message: string; hint: string; undo: () => void; keyboard: boolean };
type FocusPlan = { key: string; index: number };

const asTarget = (pr: SessionPullRequest): PrTarget => toRef(pr);

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/** The "Pull requests" section: every PR of a session, the held state of each, and per-PR actions. */
export function PullRequestList(props: PullRequestListProps) {
  const { pullRequests, holds = [], actions, loading, loadError, onRetryLoad, fallbackUrl, onCreatePullRequest } = props;
  const nowMs = props.nowMs ?? Date.now();
  const compact = useMediaQuery(PHONE);
  const [expanded, setExpanded] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const [note, setNote] = useState<{ text: string; alert: boolean } | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [timedOut, setTimedOut] = useState(false);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const { triggerRef: headerMenuRef, menuRef: headerMenuPanelRef, onKeyDown: onHeaderMenuKeyDown } = usePopupMenu(menuOpen, setMenuOpen);
  const rowEls = useRef(new Map<string, HTMLLIElement>());
  const toastFocus = useRef<FocusPlan | null>(null);
  const dialogFocus = useRef<FocusPlan | null>(null);
  const toastSeq = useRef(0);
  const keyboardToast = useRef(false);

  usePullRequestsAnchor(headingRef, !loading);

  const plan = useMemo(() => planRows(pullRequests, { expanded, compact }), [pullRequests, expanded, compact]);
  const visibleKeys = plan.visible.map(prKey);
  const visibleKeysRef = useRef(visibleKeys);
  visibleKeysRef.current = visibleKeys;

  const announcement = usePullRequestAnnouncements(pullRequests, holds, (h, pr) => holdLine(h, pr, "session", Date.now()));

  useEffect(() => {
    if (!loading) { setTimedOut(false); return; }
    const t = setTimeout(() => setTimedOut(true), LOAD_TIMEOUT_MS);
    return () => clearTimeout(t);
  }, [loading]);

  const focusDestination = useCallback((plan: FocusPlan) => {
    const keys = visibleKeysRef.current;
    const invoker = keys[plan.index] === plan.key ? rowEls.current.get(plan.key)?.querySelector<HTMLElement>('[aria-haspopup="menu"]') : null;
    const slot = keys[plan.index] ?? keys[plan.index - 1];
    const target = invoker ?? (slot ? rowEls.current.get(slot)?.querySelector<HTMLElement>("a") : null) ?? headingRef.current;
    target?.focus();
  }, []);

  const planFocus = (pr: SessionPullRequest): FocusPlan => ({ key: prKey(pr), index: visibleKeysRef.current.indexOf(prKey(pr)) });

  // After the action's re-render: focus the invoker if its row stayed put, else the row now in its slot,
  // else the previous row, else the heading (never document.body).
  const focusAfterRender = (plan: FocusPlan) => { setTimeout(() => focusDestination(plan), 0); };

  const showToast = (message: string, hint: string, undo: () => void, keyboard: boolean) => {
    keyboardToast.current = keyboard;
    toastSeq.current += 1;
    setToast({ id: toastSeq.current, message, hint, undo, keyboard });
  };

  const run = async (label: string, action: () => Promise<PrActionResult>): Promise<PrActionResult> => {
    const result = await action();
    setNote(result.ok ? null : { text: `${label}: ${result.message}`, alert: true });
    return result;
  };

  const refresh = async () => {
    setRefreshing(true);
    await run("Couldn't refresh", actions.refresh);
    setRefreshing(false);
  };

  const pin = async (pr: SessionPullRequest, keyboard: boolean) => {
    const previous = pullRequests.find((p) => p.pinned && prKey(p) !== prKey(pr));
    const plan = planFocus(pr);
    const result = await run(`Couldn't pin #${pr.number}`, () => actions.pin(asTarget(pr)));
    if (!result.ok) return;
    toastFocus.current = plan;
    // A keyboard pin gives the focus to the toast's Undo; a deferred focus on the row would take it back.
    if (!keyboard) focusAfterRender(plan);
    showToast(`Pinned #${pr.number} as current.`, "Undo, or choose Unpin in the row menu.", () => void actions.pin(previous ? asTarget(previous) : null), keyboard);
  };

  const detach = async (pr: SessionPullRequest, keyboard: boolean): Promise<PrActionResult> => {
    const result = await actions.detach(asTarget(pr));
    if (result.ok) {
      toastFocus.current = dialogFocus.current;
      showToast(`Stopped tracking #${pr.number}.`, "Undo, or use Track a PR by URL.", () => void actions.attachByUrl(prUrl(pr)), keyboard);
    }
    return result;
  };

  const onCommand = (command: RowCommand, pr: SessionPullRequest, e: { detail?: number }) => {
    const keyboard = (e.detail ?? 1) === 0;
    keyboardToast.current = false;
    switch (command) {
      case "merge": case "close": case "detach": dialogFocus.current = planFocus(pr); setDialog({ kind: command, pr }); break;
      case "pin": void pin(pr, keyboard); break;
      case "unpin": void run(`Couldn't unpin #${pr.number}`, () => actions.pin(null)).then((r) => { if (r.ok) focusAfterRender(planFocus(pr)); }); break;
      case "refresh": void refresh(); break;
      case "askToFix": {
        const { askToFix } = actions;
        if (!askToFix) break;
        void run(`Couldn't ask to fix #${pr.number}`, () => askToFix(asTarget(pr))).then((r) => { if (r.ok) setNote({ text: r.notice ?? `Asked the session to fix ${prTarget(pr)}.`, alert: false }); });
        break;
      }
      case "copyLink":
        void copyText(prUrl(pr)).then((ok) => setNote(ok ? { text: `Copied link to ${prTarget(pr)}.`, alert: false } : { text: "Couldn't copy link", alert: true }));
        break;
    }
  };

  const finishDialog = () => {
    setDialog(null);
    if (dialogFocus.current && !keyboardToast.current) focusAfterRender(dialogFocus.current);
  };

  // Cancel and Esc: the focus trap hands focus back to the invoker, but a row-menu item is gone by then, so
  // focus would drop to <body>. Only in that case, go to the row's menu (or the header menu): UX-S3-3.
  const cancelDialog = () => {
    setDialog(null);
    setTimeout(() => {
      if (document.activeElement && document.activeElement !== document.body) return;
      if (dialogFocus.current) focusDestination(dialogFocus.current);
      else headerMenuRef.current?.focus();
    }, 0);
  };

  const active = pullRequests.find((p) => p.isActive);
  const showHint = pullRequests.length > 1 && !!active;
  const failedToLoad = loadError || (loading && timedOut);

  const header = (
    <div className={s.header}>
      <h3 id="pull-requests-heading" ref={headingRef} tabIndex={-1} className={s.heading}>
        Pull requests{pullRequests.length > 0 ? ` (${pullRequests.length})` : ""}
      </h3>
      <div className={s.headerActions}>
        <button type="button" className={s.button} onClick={() => void refresh()} aria-busy={refreshing} disabled={refreshing} data-testid="pr-refresh">{/* analytics-exempt */}
          {refreshing ? "Refreshing" : "Refresh"}
        </button>
        <button ref={headerMenuRef} type="button" className={s.button} aria-haspopup="menu" aria-expanded={menuOpen} aria-label="More pull request actions" onClick={() => setMenuOpen((v) => !v)} data-testid="pr-header-menu">{/* analytics-exempt */}
          <span aria-hidden="true">...</span>
        </button>
        {menuOpen && (
          <div ref={headerMenuPanelRef} role="menu" className={s.menu} onKeyDown={onHeaderMenuKeyDown}>
            <button type="button" role="menuitem" className={s.menuItem} data-testid="pr-track-by-url" onClick={() => { setMenuOpen(false); dialogFocus.current = null; setDialog({ kind: "track" }); }}>{/* analytics-exempt */}
              Track a PR by URL
            </button>
          </div>
        )}
      </div>
    </div>
  );

  let content;
  if (failedToLoad) {
    content = (
      <div role="alert" className={s.alertBox}>
        <span>Couldn&apos;t load pull requests</span>
        {onRetryLoad && <button type="button" className={s.button} onClick={onRetryLoad}>{/* analytics-exempt */}Retry</button>}
        {fallbackUrl && <a href={fallbackUrl} target="_blank" rel="noopener noreferrer" className={s.prLink}>Open the pull request on GitHub</a>}
      </div>
    );
  } else if (loading) {
    content = (
      <div aria-busy="true" data-testid="pr-list-loading">
        <span className={s.visuallyHidden}>Loading pull requests</span>
        <div className={s.skeleton} />
      </div>
    );
  } else if (pullRequests.length === 0) {
    content = (
      <div className={s.empty}>
        <span>No pull requests yet.</span>
        {onCreatePullRequest && <button type="button" className={s.button} onClick={onCreatePullRequest}>{/* analytics-exempt */}Create pull request</button>}
      </div>
    );
  } else {
    content = (
      <>
        <ul id="pr-list-items" className={s.list} data-testid="pr-list">
          {plan.visible.map((pr) => (
            <PullRequestRow
              key={prKey(pr)}
              ref={(el) => { if (el) rowEls.current.set(prKey(pr), el); else rowEls.current.delete(prKey(pr)); }}
              pr={pr}
              holds={holdsFor(holds, pr)}
              nowMs={nowMs}
              canAskToFix={!!actions.askToFix}
              canAct
              onCommand={onCommand}
            />
          ))}
        </ul>
        {plan.disclosure && (
          <button type="button" className={s.disclosure} aria-expanded={expanded} aria-controls="pr-list-items" data-testid="pr-disclosure" onClick={() => setExpanded((v) => !v)}>{/* analytics-exempt */}
            {plan.disclosure === "earlier"
              ? expanded ? "Hide earlier PRs" : `${plan.hidden} earlier ${plan.hidden === 1 ? "PR" : "PRs"}`
              : expanded ? "Show fewer" : `Show all (${pullRequests.length})`}
          </button>
        )}
      </>
    );
  }

  return (
    <section id={PULL_REQUESTS_ANCHOR} aria-labelledby="pull-requests-heading" className={s.section} data-testid="pr-section">
      {header}
      {showHint && active && <p className={s.hint}>Actions apply to {prTarget(active)} unless you pick another.</p>}
      {content}
      {note && (
        <p role={note.alert ? "alert" : "status"} className={note.alert ? s.errorChip : s.hint} data-testid="pr-note">
          {note.text}
        </p>
      )}
      <div role="status" aria-live="polite" className={s.visuallyHidden} data-testid="pr-live-region">{announcement}</div>

      {dialog?.kind === "merge" && (
        <MergeDialog
          target={prTarget(dialog.pr)}
          baseBranch={dialog.pr.baseBranch}
          isActive={dialog.pr.isActive}
          url={prUrl(dialog.pr)}
          onMerge={(method) => actions.merge(asTarget(dialog.pr), method)}
          onClose={cancelDialog}
          onDone={finishDialog}
          onStale={() => void actions.refresh()}
        />
      )}
      {dialog?.kind === "close" && (
        <ConfirmPrDialog
          testId="pr-close-dialog"
          title={`Close ${prTarget(dialog.pr)} without merging?`}
          body={`The pull request is closed on GitHub, not merged. You can reopen it there.`}
          confirmLabel="Close pull request"
          busyLabel="Closing..."
          danger
          onConfirm={() => actions.close(asTarget(dialog.pr))}
          onClose={cancelDialog}
          onDone={finishDialog}
        />
      )}
      {dialog?.kind === "detach" && (
        <ConfirmPrDialog
          testId="pr-detach-dialog"
          title={`Remove ${prTarget(dialog.pr)} from this session?`}
          body="The PR stays on GitHub. It won't be added back automatically; you can track it again from Track a PR by URL."
          confirmLabel="Not mine"
          busyLabel="Removing..."
          onConfirm={(keyboard) => detach(dialog.pr, keyboard)}
          onClose={cancelDialog}
          onDone={finishDialog}
        />
      )}
      {dialog?.kind === "track" && (
        <TrackPrDialog onAttach={actions.attachByUrl} onClose={cancelDialog} onDone={(notice) => { cancelDialog(); if (notice) setNote({ text: notice, alert: false }); }} />
      )}

      {toast && (
        <PullRequestUndoToast
          key={toast.id}
          message={toast.message}
          hint={toast.hint}
          focusUndoOnAppear={toast.keyboard}
          onUndo={toast.undo}
          onDismiss={(hadFocus) => {
            setToast(null);
            if (hadFocus && toastFocus.current) focusDestination(toastFocus.current);
          }}
        />
      )}
    </section>
  );
}
