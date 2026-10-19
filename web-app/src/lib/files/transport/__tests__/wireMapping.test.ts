import { Code, ConnectError } from "@connectrpc/connect";
import { TransportFailure } from "../fileTransport";
import { mapError } from "../wireMapping";

describe("mapError network failures", () => {
  it("mapError_should_ReturnNetworkTransportFailure_When_FetchRejectsWithTypeError", () => {
    const raw = mapError(new TypeError("Failed to fetch"));
    const wrapped = mapError(ConnectError.from(new TypeError("Load failed")));

    expect(raw).toBeInstanceOf(TransportFailure);
    expect((raw as TransportFailure).reason).toBe("network");
    expect(wrapped).toBeInstanceOf(TransportFailure);
    expect((wrapped as TransportFailure).reason).toBe("network");
  });

  it("mapError_should_PassThroughApplicationErrors_When_NotANetworkFailure", () => {
    const internal = new ConnectError("boom", Code.Internal);
    const plain = new Error("bug");

    expect(mapError(internal)).toBe(internal);
    expect(mapError(plain)).toBe(plain);
  });
});
