/** How eagerly a view polls: off in a hidden window, slow for a visible unfocused pane, fast for the focused one. */
export type PollMode = "off" | "slow" | "fast";

/** Poll interval for `mode`, or null when polling is off. */
export function pollIntervalMs(mode: PollMode, fastMs: number, slowMs: number): number | null {
  if (mode === "off") return null;
  return mode === "fast" ? fastMs : slowMs;
}
