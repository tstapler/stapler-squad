// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.
import type { Metadata } from "next";
import { TaggingClassifierSettings } from "@/components/settings/TaggingClassifierSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Tag Classification - Settings - Stapler Squad",
  description: "Configure the LLM model hierarchy for session-tag classification.",
};

export default function TaggingClassifierSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <main id="main-content">
        <TaggingClassifierSettings />
      </main>
    </>
  );
}
