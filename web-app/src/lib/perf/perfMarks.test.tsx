import { act } from "@testing-library/react";
import { useRef } from "react";
import { useRouter } from "@/lib/nav/impl.memory";
import { renderWithNav } from "@/lib/nav/testing";
import { useFocusOnNavigate } from "@/lib/nav/useFocusOnNavigate";
import { markWindowSwitchEnd, markWindowSwitchStart, _resetPerfMarksForTesting } from "./perfMarks";

jest.mock("@/lib/nav/impl.router", () => jest.requireActual("@/lib/nav/impl.memory"));

let rafQueue: FrameRequestCallback[] = [];
const flushFrame = () => {
  const q = rafQueue;
  rafQueue = [];
  q.forEach((cb) => cb(0));
};

// jsdom has no User Timing; this fake keeps the same start/duration semantics.
type Entry = { name: string; startTime: number; duration: number };
let entries: Entry[] = [];
function installFakeUserTiming() {
  entries = [];
  const perf = performance as unknown as Record<string, unknown>;
  perf.mark = (name: string) => {
    entries.push({ name, startTime: performance.now(), duration: 0 });
  };
  perf.measure = (name: string, a: string | { start: number; end: number }, b?: string) => {
    if (typeof a === "object") {
      entries.push({ name, startTime: a.start, duration: a.end - a.start });
      return;
    }
    const from = entries.find((e) => e.name === a)!;
    const to = entries.find((e) => e.name === b)!;
    entries.push({ name, startTime: from.startTime, duration: to.startTime - from.startTime });
  };
  perf.getEntriesByName = (name: string) => entries.filter((e) => e.name === name);
}

beforeEach(() => {
  installFakeUserTiming();
  rafQueue = [];
  jest.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
    rafQueue.push(cb);
    return rafQueue.length;
  });
  _resetPerfMarksForTesting();
});
afterEach(() => jest.restoreAllMocks());

describe("window switch measure", () => {
  it("windowSwitchMeasure_should_StartBeforeKeydownReturns_And_EndAfterDoubleRafFitAndFocus", () => {
    const keydownReturned = { at: 0 };
    document.addEventListener("keydown", () => {
      markWindowSwitchStart();
      keydownReturned.at = performance.now();
    }, { once: true });
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "2" }));
    markWindowSwitchEnd();
    expect(performance.getEntriesByName("ssq:window-switch")).toHaveLength(0);
    flushFrame();
    expect(performance.getEntriesByName("ssq:window-switch")).toHaveLength(0);
    flushFrame();
    const [m] = performance.getEntriesByName("ssq:window-switch");
    expect(m.startTime).toBeLessThanOrEqual(keydownReturned.at);
    expect(m.duration).toBeGreaterThanOrEqual(0);
  });

  it("windowSwitchEnd_should_NotMeasure_When_NoSwitchStarted", () => {
    markWindowSwitchEnd();
    flushFrame();
    flushFrame();
    expect(performance.getEntriesByName("ssq:window-switch")).toHaveLength(0);
  });
});

function Harness() {
  const ref = useRef<HTMLElement>(null);
  const router = useRouter();
  useFocusOnNavigate(ref);
  return (
    <main ref={ref} tabIndex={-1}>
      <button onClick={() => router.push("/backlog")}>nav</button>
      <button onClick={() => router.push("/?window=a2")}>search</button>
    </main>
  );
}

describe("route change measure", () => {
  it("routeChange_should_RecordMeasure_When_PathnameChanges_And_NoneWhenOnlySearchChanges", () => {
    const { getByText } = renderWithNav(<Harness />, { initialEntries: ["/"] });
    const fire = (el: HTMLElement) => {
      el.dispatchEvent(new Event("pointerdown", { bubbles: true }));
      act(() => el.click());
    };
    fire(getByText("search"));
    flushFrame();
    flushFrame();
    expect(performance.getEntriesByName("ssq:route-change")).toHaveLength(0);
    fire(getByText("nav"));
    flushFrame();
    flushFrame();
    expect(performance.getEntriesByName("ssq:route-change")).toHaveLength(1);
  });
});
