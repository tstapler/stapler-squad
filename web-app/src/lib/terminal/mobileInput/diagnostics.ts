/**
 * Content-free input diagnostics (Story 4.1.1). The ring stores class and length only: `pushInput` reads
 * the data once to classify it and never keeps it, so nothing downstream (report, console, telemetry)
 * can leak typed text. "Reveal text" lives in the separate `RevealBuffer`, which the report never sees.
 */

export const DIAGNOSTICS_RING_CAPACITY = 200;
export const DIAGNOSTICS_DISPLAY_COUNT = 50;

export type InputDataClass = "text" | "control" | "escape" | "space" | "empty";
export type DiagnosticSource = "beforeinput" | "input" | "keydown" | "composition" | "emit" | "paste" | "other";

export interface DiagnosticsEvent {
  t: number;
  source: DiagnosticSource;
  cls: InputDataClass;
  length: number;
}

export interface ReportHeader {
  renderer: string;
  flag: string;
  streamState: string;
  keyboardHint: string;
  buildId?: string;
}

export interface DiagnosticsReport {
  version: 1;
  header: ReportHeader;
  events: Array<DiagnosticsEvent & { label: string }>;
}

export interface ReportSkeleton {
  eventCount: number;
  byClass: Partial<Record<InputDataClass, number>>;
  totalLength: number;
  spanMs: number;
}

/** Class and code-point length of `data`; the string itself is not returned. */
export function classifyInput(data: string): { cls: InputDataClass; length: number } {
  const length = [...data].length;
  if (length === 0) return { cls: "empty", length };
  if (data.startsWith("\x1b")) return { cls: "escape", length };
  if (/^[\x00-\x1f\x7f]+$/.test(data)) return { cls: "control", length };
  if (/^\s+$/.test(data)) return { cls: "space", length };
  return { cls: "text", length };
}

export interface Diagnostics {
  readonly length: number;
  readonly isOpen: boolean;
  open(): void;
  close(): void;
  pushInput(source: DiagnosticSource, data: string, t: number): void;
  displayEvents(): DiagnosticsEvent[];
  subscribe(listener: () => void): () => void;
}

export function createDiagnostics(): Diagnostics {
  let events: DiagnosticsEvent[] = [];
  // Stable reference between mutations, as useSyncExternalStore requires.
  let shown: DiagnosticsEvent[] | null = null;
  let open = false;
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((l) => l());
  return {
    get length() {
      return events.length;
    },
    get isOpen() {
      return open;
    },
    open() {
      open = true;
      notify();
    },
    close() {
      open = false;
      events = [];
      shown = null;
      notify();
    },
    pushInput(source, data, t) {
      if (!open) return;
      events.push({ t, source, ...classifyInput(data) });
      if (events.length > DIAGNOSTICS_RING_CAPACITY) events = events.slice(-DIAGNOSTICS_RING_CAPACITY);
      shown = null;
      notify();
    },
    displayEvents() {
      shown ??= events.slice(-DIAGNOSTICS_DISPLAY_COUNT);
      return shown;
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

/** Process-wide ring the adapter pushes to; a no-op until a panel opens it. */
export const inputDiagnostics: Diagnostics = createDiagnostics();

export function buildReport(header: ReportHeader, events: readonly DiagnosticsEvent[]): DiagnosticsReport {
  return {
    version: 1,
    header,
    events: events.map((e) => ({ ...e, label: `${e.cls}(${e.length})` })),
  };
}

/** Shape summary for triage. The report cannot become a replayable trace (captures do that, Story 4.1.3). */
export function reportToSkeleton(report: DiagnosticsReport): ReportSkeleton {
  const byClass: Partial<Record<InputDataClass, number>> = {};
  let totalLength = 0;
  for (const e of report.events) {
    byClass[e.cls] = (byClass[e.cls] ?? 0) + 1;
    totalLength += e.length;
  }
  const times = report.events.map((e) => e.t);
  const spanMs = times.length ? Math.max(...times) - Math.min(...times) : 0;
  return { eventCount: report.events.length, byClass, totalLength, spanMs };
}

/** In-memory "Reveal text" content: off by default, never serialized, cleared on reveal-off and close. */
export class RevealBuffer {
  private text = "";
  private on = false;
  get enabled(): boolean {
    return this.on;
  }
  setEnabled(on: boolean): void {
    this.on = on;
    if (!on) this.text = "";
  }
  append(s: string): void {
    if (this.on) this.text += s;
  }
  read(): string {
    return this.text;
  }
  close(): void {
    this.setEnabled(false);
  }
  toJSON(): null {
    return null;
  }
}
