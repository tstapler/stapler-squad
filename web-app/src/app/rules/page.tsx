"use client";

import { ApprovalRulesPanel } from "@/components/sessions/ApprovalRulesPanel";
import { ApprovalAnalyticsPanel } from "@/components/sessions/ApprovalAnalyticsPanel";
import styles from "./page.module.css";

export default function RulesPage() {
  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <ApprovalRulesPanel />
        <ApprovalAnalyticsPanel />
      </main>
    </div>
  );
}
