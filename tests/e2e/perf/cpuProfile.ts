/** Aggregation of a CDP `Profiler.stop` profile into self-time per function (port of /tmp/ssqprof/cpu.cjs). */
export interface CpuProfileNode {
  id: number;
  callFrame: { functionName: string; url: string; lineNumber: number };
}
export interface CpuProfile {
  nodes: CpuProfileNode[];
  samples: number[];
  timeDeltas: number[];
}
export interface SelfTimeEntry {
  fn: string;
  selfMs: number;
}
export interface CpuSummary {
  totalSampledMs: number;
  /** Self-time of `getBoundingClientRect` (the forced-layout probe the plan tracks). */
  getBoundingClientRectMs: number;
  top: SelfTimeEntry[];
}

export function summarizeProfile(profile: CpuProfile, topN = 14): CpuSummary {
  const nodes = new Map(profile.nodes.map((n) => [n.id, n]));
  const selfUs = new Map<number, number>();
  profile.samples.forEach((id, i) => selfUs.set(id, (selfUs.get(id) ?? 0) + (profile.timeDeltas[i] ?? 0)));

  const byFn = new Map<string, number>();
  let rectUs = 0;
  for (const [id, us] of selfUs) {
    const c = nodes.get(id)?.callFrame;
    if (!c) continue;
    const name = c.functionName || '(anon)';
    const key = `${name} ${c.url.split('/').slice(-2).join('/')}:${c.lineNumber}`;
    byFn.set(key, (byFn.get(key) ?? 0) + us);
    if (name === 'getBoundingClientRect') rectUs += us;
  }
  const totalUs = [...byFn.values()].reduce((a, b) => a + b, 0);
  const top = [...byFn]
    .sort((a, b) => b[1] - a[1])
    .slice(0, topN)
    .map(([fn, us]) => ({ fn, selfMs: Math.round(us / 1000) }));
  return { totalSampledMs: Math.round(totalUs / 1000), getBoundingClientRectMs: Math.round(rectUs / 1000), top };
}
