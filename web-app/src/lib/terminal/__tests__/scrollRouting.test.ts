import type { Terminal } from "@xterm/xterm";
import {
  DEFAULT_ROUTING_POLICY,
  PageAccumulator,
  ROUTING_VERIFIED,
  TUI_SCROLL_POLICY,
  decideAndLogScrollTarget,
  decideScrollTarget,
  encodePageKeys,
  encodeWheel,
  encodeWheelX10,
  tuiPageStepLines,
  toolbarPageAction,
  PAGE_UP_BYTES,
  PAGE_DOWN_BYTES,
  type MouseTrackingMode,
  type ScrollMode,
  type ScrollRoutingPolicy,
} from "../scrollRouting";
import { readScrollMode } from "../mouseTracking";

const TRACKING: MouseTrackingMode[] = ["none", "x10", "vt200", "drag", "any"];
const modes: ScrollMode[] = (["normal", "alternate"] as const).flatMap((bufferType) =>
  TRACKING.map((mouseTrackingMode) => ({ bufferType, mouseTrackingMode })),
);
const cell = { col: 10, row: 5, cols: 80, rows: 24 };

describe("decideScrollTarget", () => {
  it("decideScrollTarget_should_RouteAlternateToTuiPgkeys_When_DefaultTableAndAuto", () => {
    expect(ROUTING_VERIFIED).toBe(false);
    expect(TUI_SCROLL_POLICY).toBe("pgkeys");
    for (const t of TRACKING) {
      expect(decideScrollTarget({ bufferType: "alternate", mouseTrackingMode: t })).toBe("tui-pgkeys");
    }
    expect(decideScrollTarget({ bufferType: "normal", mouseTrackingMode: "none" })).toBe("xterm-local");
    for (const t of TRACKING.filter((x) => x !== "none")) {
      expect(decideScrollTarget({ bufferType: "normal", mouseTrackingMode: t })).toBe("tui-pgkeys");
    }
  });

  it("decideScrollTarget_should_ReturnPerTableRow_When_ModeMatchesRow", () => {
    const table: ScrollRoutingPolicy = {
      rules: [
        { when: { bufferType: "alternate", mouseTrackingMode: "none" }, target: "xterm-local" },
        { when: { bufferType: "normal", mouseTrackingMode: "vt200" }, target: "tui-pgkeys" },
      ],
      default: "xterm-local",
    };
    expect(decideScrollTarget({ bufferType: "alternate", mouseTrackingMode: "none" }, table)).toBe("xterm-local");
    expect(decideScrollTarget({ bufferType: "normal", mouseTrackingMode: "vt200" }, table)).toBe("tui-pgkeys");
    expect(decideScrollTarget({ bufferType: "alternate", mouseTrackingMode: "any" }, table)).toBe("xterm-local");
  });

  it("decideScrollTarget_should_IgnoreTableAndReturnOverride_When_OverrideLocalOrTui", () => {
    for (const m of modes) {
      expect(decideScrollTarget(m, DEFAULT_ROUTING_POLICY, "pgkeys", "local")).toBe("xterm-local");
      expect(decideScrollTarget(m, DEFAULT_ROUTING_POLICY, "pgkeys", "tui")).toBe("tui-pgkeys");
    }
  });

  it("decideScrollTarget_should_NeverReturnTuiWheel_When_PolicyNotWheel", () => {
    for (const m of modes) {
      for (const override of ["auto", "local", "tui"] as const) {
        expect(decideScrollTarget(m, DEFAULT_ROUTING_POLICY, "pgkeys", override)).not.toBe("tui-wheel");
        expect(decideScrollTarget(m, DEFAULT_ROUTING_POLICY, undefined, override)).not.toBe("tui-wheel");
      }
    }
    expect(decideScrollTarget({ bufferType: "alternate", mouseTrackingMode: "any" }, DEFAULT_ROUTING_POLICY, "wheel")).toBe(
      "tui-wheel",
    );
  });

  it("decideScrollTarget_should_LogUnverified_When_RoutingVerifiedFalse", () => {
    const log = jest.fn();
    const mode: ScrollMode = { bufferType: "alternate", mouseTrackingMode: "none" };
    decideAndLogScrollTarget(mode, { log });
    expect(log).toHaveBeenCalledWith(expect.objectContaining({ target: "tui-pgkeys", source: "auto", unverified: true }));
    decideAndLogScrollTarget(mode, { log, override: "local" });
    expect(log).toHaveBeenLastCalledWith(expect.objectContaining({ target: "xterm-local", source: "override" }));
    decideAndLogScrollTarget(mode, { log, routingVerified: true });
    expect(log.mock.calls[2][0]).not.toHaveProperty("unverified");
  });

  it("decideScrollTarget_should_NotReachX10_FromAnyModeOrPolicy", () => {
    const targets = new Set<string>();
    for (const m of modes) {
      for (const p of ["wheel", "pgkeys"] as const) targets.add(decideScrollTarget(m, DEFAULT_ROUTING_POLICY, p));
    }
    expect([...targets].sort()).toEqual(["tui-pgkeys", "tui-wheel", "xterm-local"]);
    expect(encodeWheel(-1, cell)).not.toBe(encodeWheelX10(-1, cell));
  });
});

describe("readScrollMode", () => {
  const term = (type: string, mouse?: string) =>
    ({ buffer: { active: { type } }, modes: mouse === undefined ? undefined : { mouseTrackingMode: mouse } }) as unknown as Terminal;
  it("reads buffer type and tracking mode, defaulting to normal/none", () => {
    expect(readScrollMode(term("alternate", "any"))).toEqual({ bufferType: "alternate", mouseTrackingMode: "any" });
    expect(readScrollMode(term("normal"))).toEqual({ bufferType: "normal", mouseTrackingMode: "none" });
  });
});

describe("encodeWheel", () => {
  it("encodeWheel_should_EmitSgrBytes_ForUpAndDown", () => {
    expect(encodeWheel(-2, cell)).toBe("\x1b[<64;10;5M\x1b[<64;10;5M");
    expect(encodeWheel(3, cell)).toBe("\x1b[<65;10;5M".repeat(3));
    expect(encodeWheel(0, cell)).toBe("");
  });

  it("encodeWheel_should_ClampColRowToViewport_When_OutOfRange", () => {
    expect(encodeWheel(1, { col: 0, row: -4, cols: 80, rows: 24 })).toBe("\x1b[<65;1;1M");
    expect(encodeWheel(1, { col: 500, row: 99, cols: 80, rows: 24 })).toBe("\x1b[<65;80;24M");
  });

  it("encodeWheel_should_CapReportsPerFrameAtThree_When_LargeLineDelta", () => {
    expect(encodeWheel(50, cell)).toBe("\x1b[<65;10;5M".repeat(3));
    expect(encodeWheel(-50, cell)).toBe("\x1b[<64;10;5M".repeat(3));
  });

  it("encodeWheel_should_EncodeX10_When_UsingFallbackEncoder", () => {
    expect(encodeWheelX10(-1, cell)).toBe("\x1b[M" + String.fromCharCode(96, 42, 37));
  });
});

describe("encodePageKeys", () => {
  it("encodePageKeys_should_EmitExactToolbarBytes_ForPgUpAndPgDn", () => {
    expect(encodePageKeys("older")).toBe("\x1b[5~");
    expect(encodePageKeys("newer")).toBe("\x1b[6~");
  });
});

describe("PageAccumulator", () => {
  let t = 0;
  const make = (rows = 24) => new PageAccumulator({ rows, now: () => t });
  beforeEach(() => {
    t = 1000;
  });

  it("step is floor((rows-1)/2), minimum 1", () => {
    expect(tuiPageStepLines(24)).toBe(11);
    expect(tuiPageStepLines(1)).toBe(1);
  });

  it("pageAccumulator_should_EmitZeroKeys_When_TravelBelowHalfPageStep", () => {
    const a = make();
    expect(a.push(-1)).toBe("");
    expect(a.push(-9)).toBe("");
  });

  it("emits one key at the step and carries the remainder (both directions)", () => {
    const a = make();
    expect(a.push(-12)).toBe("\x1b[5~");
    t += 100;
    expect(a.push(-10)).toBe("\x1b[5~"); // carry 1 + 10 = 11
    const b = make();
    expect(b.push(11)).toBe("\x1b[6~");
  });

  it("emits at most one key per frame; 22 lines yields two keys over two frames", () => {
    const a = make();
    expect(a.push(-22)).toBe("\x1b[5~");
    t += 100;
    expect(a.push(0)).toBe("\x1b[5~");
  });

  it("pageAccumulator_should_CapPagesAndRateLimit_When_Fling", () => {
    const a = make();
    expect(a.push(-11)).toBe("\x1b[5~");
    t += 50;
    expect(a.push(-11)).toBe(""); // within 100 ms
    t += 50;
    expect(a.push(0)).toBe("\x1b[5~"); // carried travel emits once window passes
    let keys = 2;
    for (let i = 0; i < 10; i++) {
      t += 100;
      if (a.push(-11)) keys++;
    }
    expect(keys).toBe(5);
    expect(a.capped).toBe(true);
    a.reset();
    expect(a.capped).toBe(false);
  });
});

describe("toolbarPageAction", () => {
  const none = { ctrl: false, alt: false, shift: false };

  it("toolbarPageAction_should_ReturnScrollPages_When_LocalRouteAndNoModifier", () => {
    expect(toolbarPageAction({ route: "xterm-local", direction: "up", modifiers: none, canScroll: true, override: "auto" })).toEqual({
      type: "scroll-pages",
      pages: -1,
    });
    expect(toolbarPageAction({ route: "xterm-local", direction: "down", modifiers: none, canScroll: true, override: "local" })).toEqual({
      type: "scroll-pages",
      pages: 1,
    });
  });

  it("toolbarPageAction_should_ReturnBytes_When_TuiRoute", () => {
    for (const route of ["tui-pgkeys", "tui-wheel"] as const) {
      expect(toolbarPageAction({ route, direction: "up", modifiers: none, canScroll: true, override: "auto" })).toEqual({
        type: "send-keys",
        bytes: PAGE_UP_BYTES,
        countsTowardNetPages: true,
      });
      expect(toolbarPageAction({ route, direction: "down", modifiers: none, canScroll: false, override: "tui" })).toEqual({
        type: "send-keys",
        bytes: PAGE_DOWN_BYTES,
        countsTowardNetPages: true,
      });
    }
  });

  it("toolbarPageAction_should_ReturnModifiedBytes_When_ModifierArmed", () => {
    // The action carries the base key; the caller's sendKey applies the armed modifier map.
    for (const modifiers of [{ ...none, ctrl: true }, { ...none, alt: true }, { ...none, shift: true }]) {
      const action = toolbarPageAction({ route: "xterm-local", direction: "up", modifiers, canScroll: true, override: "local" });
      expect(action).toEqual({ type: "send-keys", bytes: PAGE_UP_BYTES, countsTowardNetPages: false });
    }
  });

  it("toolbarPageAction_should_FallThroughToBytes_When_AutoLocalAtEdge", () => {
    expect(toolbarPageAction({ route: "xterm-local", direction: "up", modifiers: none, canScroll: false, override: "auto" })).toEqual({
      type: "send-keys",
      bytes: PAGE_UP_BYTES,
      countsTowardNetPages: false,
    });
  });

  it("toolbarPageAction_should_NotFallThrough_When_ExplicitLocalAtEdge", () => {
    expect(toolbarPageAction({ route: "xterm-local", direction: "down", modifiers: none, canScroll: false, override: "local" })).toEqual({
      type: "scroll-pages",
      pages: 1,
    });
  });
});
