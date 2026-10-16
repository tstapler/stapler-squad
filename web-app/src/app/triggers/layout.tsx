import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Triggers - Stapler Squad",
  description: "Manage inbound triggers (GitHub push, cron, webhook) and outbound callback settings.",
};

export default function TriggersLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
