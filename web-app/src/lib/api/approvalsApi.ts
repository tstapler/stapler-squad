import { connectApi } from "./connectApi";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import {
  ListPendingApprovalsRequestSchema,
  ResolveApprovalRequestSchema,
} from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
import { toPlainObject } from "@/lib/api/serialization";

export interface PlainApproval {
  id: string;
  sessionId: string;
  toolName: string;
  toolInput: { [key: string]: string };
  cwd: string;
  permissionMode: string;
  createdAt: Record<string, unknown> | undefined;
  expiresAt: Record<string, unknown> | undefined;
  secondsRemaining: number;
  riskLevel: string;
}

function getClient() {
  const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
  return createClient(SessionService, transport);
}

export const approvalsApi = connectApi.injectEndpoints({
  endpoints: (builder) => ({
    getApprovals: builder.query<{ approvals: PlainApproval[] }, void>({
      queryFn: async () => {
        try {
          const client = getClient();
          const req = create(ListPendingApprovalsRequestSchema, {});
          const response = await client.listPendingApprovals(req);
          // Serialize to plain objects (boundary rule: no protobuf instances in Redux).
          const approvals = response.approvals.map((a) => toPlainObject(a) as unknown as PlainApproval);
          return { data: { approvals } };
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Failed to fetch approvals";
          return { error: { status: -1, error: msg } };
        }
      },
      providesTags: ["Approvals"],
    }),
    resolveApproval: builder.mutation<void, { approvalId: string; decision: "allow" | "deny"; message?: string }>({
      queryFn: async ({ approvalId, decision, message }) => {
        try {
          const client = getClient();
          const req = create(ResolveApprovalRequestSchema, { approvalId, decision, message });
          await client.resolveApproval(req);
          return { data: undefined };
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Failed to resolve approval";
          return { error: { status: -1, error: msg } };
        }
      },
      invalidatesTags: ["Approvals"],
    }),
  }),
});

export const { useGetApprovalsQuery, useResolveApprovalMutation } = approvalsApi;
