"use client";

// useWatchBacklogItems.ts — shared real-time subscription hook for backlog
// items (Epic 4.2, project_plans/backlog-event-driven-updates). Structurally
// mirrors useReviewQueue.ts's connection lifecycle (AbortController-per-
// effect-run, exponential backoff, REST fallback polling) and additionally
// ports useSessionService.ts's idle-staleness backstop (30s periodic timer +
// 15s visibility/online check) plus new after_seq-based forward/backward
// gap detection that neither existing hook has (pitfalls.md #4).
//
// Design notes on the two open questions this epic resolved:
//
// 1. item_archived events are intentionally NOT translated into a
//    backlogItemsSlice action here. BacklogItemArchivedEvent carries only
//    itemId/archivedAt — no full BacklogItem payload — so it cannot call
//    upsertItem, and plan.md's Task 4.2.1b oneof-to-action mapping omits it
//    on purpose. It is consumed at the component layer instead (Phase 5
//    Tasks 5.3.1c/5.4.1c) via a separate item-scoped subscription, not this
//    shared list-level hook or the normalized store.
//
// 2. connectionState is hook-local React state, not a backlogItemsSlice
//    reducer. Unlike sessionsSlice.connectionState (which useSessionService
//    reads via a Redux selector), no Epic 4.2 task lists backlogItemsSlice.ts
//    as a touched file for connectionState, and Task 4.2.1a's signature
//    returns connectionState directly from the hook's own return value.
//
// 3. Epic 5.2 fix (found blocking Phase 5 consumer wiring, both this board
//    and the /backlog list page): backlogItemsSlice stores the raw proto
//    BacklogItem (from @/gen/session/v1/backlog_pb) — acceptanceCriteria,
//    autoCreatePr, proto Timestamp fields — but every rendering consumer
//    (BacklogItemCard, BacklogBoard, BacklogItemDetail) is written against
//    useBacklogService.ts's mapped domain BacklogItem (acCriteria,
//    gateVerdict, triageStatus derived from itemSessions, ISO date strings).
//    Neither type is assignable to the other. This hook now maps through
//    useBacklogService's mapBacklogItem before returning items, so the
//    normalized store itself stays proto-shaped (unaffected) while every
//    consumer of this hook gets the domain shape it actually renders.
export type BacklogConnectionState =
  | "connecting"
  | "live"
  | "reconnecting"
  | "polling"
  | "stale";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem, BacklogItemEvent, ReviewVerdict } from "@/gen/session/v1/backlog_pb";
import { useAppDispatch, useAppSelector } from "@/lib/store";
import {
  upsertItem,
  removeItem,
  appendActivityNote,
  selectAllBacklogItems,
  selectBacklogItemsLiveVersionMap,
} from "@/lib/store/backlogItemsSlice";
import { mapBacklogItem } from "@/lib/hooks/useBacklogService";
import type { BacklogItem as MappedBacklogItem } from "@/lib/hooks/useBacklogService";

const MAX_RETRIES = 5;
const FALLBACK_POLL_INTERVAL_MS = 30_000;
const BACKSTOP_INTERVAL_MS = 30_000;
const STALE_THRESHOLD_MS = 15_000;
const VISIBILITY_DEBOUNCE_MS = 200;
// AC #19 (project_plans/backlog-event-driven-updates/design/ux.md,
// ConnectionIndicator's "Rapid connect/disconnect flapping" edge case): only
// the reconnecting -> live transition is debounced by a "few hundred ms"
// hold, never connected -> reconnecting (that must announce trouble
// immediately). A flaky connection that reconnects and drops again within
// this window never visibly flickers back to "Live".
const LIVE_TRANSITION_DEBOUNCE_MS = 300;

export interface UseWatchBacklogItemsFilters {
  statusFilter?: string[];
  categoryFilter?: string[];
}

export interface UseWatchBacklogItemsReturn {
  items: MappedBacklogItem[];
  connectionState: BacklogConnectionState;
}

/**
 * Defense-in-depth patch for the verdict_recorded event (see the switch case
 * below): finds the most recent review-role ItemSession in `item.itemSessions`
 * — the same session mapBacklogItem's own "most recent review session" logic
 * (useBacklogService.ts) reads gateVerdict/gateVerdictSummary from — and, if
 * its embedded reviewVerdict doesn't already match, overwrites it with the
 * event's inline `verdict` field. Returns `item` unchanged (same reference)
 * when there is nothing to patch, so callers that don't need this stay cheap.
 *
 * Deliberately does NOT fabricate a review session when none exists in the
 * embedded item — that gap is deeper than a missing verdict field, and
 * inventing session metadata (id, sessionUuid, startedAt, ...) this hook has
 * no visibility into would be worse than leaving it to the primary
 * (server-side eager-load) fix path.
 */
function applyInlineVerdict(item: BacklogItem, verdict: ReviewVerdict | undefined): BacklogItem {
  if (!verdict) return item;
  const sessions = item.itemSessions;
  let lastReviewIdx = -1;
  for (let i = 0; i < sessions.length; i++) {
    if (sessions[i].sessionRole === "review") lastReviewIdx = i;
  }
  if (lastReviewIdx === -1) return item;

  const existing = sessions[lastReviewIdx];
  if (
    existing.reviewVerdict?.overallOutcome === verdict.overallOutcome &&
    existing.reviewVerdict?.summary === verdict.summary
  ) {
    return item;
  }

  const patchedSessions = sessions.slice();
  patchedSessions[lastReviewIdx] = { ...existing, reviewVerdict: verdict };
  return { ...item, itemSessions: patchedSessions };
}

/**
 * Subscribes to real-time backlog item changes via the WatchBacklogItems
 * streaming RPC, dispatching every received event into backlogItemsSlice,
 * and keeps the store fresh across disconnects (exponential-backoff
 * reconnect, REST fallback polling, after_seq replay, and an idle-staleness
 * backstop for periods with zero live events).
 */
export function useWatchBacklogItems(
  filters: UseWatchBacklogItemsFilters = {}
): UseWatchBacklogItemsReturn {
  const { statusFilter, categoryFilter } = filters;
  // Stable string keys so effects don't re-run just because the caller
  // passed a fresh array/object literal this render (a likely pitfall for a
  // hook that Phase 5 will call inline in component bodies).
  const statusFilterKey = (statusFilter ?? []).join(",");
  const categoryFilterKey = (categoryFilter ?? []).join(",");

  const dispatch = useAppDispatch();
  const items = useAppSelector(selectAllBacklogItems);
  const liveVersions = useAppSelector(selectBacklogItemsLiveVersionMap);
  const [connectionState, setConnectionState] = useState<BacklogConnectionState>("connecting");

  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

  // Stream health/reconnect bookkeeping — hoisted to hook-level refs (rather
  // than effect-local) so the fallback-poll, backstop, and visibility/online
  // effects can all read/drive the same connection state, mirroring
  // useSessionService.ts's ref layout.
  const isConnectedRef = useRef(false);
  const lastEventTimeRef = useRef<number | null>(null);
  const lastSeqRef = useRef<bigint>(0n);
  const resyncInFlightRef = useRef(false);
  // Set right before a backstop- or visibility/online-triggered reconnect;
  // cleared (and a full refetch fired) on that reconnect's first received
  // event. This ties the "reconnect success path issues a refetch"
  // requirement (Story 4.2.3) specifically to self-healing reconnects, not
  // ordinary in-loop backoff retries — and fires even if zero
  // BacklogItemEvents were ever received before the staleness was detected.
  const staleReconnectPendingRef = useRef(false);
  const backstopTriggeredRef = useRef(false);
  const streamRetriesRef = useRef(0);
  const streamDeadRef = useRef(false);
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // AC #19: pending reconnecting -> live debounce (see
  // LIVE_TRANSITION_DEBOUNCE_MS above). Re-armed on every successful
  // (re)connect/resync; the scheduled flip to "live" only actually commits if
  // the stream is still connected once the timer fires, so a flap that drops
  // again mid-hold never shows "Live" at all.
  const liveTransitionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Set by the main watch effect on every run; called by the fallback-poll,
  // backstop, and visibility/online effects to force a reconnect without
  // needing `connect` in their own dependency arrays.
  const reconnectRef = useRef<(() => void) | null>(null);

  // Initialize ConnectRPC client. Uses plain HTTP (not the WebSocket bridge
  // transport useSessionService/useReviewQueue use for their Watch* RPCs)
  // because BacklogService.WatchBacklogItems is not yet registered with
  // server.go's StreamingWSBridge — standard Connect server-streaming over
  // HTTP works today without that registration; wiring the WS bridge is a
  // separate, larger server.go change out of scope for this frontend epic.
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    clientRef.current = createClient(BacklogService, transport);
  }, []);

  // Full REST refetch — used for the initial load, gap-detected resyncs, the
  // fallback poll, and every successful (re)connection after the first.
  const refresh = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      const resp = await clientRef.current.listBacklogItems({
        // ListBacklogItemsRequest has no category field (see
        // backlogItemMatchesFilters's doc comment server-side) — only status
        // can be enforced on this REST snapshot; category filtering only
        // applies to the live stream below.
        status: statusFilter ?? [],
        priority: [],
        includeTerminal: false,
        includeArchived: false,
        sortBy: "",
      });
      for (const item of resp.items ?? []) {
        dispatch(upsertItem(item));
      }
    } catch (err) {
      console.error("[useWatchBacklogItems] listBacklogItems failed:", err);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dispatch, statusFilterKey]);

  // On mount, immediately issue the REST snapshot fetch alongside (not
  // gated behind) opening the stream below — Task 4.2.1a/pitfalls.md #1.
  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilterKey]);

  // AC #19: debounce the reconnecting -> live transition only (never
  // connected -> reconnecting, which must show immediately elsewhere in this
  // hook). Cancels any previously scheduled flip and re-arms the hold, so
  // repeated flapping keeps deferring "Live" until the connection has been
  // stable for the full window rather than flickering it on and off.
  const scheduleLiveTransition = useCallback(() => {
    if (liveTransitionTimerRef.current) clearTimeout(liveTransitionTimerRef.current);
    liveTransitionTimerRef.current = setTimeout(() => {
      liveTransitionTimerRef.current = null;
      if (isConnectedRef.current) setConnectionState("live");
    }, LIVE_TRANSITION_DEBOUNCE_MS);
  }, []);

  // Gap-detected resync: briefly reflect a "reconnecting" state while the
  // refetch is in flight, then return to "live" if the stream is still up.
  const triggerResync = useCallback(async () => {
    if (resyncInFlightRef.current) return;
    resyncInFlightRef.current = true;
    setConnectionState((prev) => (prev === "live" ? "reconnecting" : prev));
    try {
      await refresh();
    } finally {
      resyncInFlightRef.current = false;
      if (isConnectedRef.current) scheduleLiveTransition();
    }
  }, [refresh, scheduleLiveTransition]);

  // Apply after_seq bookkeeping (Story 4.2.2) then dispatch to the store.
  const handleEvent = useCallback(
    (event: BacklogItemEvent) => {
      lastEventTimeRef.current = Date.now();

      // seq === 0 marks a synthetic per-item snapshot event sent on a fresh
      // (non-replay) connection — it carries no real bus sequence number, so
      // it must not participate in gap detection (see BacklogItemEvent.seq's
      // proto doc comment).
      const seq = event.seq;
      if (seq > 0n) {
        const prev = lastSeqRef.current;
        if (prev > 0n) {
          if (seq < prev) {
            // Backwards jump: server restarted and its seq counter reset.
            // Mirrors useSessionService.ts:730-742 exactly.
            lastSeqRef.current = 0n;
            void triggerResync();
          } else if (seq !== prev + 1n) {
            // Forward gap: the bus's bounded, non-blocking fan-out dropped
            // an event under backpressure (pitfalls.md #4 — new logic, not
            // present in useSessionService.ts today).
            lastSeqRef.current = seq;
            void triggerResync();
          } else {
            lastSeqRef.current = seq;
          }
        } else {
          // No established baseline yet (very first real-seq event this
          // client has ever seen) — seed it without treating an arbitrary
          // starting seq as a gap.
          lastSeqRef.current = seq;
        }
      }

      switch (event.event.case) {
        case "verdictRecorded": {
          // Defense-in-depth (Phase 5 spec-compliance sweep,
          // backlog-event-driven-updates): BacklogItemVerdictRecordedEvent
          // carries the just-saved verdict inline (`verdict`), populated
          // directly from the ReviewVerdictData the backend just wrote —
          // independent of whether the embedded `item.itemSessions` snapshot
          // was eager-loaded correctly (the root-cause fix for that lives
          // server-side, session/ent_repository_backlog.go's
          // attachItemSessionsForPublish). Patch the inline verdict onto the
          // most recent review-role session in the embedded item before
          // dispatching, so gateVerdict/gateVerdictSummary (mapBacklogItem,
          // useBacklogService.ts) get a second, independent path to
          // correctness even if that primary path has a gap.
          const { item, verdict, isSnapshot } = event.event.value;
          if (item) dispatch(upsertItem(applyInlineVerdict(item, verdict), isSnapshot));
          break;
        }
        case "statusChanged":
        case "sessionAttached":
        case "itemUpdated": {
          const item = event.event.value.item;
          // Epic 6.1: thread this event's own is_snapshot flag through so
          // the store only bumps liveVersion (flash-eligible) for a genuine
          // live event — never for the initial snapshot or a forced-
          // is_snapshot replay-branch copy (pre-mortem #4).
          if (item) dispatch(upsertItem(item, event.event.value.isSnapshot));
          break;
        }
        case "itemArchived":
          // Intentionally not applied to backlogItemsSlice — see file header.
          break;
        case "itemRemoved":
          dispatch(removeItem(event.event.value.itemId));
          break;
        case "activityNoteAdded": {
          // ADR-002: a dedicated single-entry event — never a full item
          // snapshot — so it dispatches the targeted appendActivityNote
          // reducer, never upsertItem.
          const { itemId, note } = event.event.value;
          if (note) dispatch(appendActivityNote({ itemId, note }));
          break;
        }
        case "snapshotComplete":
          // Synthetic, content-free marker (see backlog_service_events.go's
          // watchBacklogItems) sent when the initial snapshot/replay phase
          // had nothing else to send — e.g. a genuinely empty backlog. It
          // carries no item, so there's nothing to dispatch; its only job is
          // to be *an event at all* so the `for await` loop above advances
          // past its first iteration and connectionState can reach "live"
          // even with zero backlog items.
          break;
        default:
          break;
      }
    },
    [dispatch, triggerResync]
  );

  // Main stream connection lifecycle: connect, consume via `for await`,
  // reconnect with exponential backoff on error (capped at 30s, 5 retries,
  // matching useReviewQueue.ts's constants exactly), then fall back to
  // polling once retries are exhausted.
  useEffect(() => {
    streamRetriesRef.current = 0;
    streamDeadRef.current = false;

    const abortController = new AbortController();
    const signal = abortController.signal;

    const connect = async () => {
      if (signal.aborted || !clientRef.current) return;

      // Treat stream (re)connect attempts as activity so the 30s backstop
      // below can engage even if the connection never yields a single event
      // (mirrors useSessionService.ts:822 exactly).
      lastEventTimeRef.current = Date.now();

      try {
        const stream = clientRef.current.watchBacklogItems(
          {
            statusFilter: statusFilter ?? [],
            categoryFilter: categoryFilter ?? [],
            afterSeq: lastSeqRef.current,
          },
          { signal }
        );

        let firstEvent = true;
        for await (const event of stream) {
          if (firstEvent) {
            firstEvent = false;
            isConnectedRef.current = true;
            backstopTriggeredRef.current = false;
            streamRetriesRef.current = 0;
            streamDeadRef.current = false;
            scheduleLiveTransition();
            // Story 4.2.3: a backstop- or visibility-triggered reconnect's
            // success path issues a full refetch, even if zero
            // BacklogItemEvents were ever received during the whole idle
            // period beforehand.
            if (staleReconnectPendingRef.current) {
              staleReconnectPendingRef.current = false;
              void refresh();
            }
          }
          handleEvent(event);
        }

        // Clean server-side close — reset retry counter; the fallback-poll/
        // backstop/visibility effects will drive the next reconnect attempt.
        isConnectedRef.current = false;
        streamRetriesRef.current = 0;
      } catch (err) {
        isConnectedRef.current = false;
        if (err instanceof Error && err.name === "AbortError") return;
        if (signal.aborted) return;

        console.error("[useWatchBacklogItems] watchBacklogItems stream error:", err);

        if (streamRetriesRef.current < MAX_RETRIES) {
          const delay = Math.min(1000 * Math.pow(2, streamRetriesRef.current), 30_000);
          streamRetriesRef.current++;
          setConnectionState("reconnecting");
          setTimeout(() => {
            if (signal.aborted) return;
            void connect();
          }, delay);
        } else {
          streamDeadRef.current = true;
          setConnectionState("polling");
        }
      }
    };

    reconnectRef.current = () => void connect();
    void connect();

    return () => {
      reconnectRef.current = null;
      abortController.abort();
      isConnectedRef.current = false;
      if (liveTransitionTimerRef.current) {
        clearTimeout(liveTransitionTimerRef.current);
        liveTransitionTimerRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilterKey, categoryFilterKey, handleEvent, refresh, scheduleLiveTransition]);

  // REST fallback polling (Task 4.2.1d): once retries are exhausted, poll
  // periodically; a successful poll that finds the stream dead attempts
  // exactly one reconnect before continuing to poll.
  useEffect(() => {
    const interval = setInterval(() => {
      void (async () => {
        await refresh();
        if (streamDeadRef.current) {
          streamDeadRef.current = false;
          streamRetriesRef.current = 0;
          reconnectRef.current?.();
        }
      })();
    }, FALLBACK_POLL_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [refresh]);

  // Story 4.2.3 — idle-staleness backstop #1: a 30s periodic timer that
  // forces a reconnect + full refetch even with zero live events, mirroring
  // useSessionService.ts:944-962 verbatim.
  useEffect(() => {
    const interval = setInterval(() => {
      if (
        !isConnectedRef.current &&
        lastEventTimeRef.current !== null &&
        Date.now() - lastEventTimeRef.current > BACKSTOP_INTERVAL_MS
      ) {
        setConnectionState("stale");
        if (!backstopTriggeredRef.current) {
          backstopTriggeredRef.current = true;
          staleReconnectPendingRef.current = true;
          reconnectRef.current?.();
        }
      }
    }, BACKSTOP_INTERVAL_MS);
    return () => clearInterval(interval);
  }, []);

  // Story 4.2.3 — idle-staleness backstop #2: a 15s staleness threshold on
  // visibility/online events, mirroring useSessionService.ts:971-986.
  useEffect(() => {
    const handleVisibilityOrOnline = (ev: Event) => {
      if (document.visibilityState !== "visible" && ev.type !== "online") return;
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
      debounceTimerRef.current = setTimeout(() => {
        debounceTimerRef.current = null;
        const isStale =
          lastEventTimeRef.current !== null && lastEventTimeRef.current < Date.now() - STALE_THRESHOLD_MS;
        if (!isConnectedRef.current || isStale) {
          if (isStale) setConnectionState("stale");
          staleReconnectPendingRef.current = true;
          streamRetriesRef.current = 0;
          streamDeadRef.current = false;
          reconnectRef.current?.();
        }
      }, VISIBILITY_DEBOUNCE_MS);
    };

    document.addEventListener("visibilitychange", handleVisibilityOrOnline);
    window.addEventListener("online", handleVisibilityOrOnline);
    return () => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
      document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
      window.removeEventListener("online", handleVisibilityOrOnline);
    };
  }, []);

  // Map proto -> domain shape at the hook boundary (see file header, note 3)
  // so every consumer gets the same fields useBacklogService.listBacklogItems
  // already produces, rather than each render component reimplementing
  // (and risking drift from) mapBacklogItem's derivation logic.
  //
  // Cached per source proto-item reference (not just per useMemo call) —
  // Epic 6.1's render-count guarantee (pre-mortem #3) needs each *unrelated*
  // item's mapped object to keep the same identity across a store update,
  // so a memoized consumer (BacklogItemCard, wrapped in React.memo) actually
  // skips re-rendering it. selectAllBacklogItems must return a brand-new
  // array whenever ANY item changes (see its own doc comment), but thanks to
  // Immer's structural sharing, an unrelated item's *element* reference
  // inside that array is untouched — plain `.map(mapBacklogItem)` would
  // still call the mapper (and allocate a fresh object) for every element on
  // every call, throwing that stability away. This WeakMap keyed on the
  // proto item reference restores it: only items whose proto reference
  // actually changed (i.e. were upserted) get remapped, and that's exactly
  // where the fresh `liveVersion` value belongs too.
  const mappedItemCacheRef = useRef(new WeakMap<BacklogItem, MappedBacklogItem>());
  const mappedItems = useMemo(() => {
    const cache = mappedItemCacheRef.current;
    return items.map((protoItem) => {
      const cached = cache.get(protoItem);
      if (cached) return cached;
      const mapped: MappedBacklogItem = {
        ...mapBacklogItem(protoItem),
        liveVersion: liveVersions[protoItem.id],
      };
      cache.set(protoItem, mapped);
      return mapped;
    });
  }, [items, liveVersions]);

  return useMemo(() => ({ items: mappedItems, connectionState }), [mappedItems, connectionState]);
}
