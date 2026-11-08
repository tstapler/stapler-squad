"use client";
// +feature: session-artifacts-browse

import { Suspense } from "react";
import { usePageView } from "@/lib/analytics/usePageView";
import { ArtifactsBrowser } from "@/components/artifacts/ArtifactsBrowser";

export default function ArtifactsPage() {
  usePageView();
  return (
    <Suspense fallback={null}>
      <ArtifactsBrowser />
    </Suspense>
  );
}
