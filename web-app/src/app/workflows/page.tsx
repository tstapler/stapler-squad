"use client";
// +feature: workflows-management

import { Suspense } from "react";
import { WorkflowsPanel } from "@/components/workflows/WorkflowsPanel";
import * as styles from "./page.css";

function WorkflowsPageInner() {
  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <WorkflowsPanel />
      </main>
    </div>
  );
}

export default function WorkflowsPage() {
  return (
    <Suspense>
      <WorkflowsPageInner />
    </Suspense>
  );
}
