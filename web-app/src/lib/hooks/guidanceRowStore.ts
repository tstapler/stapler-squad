import { create, type Message } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import {
  GetGuidanceRequestRequestSchema,
  GuidanceRequestService,
  ListAllPendingGuidanceRequestsRequestSchema,
} from "@/gen/session/v1/guidance_request_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { toPlainObject } from "@/lib/api/serialization";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import {
  isPageHidden,
  onPageVisibilityChange,
} from "@/lib/utils/pageVisibility";

export const GUIDANCE_POLL_MS = 3_000;
/** Rows that settled but are held fresh by an open card are re-read at this slower pace. */
const SETTLED_POLL_MS = 15_000;
/** Per-call deadline, shorter than the transport's, so a half-open connection cannot freeze rows. */
const POLL_TIMEOUT_MS = 10_000;
/** At most this many live rows are read individually; more use the one shared list call. */
const LIST_THRESHOLD = 2;

/** Delivery states that still change on their own: pending, sent (acknowledgement or the 60 s decision). */
const MOVING = new Set(["pending", "sent"]);

/** A row that can still change without the operator: open, or answered and not yet settled. */
export function isMoving(row: PlainGuidanceRequest | undefined): boolean {
  if (!row) return true;
  if (row.status === "pending") return true;
  return row.status === "answered" && MOVING.has(row.deliveryState ?? "");
}

interface Entry {
  row: PlainGuidanceRequest | undefined;
  listeners: Set<() => void>;
  /** Subscribers that asked for polling even once the row has settled. */
  keepFresh: number;
  /** When the row was last published, to pace reads of rows that are only held fresh. */
  lastRead: number;
}

const entries = new Map<string, Entry>();
let timer: ReturnType<typeof setInterval> | undefined;
let stopVisibility: (() => void) | undefined;
let inFlight = false;
let rerunAfterFlight = false;

const isLive = (e: Entry) => e.keepFresh > 0 || isMoving(e.row);
const toPlain = (r: Message) =>
  toPlainObject(r) as unknown as PlainGuidanceRequest;

function publish(id: string, row: PlainGuidanceRequest): void {
  const e = entries.get(id);
  if (!e) return;
  e.row = row;
  e.lastRead = Date.now();
  e.listeners.forEach((l) => l());
  syncTimer();
}

/**
 * One tick serves every subscribed row. With more than LIST_THRESHOLD live rows a single
 * ListAllPendingGuidanceRequests answers all open cards and only rows missing from it (answered, awaiting
 * acknowledgement) get an individual read; with fewer, individual reads are cheaper than the global list.
 * If the list call fails the tick ends (no 1+N fallback against a struggling server). Settled rows held
 * fresh by a subscriber are re-read only every SETTLED_POLL_MS. A tick is skipped while the previous one is
 * in flight or the tab is hidden.
 */
async function tick(urgent = false): Promise<void> {
  if (inFlight) {
    // A row that mounted mid-flight must not wait a whole interval for its first read.
    if (urgent) rerunAfterFlight = true;
    return;
  }
  if (isPageHidden()) return;
  const now = Date.now();
  const wanted = [...entries]
    .filter(([, e]) => isMoving(e.row) || (e.keepFresh > 0 && now - e.lastRead >= SETTLED_POLL_MS))
    .map(([id]) => id);
  if (wanted.length === 0) return;
  inFlight = true;
  try {
    const client = createClient(GuidanceRequestService, getConnectTransport());
    const seen = new Set<string>();
    if (wanted.length > LIST_THRESHOLD) {
      try {
        // One poll is shared by every row, so no single caller owns it.
        // abort-signal-exempt
        const res = await client.listAllPendingGuidanceRequests(
          create(ListAllPendingGuidanceRequestsRequestSchema, {}),
          { timeoutMs: POLL_TIMEOUT_MS },
        );
        for (const r of res.requests) {
          seen.add(r.id);
          if (entries.has(r.id)) publish(r.id, toPlain(r));
        }
      } catch {
        return; // keep the last rows and retry next tick
      }
    }
    await Promise.all(
      wanted
        .filter((id) => !seen.has(id))
        .map((id) =>
          Promise.resolve()
            // abort-signal-exempt
            .then(() =>
              client.getGuidanceRequest(
                create(GetGuidanceRequestRequestSchema, { id }),
                { timeoutMs: POLL_TIMEOUT_MS },
              ),
            )
            .then((res) => res.request && publish(id, toPlain(res.request)))
            .catch(() => {}),
        ),
    );
  } catch {
    // transport unavailable: keep the last rows and try again next tick
  } finally {
    inFlight = false;
    if (rerunAfterFlight) {
      rerunAfterFlight = false;
      void tick(true);
    }
  }
}

function syncTimer(): void {
  const anyLive = [...entries.values()].some(isLive);
  if (anyLive && timer === undefined) {
    timer = setInterval(() => void tick(), GUIDANCE_POLL_MS);
    stopVisibility = onPageVisibilityChange(() => {
      if (!isPageHidden()) void tick();
    });
  } else if (!anyLive && timer !== undefined) {
    stopPolling();
  }
}

function stopPolling(): void {
  if (timer !== undefined) clearInterval(timer);
  timer = undefined;
  stopVisibility?.();
  stopVisibility = undefined;
}

function entryFor(id: string): Entry {
  let e = entries.get(id);
  if (!e) {
    e = { row: undefined, listeners: new Set(), keepFresh: 0, lastRead: 0 };
    entries.set(id, e);
    // New ids are read on the next microtask so rows mounting together share one request.
    void Promise.resolve().then(() => tick(true));
  }
  return e;
}

function dropIfUnused(id: string, e: Entry): void {
  if (e.listeners.size === 0 && e.keepFresh === 0) entries.delete(id);
  syncTimer();
}

/** Subscribes to a row's updates; the cached row survives until its last subscriber and holder is gone. */
export function subscribeGuidanceRow(id: string, listener: () => void): () => void {
  const e = entryFor(id);
  e.listeners.add(listener);
  syncTimer();
  return () => {
    e.listeners.delete(listener);
    dropIfUnused(id, e);
  };
}

/** Keeps polling the row even once it has settled, until the returned release is called. */
export function holdGuidanceRowFresh(id: string): () => void {
  const e = entryFor(id);
  e.keepFresh++;
  syncTimer();
  return () => {
    e.keepFresh--;
    dropIfUnused(id, e);
  };
}

export function readGuidanceRow(id: string): PlainGuidanceRequest | undefined {
  return entries.get(id)?.row;
}

/** Test seam: drops every subscription and timer. */
export function resetGuidanceRowStore(): void {
  entries.clear();
  stopPolling();
  inFlight = false;
}
