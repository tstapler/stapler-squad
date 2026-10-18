import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Compare Variants - Stapler Squad",
  description: "Compare parallel fan-out variants side by side and pick a winner.",
};

export default function FanoutLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
