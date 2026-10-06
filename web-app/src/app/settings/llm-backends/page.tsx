// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally -- same
// pattern as tagging-classifier/page.tsx.
import type { Metadata } from "next";
import { LLMBackendSettings } from "@/components/settings/LLMBackendSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "LLM Backends - Settings - Stapler Squad",
  description: "Choose which backend serves headless LLM features.",
};

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
