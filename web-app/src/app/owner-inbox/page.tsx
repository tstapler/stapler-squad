// analytics-exempt
import type { Metadata } from "next";
// +feature: ui:owner-inbox-page
import { OwnerInboxPage } from "./OwnerInboxPage";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Owner Inbox - Stapler Squad",
  description: "Decisions agent sessions are waiting on you to make.",
};

export default function Page() {
  return (
    <>
      <PageViewTracker />
      <OwnerInboxPage />
    </>
  );
}
