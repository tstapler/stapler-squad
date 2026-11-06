// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.
import { BackgroundModelsSettings } from "@/components/settings/BackgroundModelsSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function BackgroundModelsSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <main id="main-content">
        <BackgroundModelsSettings />
      </main>
    </>
  );
}
