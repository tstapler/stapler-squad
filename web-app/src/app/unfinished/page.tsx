// analytics-exempt
import { Suspense } from "react";
// +feature: unfinished-work
import { UnfinishedTab } from "./UnfinishedTab";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

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
