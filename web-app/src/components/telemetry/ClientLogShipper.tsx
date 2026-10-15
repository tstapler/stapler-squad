"use client";

import { useEffect } from "react";
import { defaultSend, installClientLogShipper, windowIdFromUrl } from "@/lib/telemetry/clientLogShipper";

/** Mount once near the app root; ships browser warn/error to the server log. Renders nothing. */
export function ClientLogShipper(): null {
  useEffect(() => installClientLogShipper({ windowId: windowIdFromUrl, send: defaultSend }), []);
  return null;
}
