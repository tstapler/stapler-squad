// analytics-exempt
import type { Metadata } from "next";
// +feature: unfinished-work
import { UnfinishedTab } from "./UnfinishedTab";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";

export const metadata: Metadata = {
  title: "Unfinished Work - Stapler Squad",
  description: "Git worktrees with uncommitted changes or unpushed commits.",
};

export default function UnfinishedPage() {
  return (
    <>
      <PageViewTracker />
      <UnfinishedTab />
    </>
  );
}
