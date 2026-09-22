// +feature: insights-dashboard
import { card, title, table, th, thRight, td, tdRight } from "./ItemStageCostTable.css";
import { fmtCost } from "./insightsFormatters";
import type { ItemRoleCost } from "@/gen/session/v1/insights_pb";

/**
 * Breaks a no-backlog-attribution role_breakdown bucket (session_role === ""
 * or "external") down by session title — the server groups both that way
 * (see groupUnattributed, server/services/insights_service.go) precisely so
 * this table isn't one shapeless blob. `items` is already sorted by cost
 * descending (buildRoleBreakdown's own sort). Renders nothing when empty,
 * matching ItemStageCostTable's "absent, not an empty table" convention.
 */
export function UnattributedByTitleTable({
  title: heading,
  items,
  testId = "unattributed-by-title-table",
}: {
  title: string;
  items: ItemRoleCost[];
  testId?: string;
}) {
  if (items.length === 0) return null;

  return (
    <div className={card} data-testid={testId}>
      <div className={title}>{heading}</div>
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
