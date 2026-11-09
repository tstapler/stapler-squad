import { renderHook } from "@testing-library/react";
import { useMobileInputLayerActive } from "./useMobileInputLayerActive";

function env(ua: string, queries: Record<string, boolean>) {
  Object.defineProperty(navigator, "userAgent", { value: ua, configurable: true });
  window.matchMedia = ((q: string) => ({ matches: queries[q] ?? false })) as unknown as typeof window.matchMedia;
}
const ANDROID = "Mozilla/5.0 (Linux; Android 14; Pixel 7) Chrome/120 Mobile";
const IPHONE = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Safari/604.1";

beforeEach(() => localStorage.clear());

describe("useMobileInputLayerActive", () => {
  it("useMobileInputLayerActive_should_BeFalse_When_FlagTrueAndIphoneUa", () => {
    localStorage.setItem("ssq:flag:mobile_input_layer", "1");
    env(IPHONE, { "(pointer: coarse)": true });
    expect(renderHook(() => useMobileInputLayerActive()).result.current).toBe(false);
  });

  it("useMobileInputLayerActive_should_BeTrue_When_FlagTrueAndAndroidClass", () => {
    localStorage.setItem("ssq:flag:mobile_input_layer", "1");
    env(ANDROID, { "(pointer: coarse)": true });
    expect(renderHook(() => useMobileInputLayerActive()).result.current).toBe(true);
  });

  it("useMobileInputLayerActive_should_BeFalse_When_FlagFalseOnAndroid", () => {
    localStorage.setItem("ssq:flag:mobile_input_layer", "0");
    env(ANDROID, { "(pointer: coarse)": true });
    expect(renderHook(() => useMobileInputLayerActive()).result.current).toBe(false);
  });
});
