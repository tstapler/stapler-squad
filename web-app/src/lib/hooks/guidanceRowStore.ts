import { create, type Message } from "@bufbuild/protobuf";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
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
/** Consecutive list failures before the poll backs off and reads rows individually instead. */
const LIST_FAILURES_BEFORE_BACKOFF = 3;
const LIST_BACKOFF_MAX_MS = 30_000;
/** Individual reads in flight at once when the list is unavailable. */
const ROW_READ_CONCURRENCY = 3;

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
  /** The server no longer has this row (not found): stop polling it. */
  gone: boolean;
  /** When the row was last published, to pace reads of rows that are only held fresh. */
  lastRead: number;
}

const entries = new Map<string, Entry>();
let timer: ReturnType<typeof setInterval> | undefined;
let stopVisibility: (() => void) | undefined;
let inFlight = false;
let rerunAfterFlight = false;
let listFailures = 0;
let listRetryAt = 0;

const isLive = (e: Entry) => !e.gone && (e.keepFresh > 0 || isMoving(e.row));
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

type Client = ReturnType<typeof createClient<typeof GuidanceRequestService>>;

function markGone(id: string): void {
  const e = entries.get(id);
  if (e) e.gone = true;
  syncTimer();
}

async function readRow(client: Client, id: string): Promise<void> {
  try {
    // abort-signal-exempt
    const res = await client.getGuidanceRequest(
      create(GetGuidanceRequestRequestSchema, { id }),
      {
        timeoutMs: POLL_TIMEOUT_MS,
      },
    );
    if (res.request) publish(id, toPlain(res.request));
    else markGone(id);
  } catch (err) {
    if (ConnectError.from(err).code === Code.NotFound) markGone(id);
  }
}

async function readRows(client: Client, ids: string[]): Promise<void> {
  for (let i = 0; i < ids.length; i += ROW_READ_CONCURRENCY) {
    await Promise.all(
      ids.slice(i, i + ROW_READ_CONCURRENCY).map((id) => readRow(client, id)),
    );
  }
}

/** Asks the shared list; false when it failed (the caller decides whether to fall back to per-row reads). */
async function readList(client: Client, seen: Set<string>): Promise<boolean> {
  try {
    // One poll is shared by every row, so no single caller owns it.
    // abort-signal-exempt
    const res = await client.listAllPendingGuidanceRequests(
      create(ListAllPendingGuidanceRequestsRequestSchema, {}),
      {
        timeoutMs: POLL_TIMEOUT_MS,
      },
    );
    for (const r of res.requests) {
      seen.add(r.id);
      if (entries.has(r.id)) publish(r.id, toPlain(r));
    }
    listFailures = 0;
    return true;
  } catch {
    listFailures++;
    if (listFailures >= LIST_FAILURES_BEFORE_BACKOFF) {
      const backoff =
        GUIDANCE_POLL_MS *
        2 ** (listFailures - LIST_FAILURES_BEFORE_BACKOFF + 1);
      listRetryAt = Date.now() + Math.min(LIST_BACKOFF_MAX_MS, backoff);
    }
    return false;
  }
}

/**
 * One tick serves every subscribed row. With more than LIST_THRESHOLD live rows a single
 * ListAllPendingGuidanceRequests answers all open cards and only rows missing from it (answered, awaiting
 * acknowledgement) get an individual read; with fewer, individual reads are cheaper than the global list.
 * A failing list ends the tick (no 1+N burst); after LIST_FAILURES_BEFORE_BACKOFF failures it backs off
 * exponentially and rows are read individually, a few at a time, meanwhile. A row the server no longer has
 * is dropped from polling. Settled rows held fresh by a subscriber are re-read only every SETTLED_POLL_MS.
 * A tick is skipped while the previous one is in flight or the tab is hidden.
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
    .filter(
      ([, e]) =>
        !e.gone &&
        (isMoving(e.row) ||
          (e.keepFresh > 0 && now - e.lastRead >= SETTLED_POLL_MS)),
    )
    .map(([id]) => id);
  if (wanted.length === 0) return;
  inFlight = true;
  try {
    const client = createClient(GuidanceRequestService, getConnectTransport());
    const seen = new Set<string>();
    if (wanted.length > LIST_THRESHOLD && now >= listRetryAt) {
      const ok = await readList(client, seen);
      if (!ok && listFailures < LIST_FAILURES_BEFORE_BACKOFF) return; // keep the last rows and retry next tick
    }
    await readRows(
      client,
      wanted.filter((id) => !seen.has(id)),
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
    e = {
      row: undefined,
      listeners: new Set(),
      keepFresh: 0,
      gone: false,
      lastRead: 0,
    };
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
export function subscribeGuidanceRow(
  id: string,
  listener: () => void,
): () => void {
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
  rerunAfterFlight = false;
  listFailures = 0;
  listRetryAt = 0;
}
