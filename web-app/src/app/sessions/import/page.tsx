// analytics-exempt
// +feature: import-external-session
import { ImportSessionsContainer } from "@/components/sessions/ImportSessionsContainer";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function ImportSessionsPage() {
  return (
    <>
      <PageViewTracker />
      <ImportSessionsContainer />
    </>
  );
}
