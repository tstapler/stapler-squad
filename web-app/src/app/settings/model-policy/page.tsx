// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.
import type { Metadata } from "next";
import { ModelPolicySettings } from "@/components/settings/ModelPolicySettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Model Policy - Settings - Stapler Squad",
  description: "Per-feature model and effort for background LLM work.",
};

export default function ModelPolicySettingsPage() {
  return (
    <>
      <PageViewTracker />
      <main id="main-content">
        <ModelPolicySettings />
      </main>
    </>
  );
}
