"use client";

import { useAttentionDocumentTitle } from "@/lib/attention/useAttentionDocumentTitle";

/** Renders nothing; keeps the "(N)" document-title prefix in sync. Mount inside the review-queue provider. */
export function AttentionDocumentTitle() {
  useAttentionDocumentTitle();
  return null;
}
