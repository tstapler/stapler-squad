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

  it("should_batch_and_flush_after_25_events", async () => {
    const provider = new HttpAnalyticsProvider();

    for (let i = 0; i < 25; i++) {
      provider.track(makeEvent(`event_${i}`));
    }

    // Flush is triggered asynchronously via void this.flush()
    await Promise.resolve();
    await Promise.resolve();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string) as { events: AnalyticsEvent[] };
    expect(body.events).toHaveLength(25);
  });

  it("should_flush_after_2s_timer", async () => {
    const provider = new HttpAnalyticsProvider();

    provider.track(makeEvent());

    expect(fetchMock).not.toHaveBeenCalled();

    jest.advanceTimersByTime(2001);
    await Promise.resolve();
    await Promise.resolve();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("should_not_exceed_max_queue_size", () => {
    const provider = new HttpAnalyticsProvider();

    // Track 201 events without flushing (we need to prevent flush from firing)
    // Each 25th push triggers a flush, resetting the queue.
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
});
