// +feature: insights-dashboard
import { card, title, table, th, thRight, td, tdRight } from "./ItemStageCostTable.css";
import { fmtCost } from "./insightsFormatters";
import type { ItemRoleCost } from "@/gen/session/v1/insights_pb";

/**
 * Breaks the "unattributed" role_breakdown bucket (role_breakdown[].session_role
 * === "") down by session title — the server groups it that way (see
 * groupUnattributed, server/services/insights_service.go) precisely so this
 * table isn't one shapeless "unattributed" blob. `items` is already sorted by
 * cost descending (buildRoleBreakdown's own sort). Renders nothing when empty,
 * matching ItemStageCostTable's "absent, not an empty table" convention.
 */
export function UnattributedByTitleTable({ items }: { items: ItemRoleCost[] }) {
  if (items.length === 0) return null;

  return (
    <div className={card} data-testid="unattributed-by-title-table">
      <div className={title}>Unattributed Cost by Session</div>
      <table className={table}>
        <thead>
          <tr>
            <th className={th} scope="col">Session</th>
            <th className={thRight} scope="col">Cost</th>
            <th className={thRight} scope="col">Runs</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => (
            <tr key={it.itemId}>
              <td className={td}>{it.itemTitle || "(unknown)"}</td>
              <td className={tdRight}>{fmtCost(it.estimatedCostUsd)}</td>
              <td className={tdRight}>{it.sessionCount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
