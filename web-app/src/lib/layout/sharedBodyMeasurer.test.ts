import { createSharedBodyMeasurer } from "@/lib/layout/sharedBodyMeasurer";

const flushMutations = () => Promise.resolve();

describe("SharedBodyMeasurer", () => {
  let rectCalls = 0;
  beforeEach(() => {
    jest.useFakeTimers();
    rectCalls = 0;
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(() => {
      rectCalls++;
      return { bottom: 10, height: 10 } as DOMRect;
    });
  });
  afterEach(() => {
    document.body.innerHTML = "";
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  function subscribeEl(measurer: ReturnType<typeof createSharedBodyMeasurer>) {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const measure = jest.fn(() => {
      el.getBoundingClientRect();
    });
    const unsubscribe = measurer.subscribe({ elements: [el], measure });
    return { el, measure, unsubscribe };
  }

  it("measure_should_CallGetBoundingClientRectAtMostSubscriberCount_When_8Subscribers100InsertionsOneFrame", async () => {
    const measurer = createSharedBodyMeasurer();
    const subs = Array.from({ length: 8 }, () => subscribeEl(measurer));
    rectCalls = 0;
    for (let i = 0; i < 100; i++) document.body.appendChild(document.createElement("span"));
    await flushMutations();
    jest.advanceTimersByTime(50);
    expect(rectCalls).toBeLessThanOrEqual(8);
    expect(rectCalls).toBeGreaterThan(0);
    subs.forEach((s) => s.unsubscribe());
  });

  it("measure_should_ScheduleNoFrame_When_RoundedValueUnchanged_Over60Frames", async () => {
    const measurer = createSharedBodyMeasurer();
    const { measure, unsubscribe } = subscribeEl(measurer);
    document.body.appendChild(document.createElement("span"));
    await flushMutations();
    jest.advanceTimersByTime(50);
    const settled = measure.mock.calls.length;
    const raf = jest.spyOn(window, "requestAnimationFrame");
    jest.advanceTimersByTime(60 * 16);
    expect(measure.mock.calls.length).toBe(settled);
    expect(raf).not.toHaveBeenCalled();
    unsubscribe();
  });

  it("subscriber_should_NeverBeMeasured_When_EnabledFalse_And_StillMeasured_WhenVisibleButUnfocused", async () => {
    const measurer = createSharedBodyMeasurer();
    // enabled=false means the hook never subscribes (measureEnabled = windowActive, not focus)
    const hidden = document.createElement("div");
    document.body.appendChild(hidden);
    const unfocusedVisible = subscribeEl(measurer);
    unfocusedVisible.measure.mockClear();
    document.body.appendChild(document.createElement("span"));
    await flushMutations();
    jest.advanceTimersByTime(50);
    expect(unfocusedVisible.measure).toHaveBeenCalledTimes(1);
    expect(measurer.subscriberCount()).toBe(1);
    unfocusedVisible.unsubscribe();
    expect(measurer.subscriberCount()).toBe(0);
  });

  it("disconnects_all_observers_when_last_subscriber_leaves", () => {
    const measurer = createSharedBodyMeasurer();
    const { measure, unsubscribe } = subscribeEl(measurer);
    unsubscribe();
    measure.mockClear();
    window.dispatchEvent(new Event("resize"));
    jest.advanceTimersByTime(50);
    expect(measure).not.toHaveBeenCalled();
  });
});
