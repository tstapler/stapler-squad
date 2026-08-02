import type { TurnTokenStat } from "@/gen/session/v1/insights_pb";

function totalTokens(turn: TurnTokenStat): number {
  return Number(turn.inputTokens) + Number(turn.outputTokens);
}

/** Sorts turns by total (input+output) tokens, descending — highest-token turn first. */
export function sortTurnsByTokensDesc(turns: TurnTokenStat[]): TurnTokenStat[] {
  return [...turns].sort((a, b) => totalTokens(b) - totalTokens(a));
}

/** Returns 2x the mean total-tokens-per-turn, the outlier threshold. Returns 0 for an empty list. */
export function computeOutlierThreshold(turns: TurnTokenStat[]): number {
  if (turns.length === 0) return 0;
  const mean = turns.reduce((sum, t) => sum + totalTokens(t), 0) / turns.length;
  return mean * 2;
}

/** True when a turn's total tokens strictly exceed the outlier threshold. */
export function isOutlierTurn(turn: TurnTokenStat, threshold: number): boolean {
  return totalTokens(turn) > threshold;
}
