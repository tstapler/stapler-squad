"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";

export default function BacklogLayout({ children }: { children: React.ReactNode }) {
  const { flags, isLoading } = useFeatureFlags();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && !flags["backlog"]) {
      router.replace("/");
    }
  }, [isLoading, flags, router]);

  if (isLoading) return null;
  if (!flags["backlog"]) return null;
  return <>{children}</>;
}
