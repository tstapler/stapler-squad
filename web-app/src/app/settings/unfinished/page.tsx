// analytics-exempt
import type { Metadata } from "next";
import { UnfinishedSourcesSettings } from "@/components/settings/UnfinishedSourcesSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Unfinished Sources - Settings - Stapler Squad",
  description: "Configure watch directories and pinned repositories for unfinished work scanning.",
};

export default function UnfinishedSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <UnfinishedSourcesSettings />
    </>
  );
}
