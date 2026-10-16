// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.
import type { Metadata } from "next";
import { BackgroundModelsSettings } from "@/components/settings/BackgroundModelsSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Background Models - Settings - Stapler Squad",
  description: "Pin the model and effort used for unattended LLM work.",
};

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
