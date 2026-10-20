"use client";

import { useEffect, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { GetGuidanceRequestRequestSchema, GuidanceRequestService } from "@/gen/session/v1/guidance_request_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { toPlainObject } from "@/lib/api/serialization";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";

export const GUIDANCE_POLL_MS = 3_000;

/** Delivery states that still change on their own: pending, sent (acknowledgement or the 60 s decision). */
const MOVING = new Set(["pending", "sent"]);

/** A row that can still change without the operator: open, or answered and not yet settled. */
export function isMoving(row: PlainGuidanceRequest | undefined): boolean {
  if (!row) return true;
  if (row.status === "pending") return true;
  return row.status === "answered" && MOVING.has(row.deliveryState ?? "");
}

/**
 * Reads one durable question and re-reads it every few seconds while it can still change (an
 * open card, or an answer waiting on acknowledgement). A failed read keeps the last row. Pass
 * `undefined` to skip, e.g. while the structured-question flag is off.
 */
export function useGuidanceRow(id: string | undefined, keepFresh: boolean): PlainGuidanceRequest | undefined {
  const [row, setRow] = useState<PlainGuidanceRequest | undefined>(undefined);
  const live = keepFresh || isMoving(row);
  useEffect(() => {
    if (!id) return;
    const ctrl = new AbortController();
    const read = () =>
      Promise.resolve()
        .then(() => createClient(GuidanceRequestService, getConnectTransport()).getGuidanceRequest(create(GetGuidanceRequestRequestSchema, { id }), { signal: ctrl.signal }))
        .then((res) => res.request && setRow(toPlainObject(res.request) as unknown as PlainGuidanceRequest))
        .catch(() => {});
    void read();
    if (!live) return () => ctrl.abort();
    const timer = setInterval(() => void read(), GUIDANCE_POLL_MS);
    return () => {
      clearInterval(timer);
      ctrl.abort();
    };
  }, [id, live]);
  return id ? row : undefined;
}
