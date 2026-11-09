import { DeliveryLedger } from "./deliveryLedger";
import { InputEngine, type EngineOptions } from "./inputEngine";
import { DEFAULT_POLICY } from "./policy";
import { NO_MODIFIERS } from "./keymap";
import { ModelTerminal } from "./replayHarness";
import { ev } from "./testHelpers";
import { asUnitSeq, type ImeAnomaly, type SendResult } from "./types";

class Wire {
  up = true;
  dropNext = 0;
  units: Array<{ bytes: string; result: SendResult }> = [];
  term = new ModelTerminal();
  anomalies: ImeAnomaly[] = [];
  readonly engine: InputEngine;
  constructor(over: Partial<EngineOptions> = {}) {
    this.engine = new InputEngine({
      policy: DEFAULT_POLICY,
      deps: { getModifiers: () => NO_MODIFIERS, consumeModifiers: () => undefined, bracketedPasteEnabled: () => false },
      isReadOnly: () => false,
      onAnomaly: (a) => this.anomalies.push(a),
      onData: (bytes) => {
        const result: SendResult = !this.up || this.dropNext-- > 0 ? "dropped" : "sent";
        this.units.push({ bytes, result });
        if (result === "sent") this.term.apply(bytes);
        return result;
      },
      ...over,
    });
  }
  get sent(): string {
    return this.units.filter((u) => u.result === "sent").map((u) => u.bytes).join("");
  }
  update(region: string, t = 0) {
    return this.engine.dispatch(ev.beforeinput("insertCompositionText", region, region, t));
  }
  end(region: string, t = 0) {
    return this.engine.dispatch(ev.compositionend(region, t));
  }
}

describe("DeliveryLedger", () => {
  it("ledger_should_AssignMonotonicSeqAndBumpEpoch_When_BoundaryCrossed", () => {
    const l = new DeliveryLedger();
    const a = l.nextSeq();
    const b = l.nextSeq();
    expect(b).toBeGreaterThan(a);
    const e0 = l.epoch;
    expect(l.bumpEpoch()).toBeGreaterThan(e0);
  });

  it("ledger_should_ReturnBaseOfLastDeliveredUnitInRegion_When_LaterUnitDropped", () => {
    const l = new DeliveryLedger();
    l.record({ seq: l.nextSeq(), epoch: l.epoch, region: 1, result: "sent", baseAfter: "he" });
    l.record({ seq: l.nextSeq(), epoch: l.epoch, region: 1, result: "dropped", baseAfter: "hel" });
    l.record({ seq: l.nextSeq(), epoch: l.epoch, region: 2, result: "sent", baseAfter: "x" });
    expect(l.regionBase(1)).toBe("he");
    expect(l.regionBase(3)).toBe("");
  });

  it("ledger_should_CountQueuedAsDeliveredButNotDropped_When_ComputingBase", () => {
    const l = new DeliveryLedger();
    l.record({ seq: l.nextSeq(), epoch: l.epoch, region: 1, result: "queued", baseAfter: "he" });
    expect(l.regionBase(1)).toBe("he");
  });

  it("ledger_should_ForgetUnitsButKeepSeqMonotonic_When_Cleared", () => {
    const l = new DeliveryLedger();
    const s = l.nextSeq();
    l.record({ seq: s, epoch: l.epoch, region: 1, result: "sent", baseAfter: "h" });
    l.clear();
    expect(l.has(s)).toBe(false);
    expect(l.nextSeq()).toBeGreaterThan(s);
  });

  it("ledger_should_StayBounded_When_ManyUnitsRecorded", () => {
    const l = new DeliveryLedger();
    const first = l.nextSeq();
    l.record({ seq: first, epoch: l.epoch, region: 1, result: "sent", baseAfter: "" });
    for (let i = 0; i < 2000; i++) l.record({ seq: l.nextSeq(), epoch: l.epoch, region: 1, result: "sent", baseAfter: "" });
    expect(l.has(first)).toBe(false);
    expect(l.size).toBeLessThanOrEqual(512);
  });
});

describe("delivery rules through the engine", () => {
  it("ledger_should_EmitInsertOnlyTailLp_When_HeSentLDroppedAndRegionBecomesHelp", () => {
    const w = new Wire();
    w.update("h");
    w.update("he");
    w.dropNext = 1;
    w.update("hel"); // dropped
    w.up = true;
    const afterRegion = w.update("help");
    expect(afterRegion.resetTextarea).toBe(false);
    expect(w.units.map((u) => u.bytes)).toEqual(["h", "e", "l"]); // nothing emitted when the region became "help"
    w.end("help");
    expect(w.units.map((u) => u.bytes)).toEqual(["h", "e", "l", "lp"]);
    expect(w.units.at(-1)?.bytes).not.toMatch(/\x7f/);
    expect(w.term.text).toBe("help");
  });

  it("ledger_should_EmitZeroBytesAndAnomaly_When_RegionAutocorrectsToWorldAfterDrop", () => {
    const w = new Wire();
    w.update("h");
    w.update("he");
    w.dropNext = 1;
    w.update("hel");
    w.update("world");
    w.end("world");
    expect(w.units).toHaveLength(3);
    expect(w.anomalies).toEqual(["unrecognized_sequence"]);
    expect(w.term.text).toBe("he");
    expect(w.term.overDeletes).toBe(0);
  });

  it("ledger_should_EmitWholeFinalWithoutDel_When_AllKeystrokesDroppedWhileDisconnected", () => {
    const w = new Wire();
    w.up = false;
    for (const r of ["t", "te", "teh"]) w.update(r);
    w.up = true;
    w.update("the");
    w.end("the");
    expect(w.sent).toBe("the");
    expect(w.units.some((u) => u.bytes.includes("\x7f"))).toBe(false);
  });

  it("ledger_should_DemoteToDeferred_When_ThrownSendTreatedAsDropped", () => {
    let n = 0;
    const w = new Wire({
      onData: (bytes) => {
        if (++n === 2) throw new Error("x");
        w.term.apply(bytes);
        return "sent";
      },
    });
    w.update("h");
    w.update("he"); // throws: dropped
    w.update("hex");
    w.end("hex");
    expect(w.term.text).toBe("hex");
    expect(w.anomalies).toContain("unrecognized_sequence");
  });

  it("engine_should_RejectUnknownDiscardSeqAsAnomalyAndAbandon_When_ReportNotInLedger", () => {
    const w = new Wire();
    w.update("h");
    w.engine.reportQueueDiscard({ deliveredThroughSeq: asUnitSeq(9999) });
    expect(w.anomalies).toEqual(["unrecognized_sequence"]);
    w.update("hel");
    w.end("hel");
    expect(w.sent).toBe("h");
  });

  it("engine_should_StartNextCompositionFromEmptyBase_When_QueueDiscardedAfterPartialFlush", () => {
    const w = new Wire();
    w.update("h");
    w.update("he");
    const seq = asUnitSeq(2);
    w.engine.reportQueueDiscard({ deliveredThroughSeq: seq });
    expect(w.anomalies).toEqual([]);
    w.update("help");
    w.end("help");
    expect(w.sent).toBe("he");
    w.update("x");
    w.end("x");
    expect(w.sent).toBe("hex");
    expect(w.units.at(-1)?.bytes).toBe("x");
  });
});

describe("sink FIFO contract", () => {
  it("engine_should_HandUnitsToSinkInEmissionOrder_When_StubQueueHoldsEarlierUnit", () => {
    const queue: string[] = ["he"];
    const wire: string[] = [];
    const e = new InputEngine({
      policy: DEFAULT_POLICY,
      deps: { getModifiers: () => NO_MODIFIERS, consumeModifiers: () => undefined, bracketedPasteEnabled: () => false },
      isReadOnly: () => false,
      onAnomaly: () => undefined,
      onData: (b) => {
        if (queue.length) {
          queue.push(b);
          return "queued";
        }
        wire.push(b);
        return "sent";
      },
    });
    e.dispatch(ev.beforeinput("insertText", "l"));
    wire.push(...queue.splice(0));
    expect(wire).toEqual(["he", "l"]);
  });
});
