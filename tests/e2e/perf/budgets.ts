/** Per-method request budgets for the 16 s load scenario (Story 1.1.3), shared by the perf spec and its harness tests. */
export const loadBudgets = (durationS: number): Record<string, number> => ({
  GetFeatureFlags: 1,
  ListProgramsConfig: 1,
  LogClientEvents: Math.ceil(durationS / 5) + 1,
  GetFileTransportCapabilities: 1,
  ListWorkflows: 1,
});

/** Budget entries (`Rpc<=max`) whose median request count exceeds the budget. */
export function budgetViolations(requestsByRpc: Record<string, { median: number | null }>, durationS: number): string[] {
  const budgets = loadBudgets(durationS);
  return Object.entries(budgets)
    .filter(([rpc, max]) => (requestsByRpc[rpc]?.median ?? 0) > max)
    .map(([rpc, max]) => `${rpc}<=${max}`);
}

/** Violations already recorded in baseline.json `expectedFail` are annotated, never skipped; anything else fails. */
export function classifyViolations(violations: string[], known: string[]): { unexpected: string[]; expectedFails: string[] } {
  const knownSet = new Set(known);
  return {
    unexpected: violations.filter((v) => !knownSet.has(v)),
    expectedFails: violations.filter((v) => knownSet.has(v)),
  };
}
