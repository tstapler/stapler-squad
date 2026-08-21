// +feature: insights-dashboard
import { Skeleton } from "@/components/ui/Skeleton";
import { grid2, section } from "./InsightsDashboard.css";
import { grid as summaryGrid } from "./SummaryCards.css";

export function InsightsDashboardSkeleton() {
  return (
    <>
      <section className={section}>
        <div className={summaryGrid}>
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} data-testid="skeleton-card">
              <Skeleton variant="rectangular" width="100%" height={96} />
            </div>
          ))}
        </div>
      </section>

      <section className={section}>
        <div className={grid2}>
          <Skeleton variant="rectangular" width="100%" height={200} />
          <Skeleton variant="rectangular" width="100%" height={200} />
        </div>
      </section>

      <section className={section}>
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} variant="text" width="100%" height={32} />
        ))}
      </section>
    </>
  );
}
