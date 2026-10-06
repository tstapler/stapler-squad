// analytics-exempt
import type { Metadata } from "next";
// +feature: import-external-session
import { ImportSessionsContainer } from "@/components/sessions/ImportSessionsContainer";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Import External Sessions - Stapler Squad",
  description: "Discover and import terminal sessions started outside Stapler Squad.",
};

export default function ImportSessionsPage() {
  return (
    <>
      <PageViewTracker />
      <ImportSessionsContainer />
    </>
  );
}
