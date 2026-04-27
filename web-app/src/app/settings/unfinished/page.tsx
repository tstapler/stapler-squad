import type { Metadata } from "next";
import { UnfinishedSourcesSettings } from "@/components/settings/UnfinishedSourcesSettings";

export const metadata: Metadata = {
  title: "Unfinished Sources - Settings - Stapler Squad",
  description: "Configure watch directories and pinned repositories for unfinished work scanning.",
};

export default function UnfinishedSettingsPage() {
  return <UnfinishedSourcesSettings />;
}
