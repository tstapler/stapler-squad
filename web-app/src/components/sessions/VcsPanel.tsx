"use client";

import type { Session } from "@/gen/session/v1/types_pb";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { fromSessionVcs } from "@/lib/vcs/adapters";
import { VcsWidget } from "@/components/shared/VcsWidget";
import * as styles from "./VcsPanel.css";

interface VcsPanelProps {
  /** Optional callback to navigate to a file in the Files tab. */
  onNavigateToFile?: (path: string) => void;
  /** Session object for displaying GitHub PR/repo info. */
  session?: Session;
}

export function VcsPanel({ onNavigateToFile, session }: VcsPanelProps) {
  const { status, statusLoading, error, refresh } = useSessionVcsContext();
  const { track } = useAnalytics();

  const handleRetry = () => {
    track({ name: "toolbar_button_click", category: "user_action", component: "VcsPanel", labels: { button: "retry" } });
    refresh();
  };

  if (statusLoading && !status) {
    return (
      <div className={styles.container}>
        <div className={styles.skeleton} role="status" aria-label="Loading VCS status">
          <div className={styles.skeletonBar} style={{ width: "40%" }} />
          <div className={styles.skeletonBar} style={{ width: "70%" }} />
          <div className={styles.skeletonBar} style={{ width: "90%" }} />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          <span className={styles.errorIcon}>⚠️</span>
          <span>{error.message}</span>
          <button className={styles.retryButton} onClick={handleRetry}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!status) {
    return (
      <div className={styles.container}>
        <div className={styles.empty}>
          <p>No VCS information available</p>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <VcsWidget
        data={fromSessionVcs(status, session)}
        mode="full"
        onNavigateToFile={onNavigateToFile}
        onRefresh={refresh}
      />
    </div>
  );
}
