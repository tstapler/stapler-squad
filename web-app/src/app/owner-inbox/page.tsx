// analytics-exempt
// +feature: ui:owner-inbox-page
import { OwnerInboxPage } from "./OwnerInboxPage";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function Page() {
  return (
    <>
      <PageViewTracker />
      <OwnerInboxPage />
    </>
  );
}
