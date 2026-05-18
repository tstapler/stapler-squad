import type { Metadata } from "next";
import { EscapeAnalyticsPage } from "@/components/analytics/EscapeAnalyticsPage";

export const metadata: Metadata = {
  title: "Escape Analytics - Stapler Squad",
  description: "Inspect terminal escape sequence statistics and mangle events per session.",
};

export default function EscapeAnalyticsRoute() {
  return <EscapeAnalyticsPage />;
}
