"use client";
// +feature: local-file-browser

import { Suspense } from "react";
import { LocalFileBrowser } from "@/components/files/LocalFileBrowser";

export default function FilesPage() {
  return (
    <Suspense fallback={<div>Loading file browser...</div>}>
      <LocalFileBrowser />
    </Suspense>
  );
}

