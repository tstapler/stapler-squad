import { act, renderHook } from "@testing-library/react";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import { resolveSessionMultiPr, useSessionMultiPR } from "./sessionMultiPr";

let mockFlags: FeatureFlag[] | null = null;
let notify: () => void = () => {};
jest.mock("./flagStore", () => ({
  loadFlags: () => Promise.resolve(mockFlags ?? []),
  peekFlags: () => mockFlags,
  subscribeFlags: (l: () => void) => { notify = l; return () => {}; },
}));

const flag = (global: boolean, scopes: Record<string, boolean> = {}) =>
  [{ name: "session_multi_pr", enabled: global, description: "", statusDetail: "", scopes: Object.entries(scopes).map(([scope, enabled]) => ({ scope, enabled })) }] as unknown as FeatureFlag[];
const allReaders = { "reader:poller": true, "reader:backlog": true, "reader:actions": true, "reader:ui": true };

describe("resolveSessionMultiPr", () => {
  it("resolve_should_beOn_When_EveryReaderUpToUiIsOn", () => {
    expect(resolveSessionMultiPr(flag(false, allReaders), "reader:ui")).toBe(true);
  });

  it("resolve_should_beOff_When_AnEarlierReaderIsOff_EvenIfUiIsOn", () => {
    expect(resolveSessionMultiPr(flag(false, { ...allReaders, "reader:backlog": false }), "reader:ui")).toBe(false);
    expect(resolveSessionMultiPr(flag(false, { ...allReaders, "reader:backlog": false }), "reader:poller")).toBe(true);
  });

  it("resolve_should_useTheGlobalValue_When_ScopeHasNoOverride", () => {
    expect(resolveSessionMultiPr(flag(true), "reader:ui")).toBe(true);
    expect(resolveSessionMultiPr(flag(false), "reader:ui")).toBe(false);
    expect(resolveSessionMultiPr([], "reader:ui")).toBe(false);
  });
});

describe("useSessionMultiPR", () => {
  it("hook_should_flipOnNextStoreNotificationWithoutAReload", async () => {
    mockFlags = flag(false, { ...allReaders, "reader:ui": false });
    const { result } = renderHook(() => useSessionMultiPR());
    await act(async () => {});
    expect(result.current).toBe(false);
    mockFlags = flag(false, allReaders);
    await act(async () => notify());
    expect(result.current).toBe(true);
  });
});
