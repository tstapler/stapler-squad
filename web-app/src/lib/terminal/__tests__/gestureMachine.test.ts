import { reduce, type GestureEvent, type GestureState, type GestureEffect } from "../gestureMachine";

const move = (absDy: number, touchCount = 1): GestureEvent => ({ type: "touchmove", touchCount, absDy, slopPx: 15 });
const end = (totalDy: number, elapsedMs: number): GestureEvent => ({
  type: "touchend",
  totalDy,
  elapsedMs,
  longPressMs: 400,
  tapTolerancePx: 15,
});

type Row = [name: string, from: GestureState, event: GestureEvent, to: GestureState, effects: GestureEffect[]];

const TABLE: Row[] = [
  ["idle single touchstart starts pending + long-press timer", "IDLE", { type: "touchstart", touchCount: 1 }, "PENDING", ["startLongPressTimer"]],
  ["multi-touch touchstart aborts", "IDLE", { type: "touchstart", touchCount: 2 }, "IDLE", ["abort"]],
  ["pending move under slop stays pending", "PENDING", move(15), "PENDING", []],
  ["pending move past slop begins scrolling", "PENDING", move(16), "SCROLLING", ["clearLongPressTimer", "beginScroll", "preventDefault"]],
  ["pending quick short touchend is a tap", "PENDING", end(5, 100), "IDLE", ["clearLongPressTimer", "tap"]],
  ["pending touchend at the tolerance is not a tap", "PENDING", end(15, 100), "IDLE", ["abort"]],
  ["pending slow touchend is not a tap", "PENDING", end(0, 400), "IDLE", ["abort"]],
  ["pending long-press enters selecting", "PENDING", { type: "longPress" }, "SELECTING", ["enterSelecting"]],
  ["scrolling move continues and prevents default", "SCROLLING", move(80), "SCROLLING", ["continueScroll", "preventDefault"]],
  ["scrolling touchend returns to idle and suppresses the click", "SCROLLING", end(80, 300), "IDLE", ["abort", "preventDefault"]],
  ["scrolling second finger aborts", "SCROLLING", move(80, 2), "IDLE", ["abort"]],
  ["selecting move extends selection", "SELECTING", move(80), "SELECTING", ["continueSelect", "preventDefault"]],
  ["selecting touchend finishes the selection", "SELECTING", end(0, 900), "IDLE", ["endSelecting", "abort"]],
  ["touchcancel aborts from scrolling", "SCROLLING", { type: "touchcancel" }, "IDLE", ["abort"]],
  ["touchcancel aborts from selecting", "SELECTING", { type: "touchcancel" }, "IDLE", ["abort"]],
  ["stale long-press after scroll is ignored", "SCROLLING", { type: "longPress" }, "SCROLLING", []],
  ["idle move is ignored", "IDLE", move(100), "IDLE", []],
  ["interrupt cancels a pending touch", "PENDING", { type: "interrupt" }, "CANCELLED", ["abort"]],
  ["interrupt cancels a scrolling touch", "SCROLLING", { type: "interrupt" }, "CANCELLED", ["abort"]],
  ["interrupt leaves selecting alone", "SELECTING", { type: "interrupt" }, "SELECTING", []],
  ["interrupt while idle is a no-op", "IDLE", { type: "interrupt" }, "IDLE", []],
  ["cancelled move is ignored (no scroll, no preventDefault)", "CANCELLED", move(200), "CANCELLED", []],
  ["cancelled touchend returns to idle without tap or preventDefault", "CANCELLED", end(0, 50), "IDLE", ["abort"]],
  ["cancelled touchcancel returns to idle", "CANCELLED", { type: "touchcancel" }, "IDLE", ["abort"]],
  ["cancelled single-finger touchstart begins a new gesture", "CANCELLED", { type: "touchstart", touchCount: 1 }, "PENDING", ["startLongPressTimer"]],
  ["cancelled long-press is ignored", "CANCELLED", { type: "longPress" }, "CANCELLED", []],
];

describe("gestureMachine.reduce", () => {
  it.each(TABLE)("%s", (_name, from, event, to, effects) => {
    const next = reduce(from, event);
    expect(next.state).toBe(to);
    expect(next.effects).toEqual(effects);
  });
});
