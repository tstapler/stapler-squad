"use client";

import { useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useDialogFocusTrap } from "@/lib/hooks/useDialogFocusTrap";
import type { PrActionResult, PrFailureKind } from "@/lib/pr/actionResult";
import * as s from "./PullRequestDialogs.css";

type MergeMethod = "merge" | "squash" | "rebase";

/** Runs one action; keeps the dialog open with an inline alert on failure (UX-S3-4). */
function useDialogAction(run: () => Promise<PrActionResult>, onDone: (notice?: string) => void) {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<{ kind: PrFailureKind; message: string } | null>(null);
  const submit = async () => {
    setBusy(true);
    setFailed(null);
    const result = await run();
    setBusy(false);
    if (result.ok) onDone(result.notice);
    else setFailed({ kind: result.kind, message: result.message });
  };
  return { busy, failed, submit };
}

interface FrameProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  testId: string;
  onSubmit?: (e: FormEvent) => void;
  initialFocus: React.RefObject<HTMLElement | null>;
  busy?: boolean;
}

/** Focus-trapped modal that is a bottom sheet at phone width. Esc and backdrop cancel unless busy. */
function DialogFrame({ title, onClose, children, testId, onSubmit, initialFocus, busy }: FrameProps) {
  const ref = useRef<HTMLDivElement>(null);
  const titleId = useId();
  useDialogFocusTrap({ dialogRef: ref, initialFocusRef: initialFocus, onEscape: () => { if (!busy) onClose(); } });
  return (
    <div className={s.backdrop} onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) onClose(); }}>
      <div ref={ref} role="dialog" aria-modal="true" aria-labelledby={titleId} className={s.sheet} data-testid={testId}>
        <h4 id={titleId} className={s.title}>{title}</h4>
        <form onSubmit={onSubmit ?? ((e) => e.preventDefault())} className={s.form}>
          {children}
        </form>
      </div>
    </div>
  );
}

function ErrorLine({ message }: { message: string }) {
  return <p role="alert" className={s.errorBox}>{message}</p>;
}

interface MergeDialogProps {
  target: string;
  baseBranch: string;
  isActive: boolean;
  url: string;
  onMerge: (method: MergeMethod) => Promise<PrActionResult>;
  onClose: () => void;
  onDone: () => void;
  /** Refreshes the list after a failure that means the list is stale. */
  onStale: () => void;
}

export function MergeDialog({ target, baseBranch, isActive, url, onMerge, onClose, onDone, onStale }: MergeDialogProps) {
  const [method, setMethod] = useState<MergeMethod>("merge");
  const confirmRef = useRef<HTMLButtonElement>(null);
  const { busy, failed, submit } = useDialogAction(async () => {
    const result = await onMerge(method);
    if (!result.ok && (result.kind === "not_attached" || result.kind === "already_merged")) onStale();
    return result;
  }, onDone);
  const closeOnly = failed?.kind === "already_merged" || failed?.kind === "not_attached";
  return (
    <DialogFrame
      title={`Merge ${target} into ${baseBranch || "main"}?`}
      onClose={onClose}
      testId="pr-merge-dialog"
      busy={busy}
      initialFocus={confirmRef}
      onSubmit={(e) => { e.preventDefault(); if (!closeOnly) void submit(); }}
    >
      {isActive && <p className={s.body}>This is the active pull request.</p>}
      {!closeOnly && (
        <label className={s.field}>
          Merge method
          <select className={s.input} value={method} disabled={busy} onChange={(e) => setMethod(e.target.value as MergeMethod)}>
            <option value="merge">Create a merge commit</option>
            <option value="squash">Squash and merge</option>
            <option value="rebase">Rebase and merge</option>
          </select>
        </label>
      )}
      {failed && <ErrorLine message={failed.kind === "already_merged" ? `${target.split(" ")[0]} was already merged` : `Can't merge ${target.split(" ")[0]}: ${failed.message}`} />}
      <div className={s.actions}>
        {failed && !closeOnly && <a className={s.button} href={url} target="_blank" rel="noopener noreferrer">Open on GitHub</a>}
        <button type="button" className={s.button} onClick={onClose} disabled={busy}>{/* analytics-exempt */}{closeOnly ? "Dismiss" : "Cancel"}</button>
        {!closeOnly && (
          <button ref={confirmRef} type="submit" className={`${s.button} ${s.primaryButton}`} disabled={busy} aria-busy={busy}>
            {busy ? "Merging..." : failed ? "Retry merge" : "Merge"}
          </button>
        )}
      </div>
    </DialogFrame>
  );
}

interface ConfirmDialogProps {
  title: string;
  body: string;
  confirmLabel: string;
  busyLabel: string;
  danger?: boolean;
  testId: string;
  onConfirm: (viaKeyboard: boolean) => Promise<PrActionResult>;
  onClose: () => void;
  onDone: () => void;
}

/** Close and Not mine/Stop tracking: copy states what does and does not happen on GitHub (UX-S3-5). */
export function ConfirmPrDialog({ title, body, confirmLabel, busyLabel, danger, testId, onConfirm, onClose, onDone }: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const keyboard = useRef(false);
  const { busy, failed, submit } = useDialogAction(() => onConfirm(keyboard.current), onDone);
  return (
    <DialogFrame title={title} onClose={onClose} testId={testId} busy={busy} initialFocus={cancelRef} onSubmit={(e) => { e.preventDefault(); void submit(); }}>
      <p className={s.body}>{body}</p>
      {failed && <ErrorLine message={failed.message} />}
      <div className={s.actions}>
        <button ref={cancelRef} type="button" className={s.button} onClick={onClose} disabled={busy}>{/* analytics-exempt */}Cancel</button>
        <button
          type="submit"
          className={`${s.button} ${danger ? s.dangerButton : s.primaryButton}`}
          disabled={busy}
          aria-busy={busy}
          onClick={(e) => { keyboard.current = e.detail === 0; }}
        >{/* analytics-exempt */}
          {busy ? busyLabel : failed ? `Retry: ${confirmLabel}` : confirmLabel}
        </button>
      </div>
    </DialogFrame>
  );
}

const URL_ERRORS: Partial<Record<PrFailureKind, string>> = {
  invalid_url: "That is not a GitHub pull request URL.",
  not_found: "Not found on GitHub.",
};

interface TrackDialogProps {
  onAttach: (url: string) => Promise<PrActionResult>;
  onClose: () => void;
  /** Receives the server's note when the PR was already tracked, so the host can say so. */
  onDone: (notice?: string) => void;
}

/** "Track a PR by URL": one field, inline errors (invalid URL, not found, already tracked). */
export function TrackPrDialog({ onAttach, onClose, onDone }: TrackDialogProps) {
  const [url, setUrl] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const { busy, failed, submit } = useDialogAction(() => onAttach(url.trim()), onDone);
  return (
    <DialogFrame title="Track a pull request by URL" onClose={onClose} testId="pr-track-dialog" busy={busy} initialFocus={inputRef} onSubmit={(e) => { e.preventDefault(); if (url.trim()) void submit(); }}>
      <label className={s.field}>
        Pull request URL
        <input ref={inputRef} className={s.input} type="url" inputMode="url" autoComplete="off" placeholder="https://github.com/owner/repo/pull/12" value={url} disabled={busy} onChange={(e) => setUrl(e.target.value)} />
      </label>
      {failed && <ErrorLine message={URL_ERRORS[failed.kind] ?? failed.message} />}
      <div className={s.actions}>
        <button type="button" className={s.button} onClick={onClose} disabled={busy}>{/* analytics-exempt */}Cancel</button>
        <button type="submit" className={`${s.button} ${s.primaryButton}`} disabled={busy || !url.trim()} aria-busy={busy}>
          {busy ? "Tracking..." : "Track"}
        </button>
      </div>
    </DialogFrame>
  );
}
