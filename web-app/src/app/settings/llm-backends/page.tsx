// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally -- same
// pattern as tagging-classifier/page.tsx.
import { LLMBackendSettings } from "@/components/settings/LLMBackendSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function LLMBackendsSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <main id="main-content">
        <LLMBackendSettings />
      </main>
    </>
  );
}
