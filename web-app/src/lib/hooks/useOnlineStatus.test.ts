import { act, renderHook } from "@testing-library/react";
import { useOnlineStatus } from "./useOnlineStatus";

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, "onLine", { configurable: true, value });
  act(() => void window.dispatchEvent(new Event(value ? "online" : "offline")));
}

describe("useOnlineStatus", () => {
  afterEach(() => setOnline(true));

  it("useOnlineStatus_should_flipWithinTwoSecondsAndRecover_When_onlineOfflineEvents", () => {
    const { result } = renderHook(() => useOnlineStatus());
    expect(result.current).toBe(true);
    setOnline(false);
    expect(result.current).toBe(false);
    setOnline(true);
    expect(result.current).toBe(true);
  });
});
