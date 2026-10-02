import { reduce, type GestureEvent, type GestureState, type GestureEffect } from "../gestureMachine";

const move = (absDy: number, touchCount = 1, absDx = 0): GestureEvent => ({
  type: "touchmove",
  touchCount,
  absDx,
  absDy,
  slopPx: 15,
});
const end = (
  totalDy: number,
  elapsedMs: number,
  flags: { flinging?: boolean; consumedByCoast?: boolean; selectionActive?: boolean } = {},
): GestureEvent => ({
  type: "touchend",
  totalDy,
  elapsedMs,
  longPressMs: 400,
  tapTolerancePx: 15,
  flinging: false,
  consumedByCoast: false,
  selectionActive: false,
  ...flags,
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
  // S9 rows added by Stories 1.2.2 / 1.2.10
  ["horizontal-first move past slop cancels (no scroll, no preventDefault)", "PENDING", move(3, 1, 40), "CANCELLED", ["abort"]],
  ["horizontal drift under slop stays pending", "PENDING", move(3, 1, 15), "PENDING", []],
  ["diagonal move with dy dominant past slop scrolls", "PENDING", move(40, 1, 20), "SCROLLING", ["clearLongPressTimer", "beginScroll", "preventDefault"]],
  ["scrolling fling release coasts and suppresses the click", "SCROLLING", end(80, 300, { flinging: true }), "COASTING", ["startMomentum", "preventDefault"]],
  ["coasting touchstart cancels momentum and marks consumedByCoast (no long-press timer)", "COASTING", { type: "touchstart", touchCount: 1 }, "PENDING", ["abort", "setConsumedByCoast"]],
  ["coasting multi-touch touchstart aborts", "COASTING", { type: "touchstart", touchCount: 2 }, "IDLE", ["abort"]],
  ["coasting momentum end returns to idle", "COASTING", { type: "momentumEnd" }, "IDLE", ["abort"]],
  ["momentum end outside coasting is ignored", "IDLE", { type: "momentumEnd" }, "IDLE", []],
  ["coasting touchcancel resets to idle", "COASTING", { type: "touchcancel" }, "IDLE", ["abort"]],
  ["coasting interrupt cancels", "COASTING", { type: "interrupt" }, "CANCELLED", ["abort"]],
  ["coasting stray move is ignored", "COASTING", move(80), "COASTING", []],
  ["coasting stray touchend does not stop momentum", "COASTING", end(0, 50), "COASTING", []],
  ["touch-to-stop release is not a tap and suppresses the click", "PENDING", end(0, 50, { consumedByCoast: true }), "IDLE", ["abort", "preventDefault"]],
  ["tap with a selection active clears it and does not tap", "PENDING", end(5, 100, { selectionActive: true }), "IDLE", ["clearLongPressTimer", "clearSelection"]],
];

describe("gestureMachine.reduce", () => {
  it.each(TABLE)("%s", (_name, from, event, to, effects) => {
    const next = reduce(from, event);
    expect(next.state).toBe(to);
    expect(next.effects).toEqual(effects);
  });
});
