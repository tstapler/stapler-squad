import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Sign In - Stapler Squad",
  description: "Sign in to Stapler Squad to manage your AI agent sessions.",
};

export default function LoginLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
