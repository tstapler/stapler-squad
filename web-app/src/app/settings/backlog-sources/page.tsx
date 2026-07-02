// +feature: settings-backlog-sources
// analytics-exempt
import type { Metadata } from "next";
import { BacklogSourcesSettings } from "@/components/settings/BacklogSourcesSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Backlog Sources - Settings - Stapler Squad",
  description: "Configure external sources (GitHub issues, pull requests) to sync into the backlog.",
};

export default function BacklogSourcesSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <BacklogSourcesSettings />
    </>
  );
}
