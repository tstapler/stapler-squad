// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.
import { JulesSettings } from "@/components/settings/JulesSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function JulesSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <main id="main-content">
        <JulesSettings />
      </main>
    </>
  );
}
