import { connectApi } from "./connectApi";
import { createClient } from "@connectrpc/connect";
import { GuidanceRequestService } from "@/gen/session/v1/guidance_request_pb";
import { create } from "@bufbuild/protobuf";
import { ListGuidanceRequestsRequestSchema, AnswerGuidanceRequestRequestSchema } from "@/gen/session/v1/guidance_request_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { toPlainObject } from "@/lib/api/serialization";

export interface PlainGuidanceRequest {
  id: string;
  scope: string;
  itemId?: string;
  sessionUuid: string;
  questionText: string;
  questionType: string;
  options: string[];
  answer: string;
  status: string;
  createdAt: Record<string, unknown> | undefined;
  answeredAt: Record<string, unknown> | undefined;
}

function getClient() {
  return createClient(GuidanceRequestService, getConnectTransport());
}

export const guidanceApi = connectApi.injectEndpoints({
  endpoints: (builder) => ({
    listGuidanceRequests: builder.query<
      { requests: PlainGuidanceRequest[]; pendingCount: number; cap: number },
      { scope: string; scopeKey: string }
    >({
      queryFn: async ({ scope, scopeKey }) => {
        try {
          const client = getClient();
          const req = create(ListGuidanceRequestsRequestSchema, { scope, scopeKey, includeAnswered: true });
          const response = await client.listGuidanceRequests(req);
          const requests = response.requests.map((r) => toPlainObject(r) as unknown as PlainGuidanceRequest);
          return { data: { requests, pendingCount: response.pendingCount, cap: response.cap } };
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Failed to fetch guidance requests";
          return { error: { status: -1, error: msg } };
        }
      },
      providesTags: ["GuidanceRequests"],
    }),
    answerGuidanceRequest: builder.mutation<{ applied: boolean }, { id: string; answer: string }>({
      queryFn: async ({ id, answer }) => {
        try {
          const client = getClient();
          const req = create(AnswerGuidanceRequestRequestSchema, { id, answer });
          // applied is false when a losing racer answers an already-answered/cancelled
          // request (server-side dedup guard) — callers must surface that, not treat it
          // as a normal success.
          const response = await client.answerGuidanceRequest(req);
          return { data: { applied: response.applied } };
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Failed to submit answer";
          return { error: { status: -1, error: msg } };
        }
      },
      invalidatesTags: ["GuidanceRequests"],
    }),
  }),
});

export const { useListGuidanceRequestsQuery, useAnswerGuidanceRequestMutation } = guidanceApi;
