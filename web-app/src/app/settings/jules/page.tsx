// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.
import type { Metadata } from "next";
import { JulesSettings } from "@/components/settings/JulesSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Jules - Settings - Stapler Squad",
  description: "Configure the Google Jules cloud coding agent integration.",
};

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
