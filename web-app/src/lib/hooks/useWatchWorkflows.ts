"use client";

// useWatchWorkflows.ts — real-time subscription hook for saved workflow
// definitions. Connection mechanics (backoff reconnect, idle-staleness
// watchdog, afterSeq bookkeeping) live in the shared useWatchStream.ts;
// this hook only supplies the WatchWorkflows-specific subscribe call and
// event-shape accessors.

import { useCallback, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { getWatchTransport } from "@/lib/api/transport";
import { SessionService, WatchWorkflowsRequestSchema } from "@/gen/session/v1/session_pb";
import type { WorkflowEvent, WorkflowProto } from "@/gen/session/v1/session_pb";
import { useWatchStream } from "@/lib/hooks/useWatchStream";

export interface UseWatchWorkflowsHandlers {
  /** Called for both workflow_created and workflow_updated events. */
  onCreatedOrUpdated: (workflow: WorkflowProto) => void;
  onDeleted: (id: string) => void;
}

/** workflow_run events don't change the workflow definition list and are
 * ignored, as is the content-free snapshot_complete marker. */
function dispatchWorkflowEvent(event: WorkflowEvent, handlers: UseWatchWorkflowsHandlers): void {
  switch (event.event.case) {
    case "workflowCreated":
    case "workflowUpdated": {
      const workflow = event.event.value.workflow;
      if (workflow) handlers.onCreatedOrUpdated(workflow);
      break;
    }
    case "workflowDeleted":
      handlers.onDeleted(event.event.value.id);
      break;
    case "workflowRun":
    case "snapshotComplete":
    default:
      break;
  }
}

/**
 * Subscribes to the WatchWorkflows streaming RPC and invokes the given
 * callbacks for each create/update/delete event. workflow_run events don't
 * change the workflow definition list and are ignored, as is the
 * content-free snapshot_complete marker.
 */
export function useWatchWorkflows(handlers: UseWatchWorkflowsHandlers): void {
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getWatchTransport());
  }, []);

  const subscribe = useCallback(
    (afterSeq: bigint, signal: AbortSignal) =>
      clientRef.current!.watchWorkflows(create(WatchWorkflowsRequestSchema, { afterSeq }), { signal }),
    []
  );

  const onEvent = useCallback((event: WorkflowEvent) => {
    dispatchWorkflowEvent(event, handlersRef.current);
  }, []);

  useWatchStream<WorkflowEvent>({
    subscribe,
    onEvent,
    isHeartbeat: (event) => event.heartbeat,
    getSeq: (event) => event.seq,
  });
}
