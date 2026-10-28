/** Story 2.1.6b: the capabilities RPC is module-cached across FilesTabs. */
const mockGetCaps = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getFileTransportCapabilities: mockGetCaps }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/files/transport/fullFileTransport", () => ({
  createFullFileTransport: () => ({
    id: "full",
    capabilities: () => ({ protocolVersion: 0, features: [], ops: [], wires: [], capsEpoch: 0, limits: {} }),
  }),
}));
jest.mock("@/lib/files/transport/connectUnaryTransport", () => ({
  createConnectUnaryTransport: () => ({ id: "connect_unary" }),
}));

import { createDefaultFileTransport, resetCapabilitiesCacheForTests } from "@/lib/hooks/useFileTransport";

type Probe = { fetchCapabilities: (s: string, e?: number) => Promise<unknown> };
// The router hands its options to fetchCapabilities; reach it via the private field.
const fetcher = (t: unknown) => (t as { opts: Probe }).opts.fetchCapabilities;

describe("useFileTransport capabilities cache", () => {
  beforeEach(() => {
    resetCapabilitiesCacheForTests();
    mockGetCaps.mockReset();
    mockGetCaps.mockResolvedValue({ capabilities: { protocolVersion: 1, capsEpoch: 3n } });
  });

  it("capabilities_should_FetchOnce_When_EightRoutersAskConcurrently", async () => {
    const routers = Array.from({ length: 8 }, () => createDefaultFileTransport());
    await Promise.all(routers.map((r, i) => fetcher(r)(`s${i}`)));
    await Promise.all(routers.map((r, i) => fetcher(r)(`s${i}`)));
    expect(mockGetCaps).toHaveBeenCalledTimes(1);
  });

  it("capabilities_should_Refetch_When_ObservedEpochDiffers", async () => {
    const r = createDefaultFileTransport();
    await fetcher(r)("s1");
    await fetcher(r)("s1", 3);
    expect(mockGetCaps).toHaveBeenCalledTimes(1);
    mockGetCaps.mockResolvedValue({ capabilities: { protocolVersion: 1, capsEpoch: 4n } });
    await fetcher(r)("s1", 4);
    expect(mockGetCaps).toHaveBeenCalledTimes(2);
  });

  it("capabilities_should_NotCacheFailure_When_RpcRejects", async () => {
    mockGetCaps.mockRejectedValueOnce(new Error("boom"));
    const r = createDefaultFileTransport();
    await expect(fetcher(r)("s1")).rejects.toThrow("boom");
    await fetcher(r)("s1");
    expect(mockGetCaps).toHaveBeenCalledTimes(2);
  });
});
