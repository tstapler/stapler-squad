// analytics-exempt
import type { Metadata } from "next";
// +feature: ui:notifications-page
import { NotificationsPage } from "./NotificationsPage";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Notifications - Stapler Squad",
  description: "Session notifications, approvals, and alerts.",
};

export default function Page() {
  return (
    <>
      <PageViewTracker />
      <NotificationsPage />
    </>
  );
}
