"use client";

import { ErrorDashboard } from "@/components/errors/ErrorDashboard";
import { usePageView } from "@/lib/analytics/usePageView";

export default function ErrorsPage() {
  usePageView();
  return <ErrorDashboard />;
}
