/** @jest-environment node */
// Story 5.5: push-sw.js notificationclick logic (public/push-sw-logic.js) and the page-side target filter.
import { notificationClickTarget } from "../usePushNotifications";

// eslint-disable-next-line @typescript-eslint/no-require-imports
const logic = require("../../../../public/push-sw-logic.js");

const ORIGIN = "https://ssq.example";
const URL_ = "/?session=h1&tab=terminal&notification=n1";

function client(over: Record<string, unknown> = {}) {
  return {
    url: `${ORIGIN}/sessions`,
    focused: false,
    visibilityState: "hidden",
    focus: jest.fn().mockResolvedValue(undefined),
    postMessage: jest.fn(),
    ...over,
  };
}

function run(clients: ReturnType<typeof client>[], url: string | undefined = URL_) {
  const openWindow = jest.fn().mockResolvedValue(undefined);
  const p = logic.handleNotificationClick({ url, origin: ORIGIN, matchAll: async () => clients, openWindow });
  return { p, openWindow };
}

describe("push-sw handoff", () => {
  it("focuses an open client and posts the URL; openWindow is not called", async () => {
    const c = client();
    const { p, openWindow } = run([c]);
    await expect(p).resolves.toEqual({ handedOff: true, url: URL_ });
    expect(c.focus).toHaveBeenCalledTimes(1);
    expect(c.postMessage).toHaveBeenCalledWith({ type: "notification-click", url: URL_ });
    expect(openWindow).not.toHaveBeenCalled();
  });

  it("opens a window only when no client exists", async () => {
    const { p, openWindow } = run([]);
    await expect(p).resolves.toEqual({ handedOff: false, url: URL_ });
    expect(openWindow).toHaveBeenCalledWith(URL_);
  });

  it("ignores clients of another origin", async () => {
    const other = client({ url: "https://evil.example/" });
    const { p, openWindow } = run([other]);
    await p;
    expect(other.postMessage).not.toHaveBeenCalled();
    expect(openWindow).toHaveBeenCalledWith(URL_);
  });

  it("prefers the focused client, then a visible one", async () => {
    const hidden = client();
    const visible = client({ visibilityState: "visible" });
    const focused = client({ focused: true });
    expect(logic.pickClient([hidden, visible, focused], ORIGIN)).toBe(focused);
    expect(logic.pickClient([hidden, visible], ORIGIN)).toBe(visible);
    expect(logic.pickClient([hidden], ORIGIN)).toBe(hidden);
  });

  it("falls back to openWindow when focus is refused", async () => {
    const c = client({ focus: jest.fn().mockRejectedValue(new Error("not allowed")) });
    const { p, openWindow } = run([c]);
    await expect(p).resolves.toEqual({ handedOff: false, url: URL_ });
    expect(openWindow).toHaveBeenCalledWith(URL_);
    // The existing window must not also navigate, or one click opens the target twice.
    expect(c.postMessage).not.toHaveBeenCalled();
  });

  it("opens a window and does not report a handoff when postMessage throws after focus", async () => {
    const c = client({ postMessage: jest.fn(() => { throw new Error("detached"); }) });
    const { p, openWindow } = run([c]);
    await expect(p).resolves.toEqual({ handedOff: false, url: URL_ });
    expect(openWindow).toHaveBeenCalledWith(URL_);
  });

  it("collapses missing, cross-origin and malformed URLs to /", () => {
    expect(logic.normalizeTargetUrl(undefined, ORIGIN)).toBe("/");
    expect(logic.normalizeTargetUrl("https://evil.example/x", ORIGIN)).toBe("/");
    expect(logic.normalizeTargetUrl(`${ORIGIN}/?session=a`, ORIGIN)).toBe("/?session=a");
  });
});

describe("notificationClickTarget", () => {
  it("accepts a click message with a same-origin path", () => {
    expect(notificationClickTarget({ type: "notification-click", url: URL_ })).toBe(URL_);
  });
  it.each([
    [null],
    [{ type: "SKIP_WAITING" }],
    [{ type: "notification-click", url: "https://evil.example/" }],
    [{ type: "notification-click", url: "//evil.example/" }],
    [{ type: "notification-click" }],
  ])("rejects %j", (data) => {
    expect(notificationClickTarget(data)).toBeNull();
  });
});
