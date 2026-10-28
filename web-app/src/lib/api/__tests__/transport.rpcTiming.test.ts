import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport, _resetTransportForTesting } from "../transport";
import { setRpcTimingSink } from "@/lib/telemetry/rpcTiming";

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({
    unary: jest.fn(async (method: { O: unknown }) => ({
      stream: false,
      service: undefined,
      method,
      header: new Headers(),
      trailer: new Headers(),
      message: {},
    })),
    stream: jest.fn(),
  })),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543"),
  createAuthInterceptor: jest.fn(() => (next: unknown) => next),
}));

const track = jest.fn();
const mark = jest.fn();
const measure = jest.fn();

beforeEach(() => {
  _resetTransportForTesting();
  jest.clearAllMocks();
  // jsdom's performance lacks mark/measure, so install stubs (and remove them afterwards).
  Object.assign(performance, { mark, measure });
  jest.spyOn(console, "debug").mockImplementation(() => {});
});

afterEach(() => {
  setRpcTimingSink(undefined);
  delete (performance as Partial<Performance>).mark;
  delete (performance as Partial<Performance>).measure;
  jest.restoreAllMocks();
});

const listShells = () => createClient(SessionService, getConnectTransport()).listShells({ sessionId: "s" });

describe("shared transport rpc timing", () => {
  it("sharedTransport_should_CarryRpcTimingInterceptor_When_AnalyticsRegistered_AndUseShellsBuildsNoOwn", async () => {
    setRpcTimingSink({ track });
    await listShells();
    expect(measure).toHaveBeenCalledWith("rpc:ListShells", expect.anything());
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith(expect.objectContaining({ name: "rpc.ListShells", category: "rpc" }));
    expect(jest.requireMock("@connectrpc/connect-web").createConnectTransport).toHaveBeenCalledTimes(1);
  });

  it("rpcTimingSink_should_BeNoOp_When_NoAnalyticsProviderRegistered", async () => {
    setRpcTimingSink(undefined);
    await expect(listShells()).resolves.toBeDefined();
    expect(measure).toHaveBeenCalledTimes(1); // the Performance entry is still emitted
    expect(track).not.toHaveBeenCalled();
  });
});
