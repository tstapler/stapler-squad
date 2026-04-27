import type { Metadata } from "next";
import { UnfinishedTab } from "./UnfinishedTab";

export const metadata: Metadata = {
  title: "Unfinished Work - Stapler Squad",
  description: "Git worktrees with uncommitted changes or unpushed commits.",
};

export default function UnfinishedPage() {
  return <UnfinishedTab />;
}
