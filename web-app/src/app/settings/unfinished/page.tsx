// analytics-exempt
import { UnfinishedSourcesSettings } from "@/components/settings/UnfinishedSourcesSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function UnfinishedSettingsPage() {
  return (
    <>
      <PageViewTracker />
      <UnfinishedSourcesSettings />
    </>
  );
}
