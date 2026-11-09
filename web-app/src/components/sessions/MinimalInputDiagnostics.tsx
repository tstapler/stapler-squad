"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useSearchParams } from "@/lib/nav";
import { readBuildId } from "@/lib/terminal/mobileInput/buildId";
import {
  buildReport,
  inputDiagnostics,
  type Diagnostics,
  type ReportHeader,
} from "@/lib/terminal/mobileInput/diagnostics";
import * as css from "./MinimalInputDiagnostics.css";
import { touchButton } from "./MobileInputChrome.css";

/** True when the page was opened with `?inputdebug=1`. */
export function useInputDebugRequested(): boolean {
  return useSearchParams().get("inputdebug") === "1";
}

const EMPTY: never[] = [];

export interface MinimalInputDiagnosticsProps {
  header: Omit<ReportHeader, "buildId">;
  /** Defaults to the entry script file name. */
  buildId?: string;
  /** Ring to read; defaults to the process-wide one the adapter pushes to. */
  diagnostics?: Diagnostics;
  /** Called after a successful copy so the chrome can announce "Report copied". */
  onCopied?: () => void;
}

/** Read-only last-50-events panel (Story 2.3.1). Opening it starts the ring; closing it stops and empties it. */
export function MinimalInputDiagnostics({ header, buildId = readBuildId(), diagnostics = inputDiagnostics, onCopied }: MinimalInputDiagnosticsProps) {
  const requested = useInputDebugRequested();
  const [dismissed, setDismissed] = useState(false);
  const open = requested && !dismissed;
  useEffect(() => {
    if (!open) return;
    diagnostics.open();
    return () => diagnostics.close();
  }, [open, diagnostics]);
  const subscribe = useCallback((fn: () => void) => diagnostics.subscribe(fn), [diagnostics]);
  const events = useSyncExternalStore(subscribe, () => diagnostics.displayEvents(), () => EMPTY);
  const [fallbackReport, setFallbackReport] = useState<string | null>(null);

  if (!open) return null;

  const copy = async () => {
    const json = JSON.stringify(buildReport({ ...header, buildId }, diagnostics.displayEvents()), null, 2);
    try {
      await navigator.clipboard.writeText(json);
      setFallbackReport(null);
      onCopied?.();
    } catch {
      setFallbackReport(json); // clipboard blocked: show it selectable instead
    }
  };

  return (
    <div role="dialog" aria-labelledby="input-diagnostics-title" className={css.panel} data-testid="input-diagnostics">
      <div className={css.header}>
        <h2 id="input-diagnostics-title" className={css.title}>Input diagnostics</h2>
        <button type="button" className={touchButton} onClick={() => setDismissed(true)} aria-label="Close input diagnostics">
          {/* analytics-exempt */}
          Close
        </button>
      </div>
      <dl className={css.meta} data-testid="input-diagnostics-header">
        <dt>Build</dt><dd>{buildId}</dd>
        <dt>Renderer</dt><dd>{header.renderer}</dd>
        <dt>Layer flag</dt><dd>{header.flag}</dd>
        <dt>Stream</dt><dd>{header.streamState}</dd>
        <dt>Keyboard</dt><dd>{header.keyboardHint}</dd>
      </dl>
      <ul className={css.events} aria-label="Last input events">
        {events.map((e, i) => (
          <li key={`${e.t}-${i}`} className={css.eventRow}>
            <span>{e.t}</span>
            <span>{e.source}</span>
            <span>{`${e.cls}(${e.length})`}</span>
          </li>
        ))}
        {events.length === 0 && <li className={css.eventRow}>No events yet</li>}
      </ul>
      <div className={css.actions}>
        <button type="button" className={touchButton} onClick={copy}>
          {/* analytics-exempt */}
          Copy report
        </button>
      </div>
      {fallbackReport !== null && (
        <textarea
          readOnly
          aria-label="Input diagnostics report"
          className={css.reportBox}
          value={fallbackReport}
          onFocus={(e) => e.currentTarget.select()}
        />
      )}
    </div>
  );
}
