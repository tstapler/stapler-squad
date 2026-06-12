import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Workflows - Stapler Squad",
  description: "Manage quick-launch agent workflows in Stapler Squad.",
};

export default function WorkflowsLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
