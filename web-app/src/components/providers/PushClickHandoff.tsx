"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { usePushClickHandoff } from "@/lib/hooks/usePushNotifications";

/**
 * Mounted at the app root so a push click is routed from any page; the service
 * worker hands the click to whichever same-origin window it focuses.
 */
export function PushClickHandoff() {
  const router = useRouter();
  usePushClickHandoff(useCallback((url: string) => router.push(url), [router]));
  return null;
}
