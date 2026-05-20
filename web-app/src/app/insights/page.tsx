// analytics-exempt
// +feature: insights-dashboard
import type { Metadata } from "next";
import { InsightsDashboard } from "./InsightsDashboard";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Insights - Stapler Squad",
  description: "Token usage analytics and cost breakdown for Claude Code sessions.",
};

export default function InsightsPage() {
  return (
    <>
      <PageViewTracker />
      <InsightsDashboard />
    </>
  );
}
