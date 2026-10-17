"use client";

import { useEffect, useRef, useState } from "react";
import type { ReadOnlyKind } from "@/lib/files/bufferReducer";
import type { FeedbackMessage } from "@/lib/hooks/useFilesFeedback";
import { bar, chip, dirtyDot, errorText, popover, region, segment } from "./StatusBar.css";

const REASONS: Record<Exclude<ReadOnlyKind, "none">, { chip: string; why: string }> = {
  flag_off: { chip: "editing is turned off", why: "Editing is turned off for this installation, so the file can only be read." },
  binary: { chip: "binary file", why: "Binary files are shown as a placeholder and cannot be edited." },
  truncated: { chip: "showing the first part only", why: "Only the first part of this file is loaded, so it cannot be edited." },
  too_large: { chip: "file too large to edit", why: "This file is larger than the editable limit and cannot be edited." },
  non_utf8: { chip: "unsupported encoding", why: "This file is not UTF-8 text and cannot be edited without corrupting it." },
  mixed_eol: { chip: "mixed line endings", why: "This file mixes line endings, so saving could rewrite them; it cannot be edited." },
  symlink: { chip: "symbolic link", why: "Symbolic links are not edited through the viewer." },
  worktree_missing: { chip: "worktree unavailable", why: "The session's worktree is not on disk (the session may be paused), so the file cannot be edited." },
  hidden_session: { chip: "session is hidden", why: "Hidden sessions cannot be edited." },
  git_internal: { chip: "git internals", why: "Files inside .git cannot be edited." },
};

export function readOnlyChipText(reason: Exclude<ReadOnlyKind, "none">, detail?: string): string {
  return `Read-only: ${detail ? `showing ${detail}` : REASONS[reason].chip}`;
}

interface StatusBarProps {
  line: number;
  col: number;
  eol: "LF" | "CRLF" | "mixed" | null;
  encoding: string | null;
  language: string;
  readOnly: ReadOnlyKind;
  /** Tail of the truncated chip, e.g. "first 1 MB of 3.2 MB". */
  readOnlyDetail?: string;
  dirty: boolean;
  offline?: boolean;
  message: FeedbackMessage | null;
  onDismissMessage?: () => void;
  onPause?: () => void;
  onResume?: () => void;
}

/** Position, format and state of the active buffer, plus the polite region for success and error messages. */
export function StatusBar(p: StatusBarProps) {
  const [open, setOpen] = useState(false);
  const chipRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      setOpen(false);
      chipRef.current?.focus();
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [open]);

  return (
    <div className={bar} data-testid="files-status-bar" style={{ position: "relative" }}>
      <span className={segment}>Ln {p.line}, Col {p.col}</span>
      {p.eol && <span className={segment}>{p.eol}</span>}
      {p.encoding && <span className={segment}>{p.encoding}</span>}
      <span className={segment}>{p.language}</span>
      {p.readOnly !== "none" && (
        <button ref={chipRef} type="button" className={chip} aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {readOnlyChipText(p.readOnly, p.readOnlyDetail)}
        </button>
      )}
      {open && p.readOnly !== "none" && <div className={popover} role="note">{REASONS[p.readOnly].why}</div>}
      {p.dirty && <span className={`${segment} ${dirtyDot}`}>● Modified</span>}
      {p.offline && <span className={segment}>Offline</span>}
      <div className={region} onMouseEnter={p.onPause} onMouseLeave={p.onResume} onFocus={p.onPause} onBlur={p.onResume}>
        <span role="status" aria-live="polite" data-testid="files-status-message" className={p.message?.kind === "error" ? errorText : undefined}>
          {p.message?.text}
        </span>
        {p.message && p.onDismissMessage && (
          <button type="button" className={chip} aria-label="Dismiss message" onClick={p.onDismissMessage}>×</button>
        )}
      </div>
    </div>
  );
}
