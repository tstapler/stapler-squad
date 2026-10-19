"use client";

import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { ACCESSIBLE_PAGE_SIZE, pageFromLines, paginateLines, pageOfLine, textPageSource, type Page, type PageSource } from "@/lib/files/pageSource";
import { useWindowedState } from "@/lib/files/useWindowedState";
import type { WindowedFile } from "@/lib/files/windowedFile";
import type { ReadViewHandle } from "./readViewHandle";
import { region, pager, control, gotoInput, pageLabel, unsaved, page as pageClass, line as lineClass, skipLink } from "./AccessibleView.css";
import { visuallyHidden } from "@/styles/a11y.css";

const TOUCH = { minHeight: 44, minWidth: 44 } as const;
const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

export interface AccessibleViewProps {
  path: string;
  /** Text of a buffer read whole; pages are cut from it with no further call. */
  source?: PageSource;
  /** A windowed buffer: each page is one `read(lines)` through the transport. */
  windowed?: WindowedFile;
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
type Remote = { page: number; status: "loading" | "failed" } | { page: number; status: "ready"; lines: string[] };

const EMPTY_SOURCE = textPageSource("");

/** Pages of a windowed file: one transport read per page, with loading and failed states. */
function useRemotePage(windowed: WindowedFile | undefined, pageIndex: number) {
  const state = useWindowedState(windowed);
  const [remote, setRemote] = useState<Remote | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (!windowed) return;
    const ctl = new AbortController();
    setRemote({ page: pageIndex, status: "loading" });
    windowed.pageLines(pageIndex * ACCESSIBLE_PAGE_SIZE + 1, ACCESSIBLE_PAGE_SIZE, ctl.signal).then(
      (lines) => setRemote({ page: pageIndex, status: "ready", lines }),
      () => { if (!ctl.signal.aborted) setRemote({ page: pageIndex, status: "failed" }); },
    );
    return () => ctl.abort();
  }, [windowed, pageIndex, attempt]);
  const here = remote?.page === pageIndex ? remote : null;
  const lines = here?.status === "ready" ? here.lines : [];
  return { total: state.lineCount, page: pageFromLines(lines, pageIndex, state.lineCount, state.exact), status: here?.status ?? "loading", retry: () => setAttempt((a) => a + 1) };
}

/** Plain, fully rendered, read-only pages of 500 numbered lines: the guaranteed screen-reader read path (ux.md 8.10). */
export function AccessibleView({ path, source = EMPTY_SOURCE, windowed, dirty = false, onSave, saveDisabledReason, onEditRequested, onReady }: AccessibleViewProps) {
  const [pageIndex, setPageIndex] = useState(0);
  const [tick, setTick] = useState(0);
  const [goto, setGoto] = useState("");
  const [announce, setAnnounce] = useState("");
  const [hitLine, setHitLine] = useState<number | null>(null);
  const pre = useRef<HTMLPreElement>(null);
  const pending = useRef<Pending>(null);
  const lastLine = useRef(1);
  const reasonId = useId();
  const remote = useRemotePage(windowed, pageIndex);
  const totalLines = windowed ? remote.total : source.lineCount;
  const current: Page = windowed ? remote.page : paginateLines(source, pageIndex);
  const pageReady = !windowed || remote.status === "ready";
  // An estimated count can overshoot; once the exact one is known the page index follows it.
  const lastPage = current.pages - 1;
  useEffect(() => { if (windowed && pageIndex > lastPage) setPageIndex(lastPage); }, [windowed, pageIndex, lastPage]);

  const goToLine = useCallback((requested: number) => {
    if (!Number.isFinite(requested)) return;
    const n = Math.min(Math.max(1, Math.trunc(requested)), totalLines);
    pending.current = { kind: "line", line: n };
    setPageIndex(pageOfLine(n));
    setTick((t) => t + 1);
  }, [totalLines]);
  const changePage = (to: number) => { pending.current = { kind: "page" }; setPageIndex(to); };

  const failedFirst = pageIndex * ACCESSIBLE_PAGE_SIZE + 1;
  const failedText = `Couldn't load lines ${failedFirst}-${Math.min(failedFirst + ACCESSIBLE_PAGE_SIZE - 1, totalLines)}. Retry`;
  useEffect(() => {
    if (windowed && remote.status === "failed") setAnnounce(failedText);
  }, [windowed, remote.status, failedText]);

  useEffect(() => {
    if (!pageReady) return;
    const p = pending.current;
    // A jump aimed at another page waits for that page (this run may be the old page finishing its load).
    if (p?.kind === "line" && pageOfLine(p.line) !== pageIndex) return;
    pending.current = null;
    if (!p) return;
    if (p.kind === "page") {
      pre.current?.focus();
      setAnnounce(current.label);
      return;
    }
    const line = windowed ? Math.min(p.line, current.last) : p.line;
    pre.current?.querySelector<HTMLElement>(`[data-line="${line}"]`)?.focus();
    lastLine.current = line;
    setAnnounce(`Line ${line}`);
    // Only a navigation (page or tick) may announce; `current.label` follows from pageIndex.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageIndex, tick, pageReady]);

  const latest = useRef({ goToLine, source });
  latest.current = { goToLine, source };
  const windowedRef = useRef(windowed);
  windowedRef.current = windowed;
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
      windowed: windowedRef.current,
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
            <input className={gotoInput} style={{ minHeight: 44 }} type="number" inputMode="numeric" min={1} max={totalLines} value={goto} onChange={(e) => setGoto(e.target.value)} />
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
      {windowed && remote.status === "failed" && (
        <button type="button" className={control} style={TOUCH} onClick={remote.retry}>Retry</button>
      )}
      <pre
        ref={pre}
        aria-busy={windowed && remote.status === "loading" ? true : undefined}
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
