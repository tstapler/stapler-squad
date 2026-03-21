import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Configuration - Stapler Squad",
  description: "Configure Stapler Squad settings including agent programs, tmux prefix, and log levels.",
};

export default function ConfigLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
