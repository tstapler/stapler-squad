// analytics-exempt
// +feature: ui:notifications-page
import { NotificationsPage } from "./NotificationsPage";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export default function Page() {
  return (
    <>
      <PageViewTracker />
      <NotificationsPage />
    </>
  );
}
