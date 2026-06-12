"use client";
// +feature: approval-rules rules-management

import { Suspense, useEffect } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { ApprovalRulesPanel } from "@/components/sessions/ApprovalRulesPanel";
import { ApprovalAnalyticsPanel } from "@/components/sessions/ApprovalAnalyticsPanel";
import { decodePrefill, RuleBuilderPrefill } from "@/lib/ruleBuilderPrefill";
import * as styles from "./page.css";

function RulesPageInner() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const rawPrefill = searchParams.get("prefill");
  const prefill: RuleBuilderPrefill | null = rawPrefill ? decodePrefill(rawPrefill) : null;

  useEffect(() => {
    if (prefill) {
      setTimeout(() => {
        document.getElementById("rule-builder")?.scrollIntoView({ behavior: "smooth" });
      }, 100);
    }
  }, [prefill]);

  function clearPrefill() {
    router.replace("/rules");
  }


  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <ApprovalRulesPanel prefill={prefill} />
        <ApprovalAnalyticsPanel />
      </main>
    </div>
  );
}

export default function RulesPage() {
  return (
    <Suspense fallback={<div style={{ padding: "2rem" }}>Loading…</div>}>
      <RulesPageInner />
    </Suspense>
  );
}
