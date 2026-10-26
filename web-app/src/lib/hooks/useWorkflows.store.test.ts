/** Story 2.1.6d: useWorkflows reads one shared store (1 ListWorkflows for N consumers). */
import { renderHook, act } from "@testing-library/react";

const mockListWorkflows = jest.fn();
// eslint-disable-next-line @typescript-eslint/require-await -- async generator body intentionally yields nothing
async function* emptyStream() {}
const mockWatchWorkflows = jest.fn().mockImplementation(() => emptyStream());

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ listWorkflows: mockListWorkflows, watchWorkflows: mockWatchWorkflows }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}), getWatchTransport: () => ({}) }));
jest.mock("@bufbuild/protobuf", () => ({ create: (_s: unknown, f: Record<string, unknown> = {}) => f }));

import { useWorkflows, resetWorkflowsStoreForTests } from "@/lib/hooks/useWorkflows";

describe("useWorkflows shared store", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    resetWorkflowsStoreForTests();
    mockListWorkflows.mockReset();
    mockListWorkflows.mockResolvedValue({ workflows: [{ id: "w1" }] });
    mockWatchWorkflows.mockClear();
  });
  afterEach(() => jest.useRealTimers());

  it("useWorkflows_should_IssueOneListWorkflows_When_EightConsumersMount", async () => {
    const views = Array.from({ length: 8 }, () => renderHook(() => useWorkflows({ watch: false })));
    await act(async () => { await Promise.resolve(); });
    expect(mockListWorkflows).toHaveBeenCalledTimes(1);
    views.forEach((v) => expect(v.result.current.workflows).toEqual([{ id: "w1" }]));
  });

  it("useWorkflows_should_RunBackstopOnceGlobally_When_EightConsumersMounted", async () => {
    Array.from({ length: 8 }, () => renderHook(() => useWorkflows({ watch: false })));
    await act(async () => { await Promise.resolve(); });
    await act(async () => { await jest.advanceTimersByTimeAsync(90_000); });
    expect(mockListWorkflows).toHaveBeenCalledTimes(2);
  });

  it("useWorkflows_should_NotOpenWatchStream_When_WatchFalse", async () => {
    renderHook(() => useWorkflows({ watch: false }));
    await act(async () => { await Promise.resolve(); });
    expect(mockWatchWorkflows).not.toHaveBeenCalled();
  });

  it("useWorkflows_should_StopBackstop_When_LastConsumerUnmounts", async () => {
    const a = renderHook(() => useWorkflows({ watch: false }));
    await act(async () => { await Promise.resolve(); });
    a.unmount();
    await act(async () => { await jest.advanceTimersByTimeAsync(180_000); });
    expect(mockListWorkflows).toHaveBeenCalledTimes(1);
  });
});
