"use client";

import { LogViewer } from "@/components/logs/LogViewer";
import * as styles from "./SessionLogsTab.css";

interface SessionLogsTabProps {
  sessionId: string;
}

export function SessionLogsTab({ sessionId }: SessionLogsTabProps) {
  return (
    <div className={styles.container}>
      {/*
        Explicit flex column wrapper ensures react-virtuoso can measure its
        scroll container height. The inner div fills the remaining space via
        flex: 1; minHeight: 0 prevents overflow in a flex parent (P-10).
      */}
      <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", height: "100%" }}>
        <LogViewer source="session" sessionId={sessionId} />
      </div>
    </div>
  );
}
