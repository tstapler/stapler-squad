import type { TurnTokenStat } from "@/gen/session/v1/insights_pb";

/** A turn's total tokens must exceed this multiple of the mean to be flagged as an outlier. */
const OUTLIER_MULTIPLIER = 2;

function totalTokens(turn: TurnTokenStat): number {
  return (
    Number(turn.inputTokens) +
    Number(turn.outputTokens) +
    Number(turn.cacheCreationTokens) +
    Number(turn.cacheReadTokens)
  );
}

/** Sorts turns by total (input+output) tokens, descending — highest-token turn first. */
export function sortTurnsByTokensDesc(turns: TurnTokenStat[]): TurnTokenStat[] {
  return [...turns].sort((a, b) => totalTokens(b) - totalTokens(a));
}

/** Returns OUTLIER_MULTIPLIER x the mean total-tokens-per-turn. Returns 0 for an empty list. */
export function computeOutlierThreshold(turns: TurnTokenStat[]): number {
  if (turns.length === 0) return 0;
  const mean = turns.reduce((sum, t) => sum + totalTokens(t), 0) / turns.length;
  return mean * OUTLIER_MULTIPLIER;
}

/** True when a turn's total tokens strictly exceed the outlier threshold. */
export function isOutlierTurn(turn: TurnTokenStat, threshold: number): boolean {
  return totalTokens(turn) > threshold;
}
