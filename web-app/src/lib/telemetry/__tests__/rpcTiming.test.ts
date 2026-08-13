import { createRpcTimingInterceptor } from "../rpcTiming";
import type { AnalyticsProvider } from "@/lib/analytics/types";

// Minimal ConnectRPC request stub
function makeReq(methodName = "ListSessions") {
  return {
    method: { name: methodName },
    url: "http://localhost/api",
  };
}

// Build an interceptor call chain: interceptor(next)(req)
async function callInterceptor(
  interceptor: ReturnType<typeof createRpcTimingInterceptor>,
  methodName: string,
  shouldThrow: boolean,
) {
  const req = makeReq(methodName);
  const next = jest.fn(async (_req: any) => {
    if (shouldThrow) {
      throw new Error("rpc error");
    }
    return { message: {} };
  });

  const handler = interceptor(next as any);
  try {
    return await handler(req as any);
  } catch {
    // swallow — we test the finally block side effects
  }
}

describe("createRpcTimingInterceptor", () => {
  let performanceMock: {
    mark: jest.Mock;
    measure: jest.Mock;
  };

  beforeEach(() => {
    performanceMock = {
      mark: jest.fn(),
      measure: jest.fn(),
    };
    // Replace global performance with our mock
    Object.defineProperty(global, "performance", {
      value: performanceMock,
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("should_track_rpc_event_on_success", async () => {
    const trackMock = jest.fn();
    const analytics: Pick<AnalyticsProvider, "track"> = { track: trackMock };

    const interceptor = createRpcTimingInterceptor(analytics);
    await callInterceptor(interceptor, "ListSessions", false);

    expect(trackMock).toHaveBeenCalledTimes(1);
    const event = trackMock.mock.calls[0][0];
    expect(event.name).toBe("rpc.ListSessions");
    expect(event.category).toBe("rpc");
    expect(typeof event.durationMs).toBe("number");
    expect(event.durationMs).toBeGreaterThanOrEqual(0);
    expect(event.labels).toMatchObject({ method: "ListSessions", ok: "true" });
  });

  it("should_track_rpc_event_on_error", async () => {
    const trackMock = jest.fn();
    const analytics: Pick<AnalyticsProvider, "track"> = { track: trackMock };

    const interceptor = createRpcTimingInterceptor(analytics);
    await callInterceptor(interceptor, "CreateSession", true);

    expect(trackMock).toHaveBeenCalledTimes(1);
    const event = trackMock.mock.calls[0][0];
    expect(event.name).toBe("rpc.CreateSession");
    expect(event.category).toBe("rpc");
    expect(event.labels).toMatchObject({ method: "CreateSession", ok: "false" });
  });

  it("should_not_throw_when_analytics_absent", async () => {
    // No analytics argument — must not throw
    const interceptor = createRpcTimingInterceptor();
    await expect(callInterceptor(interceptor, "DeleteSession", false)).resolves.not.toThrow();
  });
});
