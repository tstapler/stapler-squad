import { renderHook } from "@testing-library/react";
import { useSessionDetailActivity } from "../useSessionDetailActivity";

describe("useSessionDetailActivity", () => {
  it("activity_should_ReturnSlowPollAndNoKeys_When_WindowActiveTrueIsFocusedFalse", () => {
    const cases = [
      { isFocused: true, windowActive: true, expected: { pollMode: "fast", keysEnabled: true, measureEnabled: true } },
      { isFocused: false, windowActive: true, expected: { pollMode: "slow", keysEnabled: false, measureEnabled: true } },
      { isFocused: true, windowActive: false, expected: { pollMode: "off", keysEnabled: false, measureEnabled: false } },
      { isFocused: false, windowActive: false, expected: { pollMode: "off", keysEnabled: false, measureEnabled: false } },
    ] as const;
    for (const { isFocused, windowActive, expected } of cases) {
      const { result } = renderHook(() => useSessionDetailActivity({ isFocused, windowActive }));
      expect(result.current).toEqual(expected);
    }
  });
});
