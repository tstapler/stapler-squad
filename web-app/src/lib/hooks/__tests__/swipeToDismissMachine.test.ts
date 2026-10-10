import {
  IDLE_SWIPE,
  swipeStep,
  type SwipeSample,
  type SwipeState,
  type SwipeStep,
} from "@/lib/hooks/swipeToDismissMachine";

function run(samples: SwipeSample[]): SwipeStep[] {
  const steps: SwipeStep[] = [];
  let state: SwipeState = IDLE_SWIPE;
  for (const sample of samples) {
    const step = swipeStep(state, sample);
    steps.push(step);
    state = step.state;
  }
  return steps;
}

const start = (width = 360): SwipeSample => ({ type: "start", x: 0, y: 0, t: 0, width });

describe("swipeToDismissMachine", () => {
  it("swipe_machine_should_return_tracking_dismiss_or_cancel_and_offset_for_reveal", () => {
    const steps = run([
      start(),
      { type: "move", x: 5, y: 1, t: 20 },
      { type: "move", x: 40, y: 3, t: 80 },
      { type: "move", x: 90, y: 5, t: 150 },
      { type: "end", x: 90, y: 5, t: 200 },
    ]);

    expect(steps[0].state.phase).toBe("tracking");
    expect(steps[1].state.phase).toBe("tracking"); // under the 10px lock
    expect(steps[1].offset).toBe(0);
    expect(steps[2].state.phase).toBe("dragging");
    expect(steps[2].offset).toBe(40);
    expect(steps[3].offset).toBe(90);
    expect(steps[4].outcome).toBe("dismiss"); // 90px in 200ms is a fling
  });

  it("cancels when vertical drift reaches 10px, so the list scrolls instead", () => {
    const steps = run([start(), { type: "move", x: 30, y: 20, t: 50 }, { type: "move", x: 90, y: 60, t: 100 }]);
    expect(steps[1].outcome).toBe("cancel");
    expect(steps[1].offset).toBe(0);
    // Once cancelled, the rest of the touch is ignored: no late dismissal.
    expect(steps[2].outcome).toBeUndefined();
  });

  it("dismisses a slow drag only past 40% of the width, otherwise springs back", () => {
    const slowFar = run([start(200), { type: "move", x: 90, y: 0, t: 2_000 }, { type: "end", x: 90, y: 0, t: 2_100 }]);
    expect(slowFar[2].outcome).toBe("dismiss"); // 90 >= 0.4 * 200

    const slowShort = run([start(360), { type: "move", x: 90, y: 0, t: 2_000 }, { type: "end", x: 90, y: 0, t: 2_100 }]);
    expect(slowShort[2].outcome).toBe("cancel"); // 90 < 144 and slow
  });

  it("treats a stationary tap as no outcome", () => {
    const steps = run([start(), { type: "move", x: 2, y: 1, t: 30 }, { type: "end", x: 2, y: 1, t: 60 }]);
    expect(steps[2].outcome).toBeUndefined();
  });

  it("supports a leftward drag and a cancel event mid-drag", () => {
    const left = run([start(), { type: "move", x: -50, y: 0, t: 40 }, { type: "end", x: -90, y: 0, t: 150 }]);
    expect(left[1].offset).toBe(-50);
    expect(left[2].outcome).toBe("dismiss");

    const interrupted = run([start(), { type: "move", x: 40, y: 0, t: 40 }, { type: "cancel" }]);
    expect(interrupted[2].outcome).toBe("cancel");
  });
});
