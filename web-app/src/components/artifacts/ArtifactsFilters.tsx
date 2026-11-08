"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useDialogFocusTrap } from "@/lib/hooks/useDialogFocusTrap";
import { activeFilterCount, ARTIFACT_TYPE_OPTIONS, PR_STATE_OPTIONS, type BrowseParams } from "@/lib/artifacts/browseQuery";
import * as s from "./ArtifactsBrowser.css";

interface Props {
  params: BrowseParams;
  compact: boolean;
  onChange: (patch: Partial<BrowseParams>) => void;
  onClear: () => void;
}

function Fields({ params, onChange }: Pick<Props, "params" | "onChange">) {
  const [repo, setRepo] = useState(params.repo);
  useEffect(() => { setRepo(params.repo); }, [params.repo]);
  const commitRepo = () => { if (repo.trim() !== params.repo) onChange({ repo: repo.trim() }); };
  return (
    <>
      <label className={s.filterField}>Type
        <select className={s.select} value={params.type} onChange={(e) => onChange({ type: e.target.value })} data-testid="artifacts-filter-type">
          <option value="">All</option>
          {ARTIFACT_TYPE_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </label>
      <label className={s.filterField}>Repo
        <input
          className={s.input}
          value={repo}
          placeholder="owner/repo"
          onChange={(e) => setRepo(e.target.value)}
          onBlur={commitRepo}
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); commitRepo(); } }}
          data-testid="artifacts-filter-repo"
        />
      </label>
      <label className={s.filterField}>PR state
        <select className={s.select} value={params.state} onChange={(e) => onChange({ state: e.target.value })} data-testid="artifacts-filter-state">
          <option value="">All</option>
          {PR_STATE_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </label>
    </>
  );
}

function Sheet({ onClose, onClear, children }: { onClose: () => void; onClear: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const doneRef = useRef<HTMLButtonElement>(null);
  useDialogFocusTrap({ dialogRef: ref, initialFocusRef: doneRef, onEscape: onClose });
  return (
    <div className={s.sheetBackdrop}>
      <div ref={ref} className={s.sheet} role="dialog" aria-modal="true" aria-label="Filters" data-testid="artifacts-filter-sheet">
        {children}
        <div className={s.sheetActions}>
          <button type="button" className={s.button} onClick={onClear} data-testid="artifacts-filter-clear">{/* analytics-exempt */}Clear</button>
          <button ref={doneRef} type="button" className={`${s.button} ${s.primaryButton}`} onClick={onClose} data-testid="artifacts-filter-done">{/* analytics-exempt */}Done</button>
        </div>
      </div>
    </div>
  );
}

/** Desktop: inline filter bar. Mobile: a Filters button that opens a sheet (design/ux.md S8). */
export function ArtifactsFilters({ params, compact, onChange, onClear }: Props) {
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  // Safari does not focus a button on click, so the trap's own restore can miss it: refocus explicitly.
  useEffect(() => {
    if (wasOpen.current && !open) buttonRef.current?.focus();
    wasOpen.current = open;
  }, [open]);
  const count = activeFilterCount(params);
  if (compact) {
    return (
      <div>
        <button ref={buttonRef} type="button" className={s.button} aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(true)} data-testid="artifacts-filters-button">{/* analytics-exempt */}
          Filters{count > 0 ? ` (${count})` : ""}
        </button>
        {open && <Sheet onClose={() => setOpen(false)} onClear={onClear}><Fields params={params} onChange={onChange} /></Sheet>}
      </div>
    );
  }
  return (
    <div className={s.filterBar} data-testid="artifacts-filter-bar">
      <Fields params={params} onChange={onChange} />
      {count > 0 && <button type="button" className={s.button} onClick={onClear} data-testid="artifacts-filter-clear">{/* analytics-exempt */}Clear filters</button>}
    </div>
  );
}
