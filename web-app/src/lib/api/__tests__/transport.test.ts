import { getConnectTransport, getWatchTransport, _resetTransportForTesting } from "../transport";

const mockAuthInterceptor = jest.fn();

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn((opts: { baseUrl: string }) => ({ baseUrl: opts.baseUrl, _tag: "transport" })),
}));

jest.mock("@/lib/transport/watch-ws-transport", () => ({
  createSessionWatchTransport: jest.fn((opts: { baseUrl: string }) => ({ baseUrl: opts.baseUrl, _tag: "watchTransport" })),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543"),
  createAuthInterceptor: jest.fn(() => mockAuthInterceptor),
}));

const { createConnectTransport } = jest.requireMock("@connectrpc/connect-web");
const { createSessionWatchTransport } = jest.requireMock("@/lib/transport/watch-ws-transport");
const { getApiBaseUrl } = jest.requireMock("@/lib/config");

beforeEach(() => {
  _resetTransportForTesting();
  jest.clearAllMocks();
  (getApiBaseUrl as jest.Mock).mockReturnValue("http://localhost:8543");
});

describe("getConnectTransport", () => {
  it("creates a transport on first call, with the auth interceptor", () => {
    const t = getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledTimes(1);
    expect(createConnectTransport).toHaveBeenCalledWith({
      baseUrl: "http://localhost:8543",
      defaultTimeoutMs: 30_000,
      interceptors: [mockAuthInterceptor],
    });
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
    expect(createConnectTransport).toHaveBeenCalledWith({
      baseUrl: "https://custom.host:9000",
      defaultTimeoutMs: 30_000,
      interceptors: [mockAuthInterceptor],
    });
  });
});

describe("getWatchTransport", () => {
  it("creates a watch transport on first call, with the auth interceptor", () => {
    const t = getWatchTransport();
    expect(createSessionWatchTransport).toHaveBeenCalledTimes(1);
    expect(createSessionWatchTransport).toHaveBeenCalledWith({
      baseUrl: "http://localhost:8543",
      interceptors: [mockAuthInterceptor],
    });
    expect(t).toBeDefined();
  });

  it("returns the same instance on subsequent calls (singleton)", () => {
    const t1 = getWatchTransport();
    const t2 = getWatchTransport();
    expect(t1).toBe(t2);
    expect(createSessionWatchTransport).toHaveBeenCalledTimes(1);
  });

  it("is independent from the plain unary transport singleton", () => {
    const unary = getConnectTransport();
    const watch = getWatchTransport();
    expect(unary).not.toBe(watch);
    expect(createConnectTransport).toHaveBeenCalledTimes(1);
    expect(createSessionWatchTransport).toHaveBeenCalledTimes(1);
  });
});

describe("_resetTransportForTesting", () => {
  it("forces a new unary transport to be created after reset", () => {
    getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledTimes(1);

    _resetTransportForTesting();
    getConnectTransport();
    expect(createConnectTransport).toHaveBeenCalledTimes(2);
  });

  it("forces a new watch transport to be created after reset", () => {
    getWatchTransport();
    expect(createSessionWatchTransport).toHaveBeenCalledTimes(1);

    _resetTransportForTesting();
    getWatchTransport();
    expect(createSessionWatchTransport).toHaveBeenCalledTimes(2);
  });
});
