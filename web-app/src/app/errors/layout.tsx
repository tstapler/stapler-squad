import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Errors - Stapler Squad",
  description: "View and acknowledge RPC error events from Stapler Squad.",
};

export default function ErrorsLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
