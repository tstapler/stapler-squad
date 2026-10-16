"use client";
// +feature: changelog

import { useEffect, useState } from "react";
import { fetchUpdateCheck, type UpdateCheckResponse } from "@/lib/api/changelogApi";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import * as styles from "./changelog.css";

/** Small link chip shown only when the opt-in update check found a newer release. */
export function UpdateBadge() {
  const { flags } = useFeatureFlags();
  const enabled = !!flags["changelog"];
  const [result, setResult] = useState<UpdateCheckResponse | null>(null);

  useEffect(() => {
    if (!enabled || typeof fetch !== "function") return;
    const ctrl = new AbortController();
    fetchUpdateCheck(ctrl.signal)
      .then(setResult)
      .catch(() => {
        // Best-effort: a missing badge is the correct failure mode.
      });
    return () => ctrl.abort();
  }, [enabled]);

  if (!enabled || !result?.enabled || result.is_source_build || !result.is_outdated) return null;

  return (
    <a
      className={styles.updateBadge}
      href={result.latest_url || "/changelog"}
      target="_blank"
      rel="noopener noreferrer"
      title={`Version ${result.latest_version} is available (you have ${result.current_version})`}
      data-testid="update-badge"
    >
      Update available: v{result.latest_version}
    </a>
  );
}
