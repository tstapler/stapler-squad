"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";

export default function ChangelogLayout({ children }: { children: React.ReactNode }) {
  const { flags, isLoading } = useFeatureFlags();
  const router = useRouter();
  const enabled = !!flags["changelog"];

  useEffect(() => {
    if (!isLoading && !enabled) router.replace("/");
  }, [isLoading, enabled, router]);

  if (isLoading || !enabled) return null;
  return <>{children}</>;
}
