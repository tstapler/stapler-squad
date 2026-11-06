"use client";
// +feature: backlog:flow

import { Suspense } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { ErrorState } from "@/components/ui/ErrorState";
import { usePageView } from "@/lib/analytics/usePageView";
import { FlowPage } from "./FlowPage";

const renderFailure = (
  <>
    <ErrorState title="Flow failed to render" message="Something went wrong while drawing this page." />
    <AppLink href="/backlog">Back to Backlog</AppLink>
  </>
);

export default function BacklogFlowPage() {
  usePageView();
  return (
    <ErrorBoundary fallback={renderFailure}>
      <Suspense>
        <FlowPage />
      </Suspense>
    </ErrorBoundary>
  );
}
