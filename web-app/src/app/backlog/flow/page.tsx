"use client";
// +feature: backlog:flow

import { Suspense } from "react";
import { BacklogViewTabs } from "@/components/backlog/BacklogViewTabs";
import { usePageView } from "@/lib/analytics/usePageView";

function BacklogFlowPageInner() {
  usePageView();
  return (
    <div>
      <BacklogViewTabs active="flow" />
      <h1>Flow</h1>
    </div>
  );
}

export default function BacklogFlowPage() {
  return (
    <Suspense>
      <BacklogFlowPageInner />
    </Suspense>
  );
}
