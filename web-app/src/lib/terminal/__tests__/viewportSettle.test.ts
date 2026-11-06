import { createViewportSettle, type FrameScheduler, type ViewportSource } from "../viewportSettle";

const FRAME_MS = 16;

/** Fake rAF + clock; `step()` advances one frame and runs the pending callback. */
function makeScheduler() {
  let clock = 0;
  let nextHandle = 1;
  const pending = new Map<number, () => void>();
  const scheduler: FrameScheduler = {
    request: (cb) => {
      const h = nextHandle++;
      pending.set(h, cb);
      return h;
    },
    cancel: (h) => void pending.delete(h),
    now: () => clock,
  };
  return {
    scheduler,
    pendingCount: () => pending.size,
    step() {
      clock += FRAME_MS;
      const [h, cb] = [...pending.entries()][0] ?? [];
      if (h !== undefined && cb) {
        pending.delete(h);
        cb();
      }
    },
    clock: () => clock,
  };
}

function makeVv(height = 800, offsetTop = 0) {
  const listeners: Record<string, Set<() => void>> = { resize: new Set(), scroll: new Set() };
  const vv: ViewportSource & { height: number; offsetTop: number } = {
    height,
    offsetTop,
    addEventListener: (t, l) => void listeners[t].add(l),
    removeEventListener: (t, l) => void listeners[t].delete(l),
  };
  const emit = (t: "resize" | "scroll") => listeners[t].forEach((l) => l());
  return { vv, emit, listenerCount: () => listeners.resize.size + listeners.scroll.size };
}

const OPTS = { stableFrames: 3, maxWaitMs: 600 };

describe("createViewportSettle", () => {
  it("viewportSettle_should_FireOnceAtThirdStableFrame_When_HeightSequence800To480", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv();
    const onSettled = jest.fn();
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    const seq = [800, 640, 520, 480, 480, 480, 480];
    emit("resize");
    const firedAt: number[] = [];
    seq.forEach((h, i) => {
      vv.height = h;
      f.step();
      if (onSettled.mock.calls.length > firedAt.length) firedAt.push(i);
    });
    expect(onSettled).toHaveBeenCalledTimes(1);
    expect(firedAt).toEqual([5]);
    expect(onSettled).toHaveBeenCalledWith({ height: 480, offsetTop: 0 });
    expect(f.pendingCount()).toBe(0);
  });

  it("viewportSettle_should_ReArmAfterFiring", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv(480);
    const onSettled = jest.fn();
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    emit("resize");
    for (let i = 0; i < 3; i++) f.step();
    expect(onSettled).toHaveBeenCalledTimes(1);
    vv.height = 800;
    emit("resize");
    for (let i = 0; i < 3; i++) f.step();
    expect(onSettled).toHaveBeenCalledTimes(2);
    expect(onSettled).toHaveBeenLastCalledWith({ height: 800, offsetTop: 0 });
  });

  it("viewportSettle_should_FireAtThirdStableFrame_When_HeightStableAndOffsetTopChanges0To40", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv(480, 0);
    const onSettled = jest.fn();
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    emit("scroll");
    const fired: number[] = [];
    [0, 40, 40, 40, 40].forEach((top, i) => {
      vv.offsetTop = top;
      f.step();
      if (onSettled.mock.calls.length > fired.length) fired.push(i);
    });
    expect(fired).toEqual([3]);
    expect(onSettled).toHaveBeenCalledTimes(1);
    expect(onSettled).toHaveBeenCalledWith({ height: 480, offsetTop: 40 });
  });

  it("viewportSettle_should_ArmOnVisualViewportScroll_When_OnlyOffsetTopChanges", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv(480, 0);
    const onSettled = jest.fn();
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    expect(f.pendingCount()).toBe(0);
    vv.offsetTop = 40;
    emit("scroll");
    expect(f.pendingCount()).toBe(1);
    for (let i = 0; i < 3; i++) f.step();
    expect(onSettled).toHaveBeenCalledWith({ height: 480, offsetTop: 40 });
  });

  it("viewportSettle_should_FireOnceAtMaxWaitWithLatestHeight_When_HeightNeverStabilizes", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv(800);
    const onSettled = jest.fn();
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    emit("resize");
    let h = 800;
    let frames = 0;
    while (onSettled.mock.calls.length === 0 && frames < 200) {
      vv.height = h--;
      f.step();
      frames++;
    }
    expect(onSettled).toHaveBeenCalledTimes(1);
    expect(f.clock()).toBeGreaterThanOrEqual(600);
    expect(f.clock()).toBeLessThanOrEqual(600 + FRAME_MS);
    expect(onSettled).toHaveBeenCalledWith({ height: vv.height, offsetTop: 0 });
    // re-arms on next change
    emit("resize");
    expect(f.pendingCount()).toBe(1);
  });

  it("viewportSettle_should_UseInnerHeightAndNotThrow_When_NoVisualViewport", () => {
    const f = makeScheduler();
    const listeners = new Set<() => void>();
    const win = {
      innerHeight: 700,
      addEventListener: (_t: "resize", l: () => void) => void listeners.add(l),
      removeEventListener: (_t: "resize", l: () => void) => void listeners.delete(l),
    };
    const onSettled = jest.fn();
    let dispose: () => void = () => {};
    expect(() => {
      dispose = createViewportSettle(undefined, f.scheduler, { ...OPTS, onSettled, win });
    }).not.toThrow();
    win.innerHeight = 500;
    listeners.forEach((l) => l());
    for (let i = 0; i < 3; i++) f.step();
    expect(onSettled).toHaveBeenCalledWith({ height: 500, offsetTop: 0 });
    dispose();
    expect(listeners.size).toBe(0);
  });

  it("viewportSettle_should_ArmOnWindowResize_When_NoVisualViewport", () => {
    const f = makeScheduler();
    const listeners = new Set<() => void>();
    const win = {
      innerHeight: 700,
      addEventListener: (_t: "resize", l: () => void) => void listeners.add(l),
      removeEventListener: (_t: "resize", l: () => void) => void listeners.delete(l),
    };
    createViewportSettle(null, f.scheduler, { ...OPTS, onSettled: jest.fn(), win });
    expect(listeners.size).toBe(1);
    listeners.forEach((l) => l());
    expect(f.pendingCount()).toBe(1);
  });

  it("viewportSettle_should_NotFireAfterDispose", () => {
    const f = makeScheduler();
    const { vv, emit, listenerCount } = makeVv(480);
    const onSettled = jest.fn();
    const dispose = createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    emit("resize");
    f.step();
    dispose();
    expect(f.pendingCount()).toBe(0);
    expect(listenerCount()).toBe(0);
    for (let i = 0; i < 5; i++) f.step();
    emit("resize");
    expect(f.pendingCount()).toBe(0);
    expect(onSettled).not.toHaveBeenCalled();
  });

  it("viewportSettle_should_FireWithin400msOfFirstEvent_When_HeightStabilizesWithin300ms", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv(800);
    const onSettled = jest.fn();
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled });
    emit("resize"); // first event at t=0
    const start = f.clock();
    let settledAt = -1;
    while (settledAt < 0 && f.clock() < 1000) {
      const t = f.clock() + FRAME_MS;
      vv.height = t >= 300 ? 480 : 800 - Math.round((t / 300) * 320);
      f.step();
      if (onSettled.mock.calls.length) settledAt = f.clock() - start;
    }
    expect(settledAt).toBeGreaterThan(0);
    expect(settledAt).toBeLessThanOrEqual(400);
  });

  it("viewportSettle_should_NotCollapseNeighbouringEvents_When_EventsFireDuringActiveLoop", () => {
    const f = makeScheduler();
    const { vv, emit } = makeVv(480);
    createViewportSettle(vv, f.scheduler, { ...OPTS, onSettled: jest.fn() });
    emit("resize");
    emit("scroll");
    emit("resize");
    expect(f.pendingCount()).toBe(1);
  });
});
