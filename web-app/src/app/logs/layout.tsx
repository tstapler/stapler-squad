import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Logs - Stapler Squad",
  description: "View and search application logs from your Stapler Squad sessions.",
};

export default function LogsLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
