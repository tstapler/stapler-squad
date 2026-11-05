// analytics-exempt
// +feature: settings-backlog-sources
import { BacklogSourcesSettings } from "@/components/settings/BacklogSourcesSettings";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { RequireFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";

export default function BacklogSourcesSettingsPage() {
  return (
    <RequireFeatureFlag flag="backlog">
      <PageViewTracker />
      <BacklogSourcesSettings />
    </RequireFeatureFlag>
  );
}
