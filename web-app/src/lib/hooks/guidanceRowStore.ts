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
  e.listeners.forEach((l) => l());
  syncTimer();
}

/**
 * One tick serves every subscribed row: a single ListAllPendingGuidanceRequests answers all open
 * cards, and only rows missing from it (answered, awaiting acknowledgement) get an individual read.
 * A tick is skipped while the previous one is in flight or the tab is hidden.
 */
async function tick(urgent = false): Promise<void> {
  if (inFlight) {
    // A row that mounted mid-flight must not wait a whole interval for its first read.
    if (urgent) rerunAfterFlight = true;
    return;
  }
  if (isPageHidden()) return;
  const wanted = [...entries].filter(([, e]) => isLive(e)).map(([id]) => id);
  if (wanted.length === 0) return;
  inFlight = true;
  try {
    const client = createClient(GuidanceRequestService, getConnectTransport());
    const seen = new Set<string>();
    try {
      // One poll is shared by every row, so no single caller owns it.
      // abort-signal-exempt
      const res = await client.listAllPendingGuidanceRequests(
        create(ListAllPendingGuidanceRequestsRequestSchema, {}),
      );
      for (const r of res.requests) {
        seen.add(r.id);
        publish(r.id, toPlain(r));
      }
    } catch {
      // keep the last rows; the per-id reads below still run
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
    e = { row: undefined, listeners: new Set(), keepFresh: 0 };
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
