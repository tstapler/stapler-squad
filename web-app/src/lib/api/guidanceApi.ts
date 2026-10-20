import { connectApi } from "./connectApi";
import { createClient } from "@connectrpc/connect";
import { GuidanceRequestService } from "@/gen/session/v1/guidance_request_pb";
import { create } from "@bufbuild/protobuf";
import {
  ListGuidanceRequestsRequestSchema,
  AnswerGuidanceRequestRequestSchema,
  GetGuidanceRequestRequestSchema,
  ListSavedGuidanceAnswersRequestSchema,
} from "@/gen/session/v1/guidance_request_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { toPlainObject } from "@/lib/api/serialization";

/** One question of a mirrored or structured request (session.v1.GuidanceQuestion). */
export interface PlainGuidanceQuestion {
  question: string;
  header: string;
  multiSelect: boolean;
  options: { label: string; description: string }[];
}

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
  /** Live-question and delivery fields (ADR-003); absent on rows an older server sent. */
  source?: string;
  askerSessionUuid?: string;
  questions?: PlainGuidanceQuestion[];
  deliveryRoute?: string;
  deliveryState?: string;
  deliveryAttempts?: number;
  unacknowledged?: boolean;
  cancelReason?: string;
  sessionEnded?: boolean;
  sessionArchived?: boolean;
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
    getGuidanceRequest: builder.query<PlainGuidanceRequest, string>({
      queryFn: async (id) => {
        try {
          const response = await getClient().getGuidanceRequest(create(GetGuidanceRequestRequestSchema, { id }));
          if (!response.request) return { error: { status: -1, error: "Guidance request not found" } };
          return { data: toPlainObject(response.request) as unknown as PlainGuidanceRequest };
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Failed to fetch guidance request";
          return { error: { status: -1, error: msg } };
        }
      },
      providesTags: (_result, _error, id) => [{ type: "GuidanceRequests", id }],
    }),
    listSavedGuidanceAnswers: builder.query<PlainGuidanceRequest[], void>({
      queryFn: async () => {
        try {
          const response = await getClient().listSavedGuidanceAnswers(create(ListSavedGuidanceAnswersRequestSchema, {}));
          return { data: response.requests.map((r) => toPlainObject(r) as unknown as PlainGuidanceRequest) };
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Failed to fetch saved answers";
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

export const {
  useListGuidanceRequestsQuery,
  useAnswerGuidanceRequestMutation,
  useGetGuidanceRequestQuery,
  useListSavedGuidanceAnswersQuery,
} = guidanceApi;
