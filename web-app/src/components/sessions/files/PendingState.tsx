"use client";

import { useEffect, useState } from "react";
import type { FailureKind } from "@/lib/files/bufferReducer";
import { realClock, type Clock } from "@/lib/files/analytics/sampling";
import type { BannerSpec } from "./BannerStack";
import { actionButton, actions as actionsClass, failureText, spinner, wait } from "./PendingState.css";

/** Any wait longer than this shows a visible, announced pending state (ux.md 8.5). */
export const PENDING_MS = 150;

/** True once `active` has stayed true for `delayMs`; resets the moment it ends. */
export function usePendingDelay(active: boolean, delayMs = PENDING_MS, clock: Clock = realClock): boolean {
  const [elapsed, setElapsed] = useState(false);
  useEffect(() => {
    if (!active) {
      setElapsed(false);
      return;
    }
    const id = clock.setTimeout(() => setElapsed(true), delayMs);
    return () => clock.clearTimeout(id);
  }, [active, delayMs, clock]);
  return active && elapsed;
}

export type WaitKind = "open" | "tree" | "restore" | "outline" | "stat" | "merge";

const CLASS_TEXT: Record<FailureKind, string> = {
  not_found: "not found",
  too_large: "too large",
  binary: "binary file",
  encoding: "unsupported encoding",
  worktree_unavailable: "worktree unavailable",
  rpc_error: "network error",
};

/** Words for a failure class; never a path or raw error text. */
export const failureClass = (reason: FailureKind) => CLASS_TEXT[reason];

interface WaitContext { name?: string; count?: number }

const PENDING_COPY: Record<WaitKind, (c: WaitContext) => string> = {
  open: (c) => `Opening ${c.name ?? "file"}...`,
  tree: () => "Loading files...",
  restore: (c) => `Restoring ${c.count ?? 0} tabs`,
  outline: () => "Reading symbols...",
  stat: () => "Checking the file on disk...",
  merge: () => "Loading merge view...",
};

const FAILURE_COPY: Record<WaitKind, (c: WaitContext, reason: FailureKind) => string> = {
  open: (c, r) => `Could not open ${c.name ?? "file"}: ${failureClass(r)}`,
  tree: (_c, r) => `Could not load files: ${failureClass(r)}`,
  restore: (_c, r) => `Could not restore: ${failureClass(r)}`,
  outline: () => "Could not read symbols",
  stat: (_c, r) => `Could not check the file: ${failureClass(r)}`,
  merge: () => "Merge view unavailable - try again",
};

export interface WaitAction { label: string; onClick: () => void }

interface WaitStateProps extends WaitContext {
  kind: WaitKind;
  /** Present once the wait failed; every failure carries at least one way out. */
  failure?: { reason: FailureKind; actions: WaitAction[] };
}

/** The pending or failure panel for one of the waits of ux.md 8.5. */
export function WaitState({ kind, name, count, failure }: WaitStateProps) {
  const ctx = { name, count };
  if (failure) {
    return (
      <div className={wait} role="alert" data-testid={`files-failure-${kind}`}>
        <span className={failureText}>{FAILURE_COPY[kind](ctx, failure.reason)}</span>
        <div className={actionsClass}>
          {failure.actions.map((a) => (
            <button key={a.label} type="button" className={actionButton} onClick={a.onClick}>{a.label}</button>
          ))}
        </div>
      </div>
    );
  }
  return (
    <div className={wait} role="status" aria-live="polite" data-testid={`files-pending-${kind}`}>
      <span className={spinner} aria-hidden="true" />
      <span>{PENDING_COPY[kind](ctx)}</span>
    </div>
  );
}

interface ReloadFailure {
  path: string;
  reason: FailureKind;
  dirty: boolean;
  seq: number;
  onRetry: () => void;
  onCopy: () => void;
  onKeep: () => void;
}

/** A failed reload never touches the buffer; with unsaved edits it is blocking (priority 1), else priority 8. */
export function reloadFailedBanner(f: ReloadFailure): BannerSpec {
  return {
    id: `reload-failed:${f.path}`,
    kind: f.dirty ? "reload_failed_dirty" : "reload_failed",
    seq: f.seq,
    body: `Could not reload: ${failureClass(f.reason)}`,
    actions: [
      { label: "Retry", onClick: f.onRetry },
      ...(f.dirty ? [{ label: "Copy my version", onClick: f.onCopy }] : []),
      { label: "Keep editing", onClick: f.onKeep },
    ],
  };
}
