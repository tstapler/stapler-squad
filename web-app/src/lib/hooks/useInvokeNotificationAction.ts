"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { invokeNotificationAction } from "@/lib/api/notificationActionApi";
import type {
  ActionKind,
  ActionTargetKey} from "@/gen/session/v1/notification_action_pb";
import {
  type InvokeNotificationActionResponse,
} from "@/gen/session/v1/notification_action_pb";

export interface ActionSpec {
  kind: ActionKind;
  targetKeyType: ActionTargetKey;
  targetKey: string;
  sessionId?: string;
  params?: { answer?: string; choice?: string; reason?: string; confirm?: boolean };
}

export type InvokeState =
  | { status: "idle" }
  | { status: "pending"; spec: ActionSpec }
  | { status: "settled"; spec: ActionSpec; response: InvokeNotificationActionResponse }
  /** The request never produced a response (network, 5xx): the key is kept for Retry. */
  | { status: "failed"; spec: ActionSpec };

const IDLE: InvokeState = { status: "idle" };

function newKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `k-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/**
 * Sends one inline action. `invoke` starts an attempt with a fresh idempotency key;
 * `retry` re-sends the last spec under the same key, so a duplicate write is impossible.
 * The row layer wraps `invoke` for any hold before dispatch (commit delay).
 */
export function useInvokeNotificationAction() {
  const [state, setState] = useState<InvokeState>(IDLE);
  const attempt = useRef<{ spec: ActionSpec; key: string } | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const send = useCallback(async (spec: ActionSpec, key: string) => {
    attempt.current = { spec, key };
    setState({ status: "pending", spec });
    try {
      const response = await invokeNotificationAction({
        actionKind: spec.kind,
        targetKeyType: spec.targetKeyType,
        targetKey: spec.targetKey,
        sessionId: spec.sessionId ?? "",
        params: spec.params,
        idempotencyKey: key,
      });
      if (mounted.current) setState({ status: "settled", spec, response });
      return response;
    } catch {
      if (mounted.current) setState({ status: "failed", spec });
      return null;
    }
  }, []);

  const invoke = useCallback((spec: ActionSpec) => send(spec, newKey()), [send]);
  const retry = useCallback(() => {
    const last = attempt.current;
    return last ? send(last.spec, last.key) : Promise.resolve(null);
  }, [send]);
  const reset = useCallback(() => setState(IDLE), []);

  return { state, invoke, retry, reset };
}
