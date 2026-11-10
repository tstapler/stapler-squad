"use client";

import { create } from "@bufbuild/protobuf";
import { GetItemFlowTimelineRequestSchema, type GetItemFlowTimelineResponse } from "@/gen/session/v1/backlog_flow_pb";
import type { FlowWindow } from "./useBacklogFlow";
import { useFlowRead, type UseFlowReadReturn } from "./useFlowRead";

/** Loads one item's collapsed event timeline (thrash episodes as single spans); idle while `itemId` is null. */
export function useFlowItemTimeline(itemId: string | null, window: FlowWindow): UseFlowReadReturn<GetItemFlowTimelineResponse> {
  const allTime = window === "all";
  return useFlowRead(
    itemId === null ? null : `${itemId}|${window}`,
    (client) => client.getItemFlowTimeline(create(GetItemFlowTimelineRequestSchema, { itemId: itemId ?? "", windowDays: allTime ? 0 : window, allTime })),
    "Could not load events",
  );
}
