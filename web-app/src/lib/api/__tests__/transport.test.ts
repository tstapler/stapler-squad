import { getConnectTransport, _resetTransportForTesting } from "../transport";

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn((opts: { baseUrl: string }) => ({ baseUrl: opts.baseUrl, _tag: "transport" })),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543"),
}));

const { createConnectTransport } = jest.requireMock("@connectrpc/connect-web");
const { getApiBaseUrl } = jest.requireMock("@/lib/config");

beforeEach(() => {
  _resetTransportForTesting();
  jest.clearAllMocks();
  (getApiBaseUrl as jest.Mock).mockReturnValue("http://localhost:8543");
});

describe("getConnectTransport", () => {
  it("creates a transport on first call", () => {
    const t = getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledTimes(1);
    expect(createConnectTransport).toHaveBeenCalledWith({ baseUrl: "http://localhost:8543" });
    expect(t).toBeDefined();
  });

  it("returns the same instance on subsequent calls (singleton)", () => {
    const t1 = getConnectTransport();
    const t2 = getConnectTransport();
    expect(t1).toBe(t2);
    expect(createConnectTransport).toHaveBeenCalledTimes(1);
  });

  it("uses the URL from getApiBaseUrl at call time", () => {
    (getApiBaseUrl as jest.Mock).mockReturnValue("https://custom.host:9000");
    getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledWith({ baseUrl: "https://custom.host:9000" });
  });
});

describe("_resetTransportForTesting", () => {
  it("forces a new transport to be created after reset", () => {
    getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledTimes(1);

    _resetTransportForTesting();
    getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledTimes(2);
  });
});
