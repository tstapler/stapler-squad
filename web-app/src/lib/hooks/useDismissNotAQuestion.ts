"use client";

import { useCallback, useMemo } from "react";
import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { getWatchTransport } from "@/lib/api/transport";
import { AcknowledgeSessionRequestSchema, SessionService } from "@/gen/session/v1/session_pb";

const DISMISS_KIND_FIELD = "dismissKind";
const NOT_A_QUESTION_VALUE = /NOT_A_QUESTION/;

/**
 * Resolves the generated enum number for "not a question" from AcknowledgeSessionRequest.dismiss_kind.
 * Returns null while the proto lacks that field (it ships with the server-side dismiss work), so this
 * file compiles and the button stays hidden until `make proto-gen` brings the field in.
 */
export function resolveNotAQuestionDismissKind(): { field: string; value: number } | null {
  const field = AcknowledgeSessionRequestSchema.fields.find((f) => f.localName === DISMISS_KIND_FIELD);
  if (!field || field.fieldKind !== "enum") return null;
  const value = field.enum.values.find((v) => NOT_A_QUESTION_VALUE.test(v.name));
  return value ? { field: DISMISS_KIND_FIELD, value: value.number } : null;
}

/**
 * Returns the "Not a question" dismissal for a prose_question row, or undefined when the
 * server contract (dismiss_kind) is not in the generated client yet. Plain AcknowledgeSession
 * must never stand in for it: that would mark the episode Done instead of recording a false positive.
 */
export function useDismissNotAQuestion(): ((sessionId: string) => Promise<void>) | undefined {
  const dismiss = useMemo(resolveNotAQuestionDismissKind, []);
  const client = useMemo(() => (dismiss ? createClient(SessionService, getWatchTransport()) : null), [dismiss]);
  const call = useCallback(
    async (sessionId: string) => {
      if (!dismiss || !client) return;
      const request = create(AcknowledgeSessionRequestSchema, { id: sessionId });
      (request as unknown as Record<string, unknown>)[dismiss.field] = dismiss.value;
      await client.acknowledgeSession(request);
    },
    [client, dismiss],
  );
  return dismiss ? call : undefined;
}
