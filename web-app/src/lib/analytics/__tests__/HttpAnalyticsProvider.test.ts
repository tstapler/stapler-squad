import { HttpAnalyticsProvider } from "../HttpAnalyticsProvider";
import type { AnalyticsEvent } from "../types";

const makeEvent = (name = "test_event"): AnalyticsEvent => ({
  name,
  category: "user_action",
});

describe("HttpAnalyticsProvider", () => {
  let fetchMock: jest.Mock;

  beforeEach(() => {
    jest.useFakeTimers();
    fetchMock = jest.fn().mockResolvedValue({ ok: true });
    global.fetch = fetchMock;
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it("should_batch_and_flush_after_100_events", async () => {
    const provider = new HttpAnalyticsProvider();

    for (let i = 0; i < 100; i++) {
      provider.track(makeEvent(`event_${i}`));
    }

    // Flush is triggered asynchronously via void this.flush()
    await Promise.resolve();
    await Promise.resolve();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string) as { events: AnalyticsEvent[] };
    expect(body.events).toHaveLength(100);
  });

  it("should_flush_after_20s_timer_not_before", async () => {
    const provider = new HttpAnalyticsProvider();

    provider.track(makeEvent());

    jest.advanceTimersByTime(19_999);
    await Promise.resolve();
    expect(fetchMock).not.toHaveBeenCalled();

    jest.advanceTimersByTime(2);
    await Promise.resolve();
    await Promise.resolve();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("should_send_one_post_per_20s_window_for_a_steady_trickle_of_events", async () => {
    const provider = new HttpAnalyticsProvider();

    // 90 s idle with an event every 2 s: was one POST per 2 s window (45), now one per 20 s.
    for (let t = 0; t < 90; t += 2) {
      provider.track(makeEvent());
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    }

    expect(fetchMock.mock.calls.length).toBeLessThanOrEqual(5);
  });

  it("should_drain_the_queue_with_sendBeacon_when_the_tab_is_hidden", async () => {
    const sendBeacon = jest.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: sendBeacon });
    const provider = new HttpAnalyticsProvider();
    await provider.initialize();
    provider.track(makeEvent());

    Object.defineProperty(document, "hidden", { configurable: true, get: () => true });
    document.dispatchEvent(new Event("visibilitychange"));
    Object.defineProperty(document, "hidden", { configurable: true, get: () => false });

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("should_not_exceed_max_queue_size", () => {
    const provider = new HttpAnalyticsProvider();

    // Track 201 events without flushing (we need to prevent flush from firing)
    // Each 100th push triggers a flush, resetting the queue.
    // To test bounded queue in isolation, we inspect the internal queue.
    // We'll track 200 items (8 full batches = flushes), then add one more to hit the 201st
    // and verify the queue stays <= 200 between flushes.

    // Spy on flush to prevent actual fetch calls but also prevent queue drain
    const flushSpy = jest.spyOn(provider, "flush").mockResolvedValue(undefined);

    for (let i = 0; i < 201; i++) {
      provider.track(makeEvent(`event_${i}`));
    }

    // The queue should never exceed MAX_QUEUE_SIZE (200)
    // After 201 pushes with flush mocked, the queue holds at most 200 items
    // (the 201st push shifts the oldest before inserting)
    const queue = (provider as any).queue as AnalyticsEvent[];
    expect(queue.length).toBeLessThanOrEqual(200);

    flushSpy.mockRestore();
  });

  it("should_flush_on_close", async () => {
    const provider = new HttpAnalyticsProvider();

    provider.track(makeEvent("close_event"));
    expect(fetchMock).not.toHaveBeenCalled();

    await provider.onClose();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("should_not_call_fetch_when_queue_is_empty_on_flush", async () => {
    const provider = new HttpAnalyticsProvider();

    await provider.flush();

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("should_serialize_durationMs_as_integer_duration_ms", async () => {
    // Go backend decodes duration_ms into *int64 — a float JSON value is silently
    // dropped, leaving the column null. Regression test: ensure float durationMs
    // is rounded to an integer and serialized as the snake_case key the backend expects.
    const provider = new HttpAnalyticsProvider();

    provider.track({
      name: "session_attach",
      category: "performance",
      durationMs: 234.56, // performance.now() arithmetic produces floats
      sessionId: "test-session",
      labels: { phase: "attach" },
    });

    await provider.flush();

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string) as { events: Record<string, unknown>[] };
    const event = body.events[0];
    expect(event.duration_ms).toBe(235);        // rounded integer
    expect(event).not.toHaveProperty("durationMs"); // camelCase must not appear
    expect(event.session_id).toBe("test-session");  // snake_case
    expect(event).not.toHaveProperty("sessionId");  // camelCase must not appear
  });

  it("should_omit_duration_ms_when_durationMs_is_undefined", async () => {
    const provider = new HttpAnalyticsProvider();

    provider.track({ name: "page_view", category: "navigation", page: "/sessions" });

    await provider.flush();

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string) as { events: Record<string, unknown>[] };
    const event = body.events[0];
    expect(event).not.toHaveProperty("duration_ms");
    expect(event.page).toBe("/sessions");
  });

  it("should_split_a_flush_into_requests_of_at_most_100_events", async () => {
    const provider = new HttpAnalyticsProvider();
    const queue = (provider as unknown as { queue: AnalyticsEvent[] }).queue;
    for (let i = 0; i < 200; i++) queue.push(makeEvent(`event_${i}`));

    await provider.flush();

    const sizes = fetchMock.mock.calls.map((c) => (JSON.parse(c[1].body as string) as { events: unknown[] }).events.length);
    expect(sizes).toEqual([100, 100]);
  });

  it("should_send_one_beacon_chunk_per_drain_and_keep_the_rest_queued", async () => {
    const sendBeacon = jest.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: sendBeacon });
    const provider = new HttpAnalyticsProvider();
    await provider.initialize();
    const queue = (provider as unknown as { queue: AnalyticsEvent[] }).queue;
    const padding = "x".repeat(900);
    for (let i = 0; i < 150; i++) queue.push({ ...makeEvent(`event_${i}`), labels: { padding } });

    window.dispatchEvent(new Event("pagehide"));

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    const body = sendBeacon.mock.calls[0][1] as string;
    expect(body.length).toBeLessThanOrEqual(60_000);
    const sent = (JSON.parse(body) as { events: unknown[] }).events.length;
    expect(sent).toBeLessThanOrEqual(100);
    expect(queue.length).toBe(150 - sent);
    queue.length = 0; // window listeners outlive the provider, so leave nothing for later tests to drain
  });

  it("should_fall_back_to_a_keepalive_fetch_when_sendBeacon_refuses_the_chunk", async () => {
    const sendBeacon = jest.fn().mockReturnValue(false);
    Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: sendBeacon });
    const provider = new HttpAnalyticsProvider();
    await provider.initialize();
    provider.track(makeEvent());

    window.dispatchEvent(new Event("pagehide"));

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][1].keepalive).toBe(true);
  });

  it("should_requeue_the_chunk_when_both_beacon_and_fetch_fail", async () => {
    Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: jest.fn().mockReturnValue(false) });
    fetchMock.mockRejectedValue(new Error("offline"));
    const provider = new HttpAnalyticsProvider();
    await provider.initialize();
    provider.track(makeEvent());

    window.dispatchEvent(new Event("pagehide"));
    await Promise.resolve();
    await Promise.resolve();

    const queue = (provider as unknown as { queue: AnalyticsEvent[] }).queue;
    expect(queue).toHaveLength(1);
    queue.length = 0;
  });

  it("should_register_the_hide_listeners_only_once", async () => {
    const sendBeacon = jest.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: sendBeacon });
    const provider = new HttpAnalyticsProvider();
    await provider.initialize();
    await provider.initialize();
    provider.track(makeEvent());
    provider.track(makeEvent());

    window.dispatchEvent(new Event("pagehide"));

    expect(sendBeacon).toHaveBeenCalledTimes(1);
  });
});
