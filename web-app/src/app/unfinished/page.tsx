// analytics-exempt
import type { Metadata } from "next";
import { Suspense } from "react";
// +feature: unfinished-work
import { UnfinishedTab } from "./UnfinishedTab";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Up Next - Stapler Squad",
  description: "Work in progress, queued backlog items, and importable GitHub issues.",
};

export default function UnfinishedPage() {
  return (
    <>
      <PageViewTracker />
      <Suspense>
        <UnfinishedTab />
      </Suspense>
    </>
  );
}
