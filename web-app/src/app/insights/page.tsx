// analytics-exempt
// +feature: insights-dashboard
import { InsightsDashboard } from "./InsightsDashboard";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function InsightsPage() {
  return (
    <>
      <PageViewTracker />
      <InsightsDashboard />
    </>
  );
}
