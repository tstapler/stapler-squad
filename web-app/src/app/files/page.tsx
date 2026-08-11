"use client";
// +feature: local-file-browser

import { Suspense } from "react";
import { LocalFileBrowser } from "@/components/files/LocalFileBrowser";
import { usePageView } from "@/lib/analytics/usePageView";

export default function FilesPage() {
  usePageView();
  return (
    <Suspense fallback={<div>Loading file browser...</div>}>
      <LocalFileBrowser />
    </Suspense>
  );
}

