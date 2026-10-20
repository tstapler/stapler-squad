/** Counts discrete user actions from opening Search to opening a hit (ADR-035 section 5). */
export class FindFlowMeter {
  private count: number | null = null;

  /** Search panel opened: starts a flow with the open itself as interaction 1. */
  open(): void {
    this.count = 1;
  }

  /** One tap or chord (typing a query is one); ignored while no flow is running. */
  interact(): void {
    if (this.count !== null) this.count += 1;
  }

  /** The hit tap counts too. Returns the total and resets, or null if no flow was running. */
  finish(): number | null {
    if (this.count === null) return null;
    const total = this.count + 1;
    this.count = null;
    return total;
  }

  /** Cancel or panel close: discard without emitting. */
  cancel(): void {
    this.count = null;
  }
}
