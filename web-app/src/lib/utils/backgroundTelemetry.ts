import type { ActivityRow, ActivityState } from "@/lib/utils/backgroundActivity";

export const EMPTY_EVIDENCE_WINDOW_MS = 24 * 60 * 60 * 1000;

export type StateCounts = Partial<Record<ActivityState, number>>;

export function stateCounts(rows: readonly ActivityRow[]): StateCounts {
  const counts: StateCounts = {};
  for (const r of rows) counts[r.state] = (counts[r.state] ?? 0) + 1;
  return counts;
}

/** `labels` of a `/api/analytics` event are string-valued. */
export function stateCountLabels(counts: StateCounts): Record<string, string> {
  return Object.fromEntries(Object.entries(counts).map(([state, n]) => [state, String(n)]));
}

export interface EvidenceAlertState {
  /** When `finished == 0 && stopped_unknown > 0` first held, null while it does not. */
  since: number | null;
  fired: boolean;
}

export interface EmptyEvidenceAlertOptions {
  notify: () => void;
  initial?: EvidenceAlertState;
  onChange?: (state: EvidenceAlertState) => void;
  windowMs?: number;
}

/**
 * `finished == 0 while stopped_unknown > 0` for a full window raises one notification; the
 * first `finished` row re-arms it. Pure: the caller supplies the clock via `observe`.
 */
export function createEmptyEvidenceAlert({
  notify,
  initial = { since: null, fired: false },
  onChange,
  windowMs = EMPTY_EVIDENCE_WINDOW_MS,
}: EmptyEvidenceAlertOptions) {
  let state = initial;
  const set = (next: EvidenceAlertState) => {
    if (next.since === state.since && next.fired === state.fired) return;
    state = next;
    onChange?.(state);
  };
  return {
    observe(counts: StateCounts, nowMs: number): void {
      if ((counts.finished ?? 0) > 0 || (counts.stopped_unknown ?? 0) === 0) {
        set({ since: null, fired: false });
        return;
      }
      const since = state.since ?? nowMs;
      if (!state.fired && nowMs - since >= windowMs) {
        set({ since, fired: true });
        notify();
        return;
      }
      set({ since, fired: state.fired });
    },
  };
}
