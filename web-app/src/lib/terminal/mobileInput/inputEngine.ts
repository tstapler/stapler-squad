/**
 * Pure executor core shared by the DOM adapter and the replay harness: reduces events, hands each
 * Emit to the sink as a sequenced unit, records the result in the delivery ledger and applies the
 * drop and boundary rules of ADR-004. DOM-free; the adapter owns the textarea and the clock.
 */
import { abandonRegion, demoteToDeferred } from "./composition";
import { DeliveryLedger } from "./deliveryLedger";
import { reduceWithKeymap, type KeymapDeps } from "./keymap";
import { resetValue, type MobileInputPolicy } from "./policy";
import { graphemes } from "./rewrite";
import {
  asTerminalBytes,
  type DeliveryReport,
  type ImeAnomaly,
  type InputCommand,
  type InvalidateReason,
  type MobileInputDiagnostic,
  type MobileInputEvent,
  type MobileInputState,
  type OnData,
  type SendResult,
} from "./types";

export interface EngineOptions {
  policy: MobileInputPolicy;
  deps: KeymapDeps;
  onData: OnData;
  isReadOnly(): boolean;
  /** Receives the anomaly class only, never typed data. */
  onAnomaly(anomaly: ImeAnomaly): void;
  onDiagnostic?(entry: MobileInputDiagnostic): void;
  /** A multi-line paste was joined with spaces because bracketed paste is off. */
  onPasteFlattened?(): void;
}

export interface DispatchOutcome {
  /** True only for keydown events the layer fully handled. */
  preventDefault: boolean;
  /** The caller must put the textarea back to its reset value. */
  resetTextarea: boolean;
}

type EmitCommand = Extract<InputCommand, { kind: "Emit" }>;

export class InputEngine {
  state: MobileInputState;
  readonly ledger = new DeliveryLedger();
  private regionId = 0;

  constructor(private readonly opts: EngineOptions) {
    this.state = { kind: "idle", shadow: resetValue(opts.policy), swallow: [], inputExpected: false };
  }

  /** Graphemes held back while a region waits for its commit, or null when nothing is deferred. */
  get deferredWaiting(): number | null {
    const s = this.state;
    return s.kind === "composing" && s.mode === "deferred" ? graphemes(s.region).length : null;
  }

  dispatch(event: MobileInputEvent): DispatchOutcome {
    const prev = this.state;
    const r = reduceWithKeymap(prev, event, this.opts.policy, this.opts.deps);
    this.state = r.state;
    if (r.flattened) this.opts.onPasteFlattened?.();
    if (prev.kind !== "composing" && r.state.kind === "composing") this.regionId++;

    let dropped = false;
    let resetTextarea = false;
    for (const c of r.commands) {
      if (c.kind === "Emit") dropped = this.send(c, r.state) === "dropped" || dropped;
      else if (c.kind === "ResetTextarea") resetTextarea = true;
      else if (c.kind === "Anomaly") this.opts.onAnomaly(c.anomaly);
    }
    // A unit the transport refused leaves the region's remaining text to commit as an insert-only tail.
    if (dropped && this.state.kind === "composing" && this.state.mode === "live") {
      this.state = demoteToDeferred(this.state, this.ledger.regionBase(this.regionId));
    }
    return { preventDefault: r.preventDefault, resetTextarea };
  }

  /** A mapped physical key (Tab, Esc, arrows, Ctrl-x): its own unit outside any composition. */
  emitRaw(bytes: string): void {
    this.send({ kind: "Emit", bytes: asTerminalBytes(bytes), atomic: true }, this.state);
  }

  invalidate(reason: InvalidateReason): void {
    this.ledger.bumpEpoch();
    this.opts.onDiagnostic?.({ event: "invalidate", reason });
    if (reason === "transport") {
      this.state = demoteToDeferred(this.state, this.ledger.regionBase(this.regionId));
      return;
    }
    const abandoned = abandonRegion(this.state);
    this.state = abandoned.kind === "idle" ? { ...abandoned, swallow: [] } : abandoned;
    this.ledger.clear();
  }

  /** The queue discarded units: abandon the region, never re-send. */
  reportQueueDiscard(report: DeliveryReport): void {
    if (report.deliveredThroughSeq !== null && !this.ledger.has(report.deliveredThroughSeq)) {
      this.opts.onAnomaly("unrecognized_sequence");
    }
    this.invalidate("queue_discard");
  }

  private send(cmd: EmitCommand, after: MobileInputState): SendResult {
    const { ledger, opts } = this;
    const seq = ledger.nextSeq();
    let result: SendResult;
    if (opts.isReadOnly()) {
      result = "dropped";
    } else {
      try {
        result = opts.onData(cmd.bytes, seq) ?? "sent";
      } catch {
        result = "dropped";
        opts.onAnomaly("unrecognized_sequence");
      }
    }
    ledger.record({
      seq,
      epoch: ledger.epoch,
      region: after.kind === "composing" ? this.regionId : 0,
      result,
      baseAfter: after.kind === "composing" ? after.sent : "",
    });
    opts.onDiagnostic?.({ event: "unit", seq, epoch: ledger.epoch, result });
    // A dropped or thrown send is a transport boundary.
    if (result === "dropped") ledger.bumpEpoch();
    return result;
  }
}
