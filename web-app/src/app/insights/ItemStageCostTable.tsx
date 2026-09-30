// +feature: insights-dashboard
import { card, title, table, th, thRight, td, tdRight, emphasis } from "./ItemStageCostTable.css";
import { fmtCost } from "./insightsFormatters";

export interface ItemStageCostRow {
  role: string;
  costUsd: number;
  sessionCount: number;
  unpricedSessionCount: number;
}

function roleLabel(role: string): string {
  if (!role) return "Unattributed";
  return role.charAt(0).toUpperCase() + role.slice(1);
}

/**
 * Story 5.2.3: one backlog item's cost broken down by pipeline stage,
 * sourced from GetInsightsSummaryResponse.role_breakdown[].items filtered to
 * this item's ID (no new RPC — see BacklogItemDetail.tsx's wiring). Renders
 * nothing (not an empty table) when `rows` is empty — an item with no cost
 * data anywhere is legitimately absent from this surface, distinct from a
 * failed fetch, which the caller renders as its own error state.
 */
export function ItemStageCostTable({ rows }: { rows: ItemStageCostRow[] }) {
  if (rows.length === 0) return null;

  return (
    <div className={card} data-testid="item-stage-cost-table">
      <div className={title}>Cost by Stage</div>
      <table className={table}>
        <thead>
          <tr>
            <th className={th} scope="col">Stage</th>
            <th className={thRight} scope="col">Cost</th>
            <th className={thRight} scope="col">Runs</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const isRework = r.sessionCount > 1;
            const runsLabel = `${r.sessionCount} run${r.sessionCount === 1 ? "" : "s"}`;
            return (
              <tr key={r.role}>
                <td className={td}>
                  {roleLabel(r.role)}
                  {r.unpricedSessionCount > 0 && (
                    <span
                      className={emphasis}
                      aria-label={`${r.unpricedSessionCount} unpriced session${r.unpricedSessionCount === 1 ? "" : "s"}`}
                    >
                      {" "}
                      ⚠ {r.unpricedSessionCount} unpriced
                    </span>
                  )}
                </td>
                <td className={tdRight}>{fmtCost(r.costUsd)}</td>
                <td className={tdRight}>
                  {isRework ? (
                    <span className={emphasis} aria-label={`${runsLabel}, rework indicator`}>
                      ⚠ {runsLabel}
                    </span>
                  ) : (
                    runsLabel
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
