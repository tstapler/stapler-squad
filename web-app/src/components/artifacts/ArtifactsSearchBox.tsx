"use client";

import { useEffect, useId, useState, type FormEvent, type KeyboardEvent } from "react";
import { checkQuery, SHA_HINT } from "@/lib/artifacts/browseQuery";
import * as s from "./ArtifactsBrowser.css";

interface Props {
  /** The query in the URL. */
  value: string;
  slow: boolean;
  onSearch: (q: string) => void;
  onClear: () => void;
}

/** Submit-driven search. 2+ characters run; 1 is rejected inline and sends nothing (design/ux.md S8). */
export function ArtifactsSearchBox({ value, slow, onSearch, onClear }: Props) {
  const [draft, setDraft] = useState(value);
  const [submittedShort, setSubmittedShort] = useState(false);
  const descId = useId();
  useEffect(() => { setDraft(value); }, [value]);

  const check = checkQuery(draft);
  const tooShort = check.kind === "too_short";
  const showInvalid = tooShort && (submittedShort || [...draft.trim()].length === 1);
  const showHint = check.kind === "ok" && check.shaHint;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (check.kind === "empty") { setSubmittedShort(false); if (value) onClear(); return; }
    if (tooShort) { setSubmittedShort(true); return; }
    setSubmittedShort(false);
    onSearch(draft.trim());
  };
  const clear = () => { setDraft(""); setSubmittedShort(false); onClear(); };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Escape" && (draft || value)) { e.preventDefault(); clear(); }
  };

  return (
    <form className={s.searchForm} onSubmit={submit} role="search" aria-label="Search artifacts" data-testid="artifacts-search-form">
      <div className={s.searchField}>
        <input
          type="search"
          className={s.input}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Search artifacts: PR, commit, branch, issue, text..."
          aria-label="Search artifacts"
          aria-invalid={showInvalid}
          aria-describedby={showInvalid || showHint ? descId : undefined}
          data-testid="artifacts-search"
        />
        {showInvalid && <p id={descId} className={s.invalid} data-testid="artifacts-search-invalid">{check.kind === "too_short" ? check.message : ""}</p>}
        {showHint && <p id={descId} className={s.hint} data-testid="artifacts-search-sha-hint">{SHA_HINT}</p>}
      </div>
      <button type="submit" className={`${s.button} ${s.primaryButton}`} data-testid="artifacts-search-submit">{/* analytics-exempt */}Search</button>
      {slow && <span className={s.statusLine} aria-hidden="true"><span className={s.spinner} /></span>}
      {value && <button type="button" className={s.button} onClick={clear} data-testid="artifacts-clear-search">{/* analytics-exempt */}Clear search</button>}
    </form>
  );
}
