"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import { holdGuidanceRowFresh, readGuidanceRow, subscribeGuidanceRow } from "./guidanceRowStore";

export { GUIDANCE_POLL_MS, isMoving } from "./guidanceRowStore";

/**
 * Reads one durable question and keeps it fresh while it can still change (an open card, or an
 * answer waiting on acknowledgement). Every mounted row shares one poll (see guidanceRowStore).
 * A failed read keeps the last row. Pass `undefined` to skip, e.g. while the structured-question
 * flag is off.
 */
export function useGuidanceRow(id: string | undefined, keepFresh: boolean): PlainGuidanceRequest | undefined {
  const subscribe = useCallback(
    (onChange: () => void) => (id ? subscribeGuidanceRow(id, onChange) : () => {}),
    [id],
  );
  const getSnapshot = useCallback(() => (id ? readGuidanceRow(id) : undefined), [id]);
  useEffect(() => (id && keepFresh ? holdGuidanceRowFresh(id) : undefined), [id, keepFresh]);
  return useSyncExternalStore(subscribe, getSnapshot, () => undefined);
}
