import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Account - Stapler Squad",
  description: "Manage your registered passkeys and add new devices.",
};

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
