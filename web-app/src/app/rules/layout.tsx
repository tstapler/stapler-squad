import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Approval Rules - Stapler Squad",
  description: "Manage auto-approval rules for tool-use requests in Stapler Squad.",
};

export default function RulesLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
