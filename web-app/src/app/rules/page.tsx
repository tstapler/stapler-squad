"use client";
// +feature: approval-rules rules-management

import { ApprovalRulesPanel } from "@/components/sessions/ApprovalRulesPanel";
import { ApprovalAnalyticsPanel } from "@/components/sessions/ApprovalAnalyticsPanel";
import { usePageView } from "@/lib/analytics/usePageView";
import * as styles from "./page.css";

export default function RulesPage() {
  usePageView();
  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <ApprovalRulesPanel />
        <ApprovalAnalyticsPanel />
      </main>
    </div>
  );
}
