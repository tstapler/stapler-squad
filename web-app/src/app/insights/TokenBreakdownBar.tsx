// +feature: insights-dashboard
import { fmtTokens } from "./insightsFormatters";

interface Props {
  input: bigint;
  output: bigint;
  cacheCreation: bigint;
  cacheRead: bigint;
}

const SEGMENTS = [
  { key: "input", label: "Input", color: "#4c78a8" },
  { key: "output", label: "Output", color: "#f58518" },
  { key: "cacheCreation", label: "Cache write", color: "#b279a2" },
  { key: "cacheRead", label: "Cache read", color: "#54a24b" },
] as const;

/** Stacked bar of a session's four token types, with a labelled legend. */
export function TokenBreakdownBar(props: Props) {
  const total = SEGMENTS.reduce((sum, s) => sum + Number(props[s.key]), 0);
  if (total === 0) return <p data-testid="token-breakdown-empty">No token usage recorded.</p>;

  return (
    <div data-testid="token-breakdown">
      <div
        role="img"
        aria-label={`Token breakdown: ${SEGMENTS.map((s) => `${s.label} ${fmtTokens(props[s.key])}`).join(", ")}`}
        style={{ display: "flex", height: 12, borderRadius: 6, overflow: "hidden" }}
      >
        {SEGMENTS.map((s) => (
          <div
            key={s.key}
            data-testid={`token-segment-${s.key}`}
            style={{ width: `${(Number(props[s.key]) / total) * 100}%`, background: s.color }}
          />
        ))}
      </div>
      <ul style={{ display: "flex", flexWrap: "wrap", gap: "4px 12px", listStyle: "none", padding: 0, margin: "6px 0 0", fontSize: 12 }}>
        {SEGMENTS.map((s) => (
          <li key={s.key}>
            <span aria-hidden style={{ display: "inline-block", width: 8, height: 8, background: s.color, marginRight: 4 }} />
            {s.label}: {fmtTokens(props[s.key])}
          </li>
        ))}
      </ul>
    </div>
  );
}
