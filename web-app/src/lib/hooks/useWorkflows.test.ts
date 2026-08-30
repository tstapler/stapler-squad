/**
 * Tests for useWorkflows' updateWorkflow request-shaping — specifically that
 * expectedUpdatedAt (AC9's CAS precondition) actually reaches the constructed
 * UpdateWorkflowRequest. TriggerFormModal.test.tsx and TriggersPanel.test.tsx prove
 * their components CALL updateWorkflow with the right args (via a mocked hook), but
 * neither exercises this file's own request-building code — the one place the CAS
 * field is actually attached to the network request. Found missing during
 * sdd:6-verify's testing-quality review.
 */

import { renderHook, act } from "@testing-library/react";
import { useWorkflows } from "@/lib/hooks/useWorkflows";

const mockUpdateWorkflow = jest.fn().mockResolvedValue({});
const mockListWorkflows = jest.fn().mockResolvedValue({ workflows: [] });

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listWorkflows: mockListWorkflows,
    updateWorkflow: mockUpdateWorkflow,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => fields,
}));

describe("useWorkflows.updateWorkflow request shaping", () => {
  beforeEach(() => {
    mockUpdateWorkflow.mockClear();
  });

  it("useWorkflows_should_IncludeExpectedUpdatedAt_When_Provided", async () => {
    const { result } = renderHook(() => useWorkflows());
    const expectedUpdatedAt = { seconds: 100n, nanos: 0 };

    await act(async () => {
      await result.current.updateWorkflow("wf-1", { name: "New Name", expectedUpdatedAt: expectedUpdatedAt as never });
    });

    expect(mockUpdateWorkflow).toHaveBeenCalledTimes(1);
    const req = mockUpdateWorkflow.mock.calls[0][0] as Record<string, unknown>;
    expect(req.expectedUpdatedAt).toBe(expectedUpdatedAt);
  });

  it("useWorkflows_should_OmitExpectedUpdatedAt_When_NotProvided", async () => {
    const { result } = renderHook(() => useWorkflows());

    await act(async () => {
      await result.current.updateWorkflow("wf-1", { name: "New Name" });
    });

    expect(mockUpdateWorkflow).toHaveBeenCalledTimes(1);
    const req = mockUpdateWorkflow.mock.calls[0][0] as Record<string, unknown>;
    expect("expectedUpdatedAt" in req).toBe(false);
  });
});
