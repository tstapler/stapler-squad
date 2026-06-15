// +feature: local-file-browser
"use client";

import { Suspense } from "react";
import { LocalFileBrowser } from "@/components/files/LocalFileBrowser";

export default function FilesPage() {
  return (
    <Suspense>
      <LocalFileBrowser />
    </Suspense>
  );
}
