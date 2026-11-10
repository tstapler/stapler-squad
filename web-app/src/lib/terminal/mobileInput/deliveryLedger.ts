/** Per-unit delivery bookkeeping. DOM-free. `sent` means handed to the transport, never "arrived". */
import { asDeliveryEpoch, asUnitSeq, type DeliveryEpoch, type SendResult, type UnitSeq } from "./types";

export interface LedgerUnit {
  seq: UnitSeq;
  epoch: DeliveryEpoch;
  /** Composition region the unit belongs to; 0 for units outside any composition. */
  region: number;
  result: SendResult;
  /** Region text the layer had delivered once this unit landed (the diff base after it). */
  baseAfter: string;
}

const MAX_UNITS = 512;

export class DeliveryLedger {
  private units: LedgerUnit[] = [];
  private seq = 0;
  private epochValue = 0;

  get epoch(): DeliveryEpoch {
    return asDeliveryEpoch(this.epochValue);
  }
  get size(): number {
    return this.units.length;
  }

  nextSeq(): UnitSeq {
    return asUnitSeq(++this.seq);
  }

  bumpEpoch(): DeliveryEpoch {
    this.epochValue++;
    return this.epoch;
  }

  record(unit: LedgerUnit): void {
    this.units.push(unit);
    if (this.units.length > MAX_UNITS) this.units.splice(0, this.units.length - MAX_UNITS);
  }

  has(seq: UnitSeq): boolean {
    return this.units.some((u) => u.seq === seq);
  }

  /**
   * Text of the region the transport accepted: the base after the latest non-dropped unit.
   * A `queued` unit counts, because the queue replays in order or discards everything (ADR-004).
   */
  regionBase(region: number): string {
    for (let i = this.units.length - 1; i >= 0; i--) {
      const u = this.units[i];
      if (u.region === region && u.result !== "dropped") return u.baseAfter;
    }
    return "";
  }

  /** Forgets every unit; sequence numbers and the epoch keep counting. */
  clear(): void {
    this.units = [];
  }
}
