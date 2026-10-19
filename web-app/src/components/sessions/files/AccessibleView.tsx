"use client";

import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { paginateLines, pageOfLine, type PageSource } from "@/lib/files/pageSource";
import type { ReadViewHandle } from "./readViewHandle";
import { region, pager, control, gotoInput, pageLabel, unsaved, page as pageClass, line as lineClass, skipLink } from "./AccessibleView.css";
import { visuallyHidden } from "@/styles/a11y.css";

const TOUCH = { minHeight: 44, minWidth: 44 } as const;
const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

export interface AccessibleViewProps {
  path: string;
  source: PageSource;
  /** The buffer has unsaved edits: the view shows the draft and says so. */
  dirty?: boolean;
  /** Present when saving is possible; otherwise Save stays disabled with `saveDisabledReason`. */
  onSave?: () => void;
  saveDisabledReason?: string;
  /** Seam for Task 2.2.1b: when passed, a 44 px Edit button calls it with the line last focused. */
  onEditRequested?: (line: number) => void;
  /** Gives the editor bridge the same narrow handle CodeMirror gives it. */
  onReady?: (handle: ReadViewHandle) => void;
}

type Pending = { kind: "line"; line: number } | { kind: "page" } | null;

/** Plain, fully rendered, read-only pages of 500 numbered lines: the guaranteed screen-reader read path (ux.md 8.10). */
export function AccessibleView({ path, source, dirty = false, onSave, saveDisabledReason, onEditRequested, onReady }: AccessibleViewProps) {
  const [pageIndex, setPageIndex] = useState(0);
  const [tick, setTick] = useState(0);
  const [goto, setGoto] = useState("");
  const [announce, setAnnounce] = useState("");
  const [hitLine, setHitLine] = useState<number | null>(null);
  const pre = useRef<HTMLPreElement>(null);
  const pending = useRef<Pending>(null);
  const lastLine = useRef(1);
  const reasonId = useId();
  const current = paginateLines(source, pageIndex);

  const goToLine = useCallback((requested: number) => {
    if (!Number.isFinite(requested)) return;
    const n = Math.min(Math.max(1, Math.trunc(requested)), source.lineCount);
    pending.current = { kind: "line", line: n };
    setPageIndex(pageOfLine(n));
    setTick((t) => t + 1);
  }, [source]);
  const changePage = (to: number) => { pending.current = { kind: "page" }; setPageIndex(to); };

  useEffect(() => {
    const p = pending.current;
    pending.current = null;
    if (!p) return;
    if (p.kind === "page") {
      pre.current?.focus();
      setAnnounce(current.label);
      return;
    }
    pre.current?.querySelector<HTMLElement>(`[data-line="${p.line}"]`)?.focus();
    lastLine.current = p.line;
    setAnnounce(`Line ${p.line}`);
    // Only a navigation (page or tick) may announce; `current.label` follows from pageIndex.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageIndex, tick]);

  const latest = useRef({ goToLine, source });
  latest.current = { goToLine, source };
  const readyRef = useRef(onReady);
  readyRef.current = onReady;
  useEffect(() => {
    readyRef.current?.({
      getText: () => latest.current.source.text(),
      getCursor: () => ({ line: lastLine.current, col: 0 }),
      gotoLine: (n) => latest.current.goToLine(n),
      highlightRange: (r) => setHitLine(r ? r.line : null),
      scrollSnapshot: () => ({ kind: "scroll-snapshot" }),
      restoreScroll: () => undefined,
      focus: () => pre.current?.focus(),
    });
  }, []);

  const submitGoto = (e: FormEvent) => { e.preventDefault(); goToLine(Number.parseInt(goto, 10)); };
  const saveBlocked = !onSave;
  const name = `${baseName(path)}, accessible view${dirty ? ", unsaved changes" : ""}`;

  return (
    <section className={region} aria-label={name} data-testid="accessible-view">
      <nav className={pager} aria-label="Pager">
        <button type="button" className={control} style={TOUCH} aria-disabled={pageIndex === 0} onClick={() => pageIndex > 0 && changePage(pageIndex - 1)}>Previous page</button>
        <span className={pageLabel} data-testid="accessible-view-label">{current.label}</span>
        <button type="button" className={control} style={TOUCH} aria-disabled={pageIndex >= current.pages - 1} onClick={() => pageIndex < current.pages - 1 && changePage(pageIndex + 1)}>Next page</button>
        <form onSubmit={submitGoto}>
          <label>
            Go to line
            <input className={gotoInput} style={{ minHeight: 44 }} type="number" inputMode="numeric" min={1} max={source.lineCount} value={goto} onChange={(e) => setGoto(e.target.value)} />
          </label>
          <button type="submit" className={control} style={TOUCH}>Go</button>
        </form>
        {dirty && <span className={unsaved}>Unsaved changes</span>}
        {dirty && (
          <>
            <button type="button" className={control} style={TOUCH} aria-disabled={saveBlocked} aria-describedby={saveBlocked ? reasonId : undefined} onClick={() => onSave?.()}>Save</button>
            {saveBlocked && <span id={reasonId} className={visuallyHidden}>{saveDisabledReason ?? "saving is not available"}</span>}
          </>
        )}
        {onEditRequested && <button type="button" className={control} style={TOUCH} onClick={() => onEditRequested(lastLine.current)}>Edit</button>}
      </nav>
      <div className={visuallyHidden} role="status" aria-live="polite" data-testid="accessible-view-announce">{announce}</div>
      <pre
        ref={pre}
        className={pageClass}
        tabIndex={0}
        aria-label={current.label}
        data-testid="accessible-view-page"
        onFocus={(e) => { const n = Number((e.target as HTMLElement).dataset.line); if (n) lastLine.current = n; }}
      >
        {current.lines.map((text, i) => {
          const n = current.first + i;
          return (
            <span key={n} className={lineClass} tabIndex={-1} data-line={n} data-hit={n === hitLine || undefined} data-testid="accessible-view-line">{`${n} ${text}`}</span>
          );
        })}
      </pre>
    </section>
  );
}

/** First focusable element of the editor region: jumps between the two read views. */
export function AccessibleSkipLink({ on, onToggle }: { on: boolean; onToggle: () => void }) {
  return (
    <button type="button" className={skipLink} data-testid="accessible-skip-link" onClick={onToggle}>
      {on ? "Switch to code editor" : "Switch to accessible view"}
    </button>
  );
}
