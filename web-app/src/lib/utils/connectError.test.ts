import { ConnectError, Code } from "@connectrpc/connect";
import { getErrorMessage } from "./connectError";

describe("getErrorMessage", () => {
  it("getErrorMessage_should_ReturnRawMessage_When_GivenConnectError", () => {
    const err = new ConnectError("repo path does not exist", Code.InvalidArgument);
    expect(getErrorMessage(err, "fallback")).toBe("repo path does not exist");
  });

  it("getErrorMessage_should_ReturnMessage_When_GivenPlainError", () => {
    expect(getErrorMessage(new Error("network down"), "fallback")).toBe(
      "network down",
    );
    expect(getErrorMessage(new TypeError("fetch failed"), "fallback")).toBe(
      "fetch failed",
    );
  });

  it("getErrorMessage_should_ReturnFallback_When_GivenNonErrorValue", () => {
    expect(getErrorMessage("boom", "fallback")).toBe("fallback");
    expect(getErrorMessage(undefined, "fallback")).toBe("fallback");
    expect(getErrorMessage({ foo: "bar" }, "fallback")).toBe("fallback");
  });

  it("getErrorMessage_should_ReturnFallback_When_ConnectErrorHasEmptyRawMessage", () => {
    const err = new ConnectError("", Code.Internal);
    expect(getErrorMessage(err, "Something went wrong.")).toBe(
      "Something went wrong.",
    );
  });
});
