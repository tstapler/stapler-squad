"use client";

import { useEffect } from "react";
import { initBrowserOtel } from "@/lib/telemetry/otel";

// +feature: ui:otel-tracing
export function OtelInit() {
  useEffect(() => {
    initBrowserOtel();
  }, []);

  return null;
}
