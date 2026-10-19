"use client";

import { useEffect, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { GuidanceRequestService, ListSavedGuidanceAnswersRequestSchema } from "@/gen/session/v1/guidance_request_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { toPlainObject } from "@/lib/api/serialization";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";

export const SAVED_POLL_MS = 30_000;

/**
 * Answers stored for a stopped or archived session (agent-created requests only). Read while
 * `enabled` (the tray is open); a failed read keeps the last list, an empty list renders nothing.
 */
export function useSavedAnswers(enabled: boolean): PlainGuidanceRequest[] {
  const [rows, setRows] = useState<PlainGuidanceRequest[]>([]);
  useEffect(() => {
    if (!enabled) return;
    const ctrl = new AbortController();
    const read = () =>
      Promise.resolve()
        .then(() => createClient(GuidanceRequestService, getConnectTransport()).listSavedGuidanceAnswers(create(ListSavedGuidanceAnswersRequestSchema, {}), { signal: ctrl.signal }))
        .then((res) => setRows(res.requests.map((r) => toPlainObject(r) as unknown as PlainGuidanceRequest)))
        .catch(() => {});
    void read();
    const timer = setInterval(() => void read(), SAVED_POLL_MS);
    return () => {
      clearInterval(timer);
      ctrl.abort();
    };
  }, [enabled]);
  return rows;
}
