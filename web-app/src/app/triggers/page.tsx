// analytics-exempt
"use client";
// Note: no +feature marker here — TriggersPanel.tsx and CallbackSettings.tsx each
// carry their own canonical marker (unlike RulesPage's ApprovalRulesPanel, which has
// none of its own).

import { TriggersPanel } from "@/components/sessions/TriggersPanel";
import { CallbackSettings } from "@/components/sessions/CallbackSettings";
import * as styles from "./page.css";

export default function TriggersPage() {
  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <TriggersPanel />
        <CallbackSettings />
      </main>
    </div>
  );
}
