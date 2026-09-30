"use client";

// useWatchWorkflows.ts — real-time subscription hook for saved workflow
// definitions. Mirrors useWatchBacklogItems.ts's stream-subscribe shape
// (dedicated hook, AbortController-per-effect-run, exponential-backoff
// reconnect, after_seq replay) at a scale appropriate to workflows: no
// Redux store, no staleness-backstop timers, and no forward/backward
// gap-detection bookkeeping — a saved-workflow list is small and low
// volume, so an unbounded, ever-retrying backoff (capped at 30s) is enough
// to guarantee eventual reconnection without a separate REST poll fallback.

import { useCallback, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { getWatchTransport } from "@/lib/api/transport";
import { SessionService, WatchWorkflowsRequestSchema } from "@/gen/session/v1/session_pb";
import type { WorkflowEvent, WorkflowProto } from "@/gen/session/v1/session_pb";

const MAX_BACKOFF_MS = 30_000;

export interface UseWatchWorkflowsHandlers {
  /** Called for both workflow_created and workflow_updated events. */
  onCreatedOrUpdated: (workflow: WorkflowProto) => void;
  onDeleted: (id: string) => void;
}

/**
 * Subscribes to the WatchWorkflows streaming RPC and invokes the given
 * callbacks for each create/update/delete event, reconnecting with
 * capped exponential backoff on stream errors or a clean server close.
 * workflow_run events don't change the workflow definition list and are
 * ignored, as is the content-free snapshot_complete marker.
 */
export function useWatchWorkflows(handlers: UseWatchWorkflowsHandlers): void {
  // Ref'd so the stream-connection effect below doesn't need to re-run (and
  // reconnect) just because the caller passed fresh callback identities.
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const lastSeqRef = useRef<bigint>(0n);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getWatchTransport());
  }, []);

  const handleEvent = useCallback((event: WorkflowEvent) => {
    if (event.seq > 0n) lastSeqRef.current = event.seq;

    switch (event.event.case) {
      case "workflowCreated":
      case "workflowUpdated": {
        const workflow = event.event.value.workflow;
        if (workflow) handlersRef.current.onCreatedOrUpdated(workflow);
        break;
      }
      case "workflowDeleted":
        handlersRef.current.onDeleted(event.event.value.id);
        break;
      case "workflowRun":
      case "snapshotComplete":
      default:
        break;
    }
  }, []);

  useEffect(() => {
    const abortController = new AbortController();
    const retries = { current: 0 };

    void runWatchWorkflowsLoop({
      clientRef,
      lastSeqRef,
      retriesRef: retries,
      signal: abortController.signal,
      onEvent: handleEvent,
    });

    return () => {
      abortController.abort();
    };
  }, [handleEvent]);
}

interface WatchWorkflowsLoopArgs {
  clientRef: { current: ReturnType<typeof createClient<typeof SessionService>> | null };
  lastSeqRef: { current: bigint };
  retriesRef: { current: number };
  signal: AbortSignal;
  onEvent: (event: WorkflowEvent) => void;
}

/** Schedules the next connect() attempt, backing off exponentially up to MAX_BACKOFF_MS. */
function scheduleWorkflowReconnect(args: WatchWorkflowsLoopArgs) {
  const delay = Math.min(1000 * Math.pow(2, args.retriesRef.current), MAX_BACKOFF_MS);
  args.retriesRef.current++;
  setTimeout(() => {
    if (!args.signal.aborted) void runWatchWorkflowsLoop(args);
  }, delay);
}

/**
 * One connect-consume-reconnect cycle of the WatchWorkflows stream. Split out
 * of the effect above to keep that effect's own setup/teardown readable and
 * avoid deep nesting between the retry-scheduling and stream-consuming logic.
 */
async function runWatchWorkflowsLoop(args: WatchWorkflowsLoopArgs): Promise<void> {
  const { clientRef, lastSeqRef, retriesRef, signal, onEvent } = args;
  if (signal.aborted || !clientRef.current) return;

  try {
    const stream = clientRef.current.watchWorkflows(
      create(WatchWorkflowsRequestSchema, { afterSeq: lastSeqRef.current }),
      { signal }
    );
    for await (const event of stream) {
      retriesRef.current = 0;
      onEvent(event);
    }
    // Clean server-side close -- reconnect without backoff.
    if (!signal.aborted) {
      retriesRef.current = 0;
      scheduleWorkflowReconnect(args);
    }
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") return;
    if (signal.aborted) return;
    console.error("[useWatchWorkflows] watchWorkflows stream error:", err);
    scheduleWorkflowReconnect(args);
  }
}
