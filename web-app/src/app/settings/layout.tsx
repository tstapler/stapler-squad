import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Settings - Stapler Squad",
  description: "Configure session defaults, profiles, and directory rules.",
};

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
