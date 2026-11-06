import {
  ScrollAccumulator,
  MomentumTracker,
  MOMENTUM_CONSTANTS,
  SLOP_PX,
  clampLinesPerFrame,
} from "../scrollKinematics";

const CELL_H = 18;

describe("ScrollAccumulator", () => {
  it("push_should_EmitSymmetricLineDeltaAndCarryRemainder_When_UpAndDownThreePxFrames", () => {
    for (const [sign, expectedLines] of [[1, 1], [-1, -1]] as const) {
      const acc = new ScrollAccumulator();
      let total = 0;
      for (let i = 0; i < 10; i++) total += acc.push(sign * 3, CELL_H);
      expect(total).toBe(expectedLines);
      expect(acc.remainder).toBe(sign * 12);
    }
  });

  it("push_should_SeedWithOvershootOnlyAndEmitNoJump_When_Slop15Crossed", () => {
    expect(SLOP_PX).toBe(15);
    const acc = new ScrollAccumulator();
    acc.seed(22 - SLOP_PX);
    expect(acc.remainder).toBe(7);
    expect(acc.push(0, CELL_H)).toBe(0);
    // Later movement is 1:1: 11 more px crosses one 18 px line.
    expect(acc.push(11, CELL_H)).toBe(1);
    expect(acc.remainder).toBe(0);
  });

  it("push_should_ResetRemainder_When_CellHeightChanges", () => {
    const acc = new ScrollAccumulator();
    acc.push(10, CELL_H);
    expect(acc.remainder).toBe(10);
    acc.reset();
    expect(acc.remainder).toBe(0);
    expect(acc.push(10, 20)).toBe(0);
  });

  it("push_should_StayWithinOneLineOfTravel_When_1000Frames", () => {
    const acc = new ScrollAccumulator();
    let total = 0;
    let travel = 0;
    for (let i = 0; i < 1000; i++) {
      const dy = i % 2 === 0 ? 2.7 : -0.9;
      travel += dy;
      total += acc.push(dy, CELL_H);
    }
    expect(Math.abs(total - travel / CELL_H)).toBeLessThanOrEqual(1);
  });

  it("push_should_EmitNothingAndNotCorrupt_When_CellHeightInvalid", () => {
    const acc = new ScrollAccumulator();
    expect(acc.push(50, 0)).toBe(0);
    expect(acc.push(Number.NaN, CELL_H)).toBe(0);
    expect(acc.remainder).toBe(0);
  });
});

describe("MOMENTUM_CONSTANTS", () => {
  it("momentumConstants_should_MatchCanonicalSet", () => {
    expect(MOMENTUM_CONSTANTS).toEqual({
      windowMs: 100,
      minFlingPxPerMs: 0.3,
      decayPerFrame: 0.95,
      frameMs: 16,
      stopPxPerMs: 0.02,
      maxVelocityPxPerMs: 8,
      maxFrames: 120,
    });
  });
});

/** Feeds samples that move at `v` px/ms (negative = upward) for 80 ms ending at t=80. */
function fling(tracker: MomentumTracker, v: number): void {
  for (let t = 0; t <= 80; t += 16) tracker.addSample(t, 500 + v * t);
}

describe("MomentumTracker", () => {
  it("momentum_should_DecayBy095AndEndWithin120Frames_When_Released1p2PxPerMs", () => {
    const m = new MomentumTracker();
    fling(m, -1.2);
    expect(m.release(false)).toBe(true);
    const f = MOMENTUM_CONSTANTS.frameMs;
    const velocities: number[] = [];
    let frames = 0;
    for (;;) {
      const dy = m.step(f);
      if (dy === 0) break;
      velocities.push(dy / f);
      frames++;
      expect(frames).toBeLessThanOrEqual(120);
    }
    expect(velocities[0]).toBeCloseTo(-1.2, 5);
    expect(velocities[1] / velocities[0]).toBeCloseTo(0.95, 5);
    expect(Math.abs(velocities[velocities.length - 1])).toBeGreaterThanOrEqual(0.02);
    expect(frames).toBeGreaterThan(50);
    expect(m.active).toBe(false);
  });

  it("momentum_should_EmitNoFrames_When_ReducedMotionTrue", () => {
    const m = new MomentumTracker();
    fling(m, -1.2);
    expect(m.release(true)).toBe(false);
    expect(m.step(16)).toBe(0);
  });

  it("momentum_should_EmitNoFrames_When_BelowMinFlingVelocity", () => {
    const m = new MomentumTracker();
    fling(m, -0.2);
    expect(m.release(false)).toBe(false);
    expect(m.step(16)).toBe(0);
  });

  it("momentum_should_StopImmediately_When_Cancelled", () => {
    const m = new MomentumTracker();
    fling(m, -1.2);
    m.release(false);
    expect(m.step(16)).not.toBe(0);
    m.cancel();
    expect(m.active).toBe(false);
    expect(m.step(16)).toBe(0);
  });

  it("momentum_should_IgnoreSamplesOutsideVelocityWindow", () => {
    const m = new MomentumTracker();
    // Fast early movement, then 150 ms of nearly stationary samples.
    m.addSample(0, 0);
    m.addSample(10, -100);
    m.addSample(160, -100);
    m.addSample(200, -100);
    expect(m.release(false)).toBe(false);
  });

  it("momentum_should_CapVelocity_When_FlingExceedsMax", () => {
    const m = new MomentumTracker();
    m.addSample(0, 0);
    m.addSample(10, -1000); // 100 px/ms
    expect(m.release(false)).toBe(true);
    expect(m.step(16)).toBeCloseTo(-MOMENTUM_CONSTANTS.maxVelocityPxPerMs * 16, 5);
  });

  it("momentum_should_EndWithinMaxFrames_When_StartingAtVelocityCap", () => {
    const m = new MomentumTracker();
    m.addSample(0, 0);
    m.addSample(10, -80); // capped at 8 px/ms; 8*0.95^n < 0.02 at n=117
    m.release(false);
    let frames = 0;
    while (m.step(16) !== 0) frames++;
    expect(frames).toBe(117);
    expect(frames).toBeLessThanOrEqual(MOMENTUM_CONSTANTS.maxFrames);
  });

  it("momentum_should_ClearPreviousSamples_When_ReleasedTwice", () => {
    const m = new MomentumTracker();
    fling(m, -1.2);
    m.release(false);
    m.cancel();
    expect(m.release(false)).toBe(false);
  });
});

describe("clampLinesPerFrame", () => {
  it("clampLinesPerFrame_should_ClampToRowsBothSigns_When_LargeDelta", () => {
    expect(clampLinesPerFrame(90, 30)).toBe(30);
    expect(clampLinesPerFrame(-90, 30)).toBe(-30);
  });

  it("clampLinesPerFrame_should_PassThrough_When_WithinRows", () => {
    expect(clampLinesPerFrame(5, 30)).toBe(5);
    expect(clampLinesPerFrame(-5, 30)).toBe(-5);
  });
});
