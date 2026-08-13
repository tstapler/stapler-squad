import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Review Queue - Stapler Squad",
  description: "Review and manage sessions awaiting your attention in Stapler Squad.",
};

export default function ReviewQueueLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
