"use client";

import { RequireFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";

export default function ChangelogLayout({ children }: { children: React.ReactNode }) {
  return <RequireFeatureFlag flag="changelog">{children}</RequireFeatureFlag>;
}
