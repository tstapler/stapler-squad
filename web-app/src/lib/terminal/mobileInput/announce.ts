/**
 * Live-region contract (Story 1.4.4). One `announce(kind, text, dedupeKey)` routes each message to the
 * polite (`role="status"`) or assertive (`role="alert"`) region named by ANNOUNCE_KIND_REGION. Chips,
 * hints and notices carry no role of their own. This is separate from the app-wide Announcer
 * (`lib/hooks/useAnnounce`), which queues one message at a time; here a newer message replaces the older.
 */
import type { Diagnostics } from "./diagnostics";

export type Region = "polite" | "assertive";

/** Assertive only when typed content was lost or may be incomplete. */
export const ANNOUNCE_KIND_REGION = {
  "chip-disconnected": "polite",
  "chip-reconnecting": "polite",
  "chip-reconnected": "polite",
  "chip-still-disconnected": "polite",
  "chip-keys-not-sent": "polite",
  "read-only-hint": "polite",
  "flag-off-hint": "polite",
  "paste-flattened-hint": "polite",
  "ime-fallback-notice": "polite",
  "compose-offer": "polite",
  "deferred-commit": "polite",
  "report-copied": "polite",
  "committed-text": "polite",
  "discard-overflow": "assertive",
  "discard-ttl": "assertive",
  "discard-session-change": "assertive",
  "sending-stuck": "assertive",
} as const satisfies Record<string, Region>;

export type AnnounceKind = keyof typeof ANNOUNCE_KIND_REGION;

export interface RegionSink {
  write(region: Region, text: string): void;
}

export interface Announcer {
  announce(kind: AnnounceKind, text: string, dedupeKey: string): void;
  clear(region: Region): void;
}

export function createAnnouncer(sink: RegionSink): Announcer {
  const keys: Record<Region, string | null> = { polite: null, assertive: null };
  return {
    announce(kind, text, dedupeKey) {
      const region: Region = ANNOUNCE_KIND_REGION[kind];
      if (keys[region] === dedupeKey) return;
      // Clear first so a screen reader treats an identical-looking replacement as a change.
      if (keys[region] !== null) sink.write(region, "");
      keys[region] = dedupeKey;
      sink.write(region, text);
    },
    clear(region) {
      if (keys[region] === null) return;
      keys[region] = null;
      sink.write(region, "");
    },
  };
}

export const COMMIT_ANNOUNCE_CLEAR_MS = 1000;

export interface CommitAnnouncerDeps {
  announcer: Announcer;
  diagnostics: Pick<Diagnostics, "pushInput">;
  isReadOnly: () => boolean;
  schedule?: (fn: () => void, ms: number) => unknown;
  cancel?: (handle: unknown) => void;
  now?: () => number;
}

export interface CommitAnnouncer {
  /** `text` is spoken once; the ring keeps only its class and length, and nothing is logged. */
  onCommit(text: string): void;
}

/** TalkBack typed-character echo: the committed word goes to the polite region once and clears after 1 s. */
export function createCommitAnnouncer(deps: CommitAnnouncerDeps): CommitAnnouncer {
  const schedule = deps.schedule ?? ((fn, ms) => setTimeout(fn, ms));
  const cancel = deps.cancel ?? ((h) => clearTimeout(h as ReturnType<typeof setTimeout>));
  const now = deps.now ?? Date.now;
  let seq = 0;
  let pending: unknown = null;
  return {
    onCommit(text) {
      const word = text.trim();
      deps.diagnostics.pushInput("emit", word, now());
      if (word === "" || deps.isReadOnly()) return;
      deps.announcer.announce("committed-text", word, `commit:${++seq}`);
      if (pending !== null) cancel(pending);
      pending = schedule(() => {
        pending = null;
        deps.announcer.clear("polite");
      }, COMMIT_ANNOUNCE_CLEAR_MS);
    },
  };
}
