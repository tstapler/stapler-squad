"use client";
// +feature: backlog:workflow-suggestions

import { useState } from "react";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useAnalytics } from "@/lib/analytics";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { DIAGNOSE_SESSION_PANEL_FLAG } from "@/lib/backlog/sessionKind";
import * as styles from "./DiagnoseSessionPanel.css";

const ACTION_LABELS: Record<string, string> = {
  transition_status: "Transition",
  set_config: "Config",
  operator_note: "Note",
};

export interface WorkflowSuggestionsSectionProps {
  item: BacklogItem;
  onDismiss: (itemId: string, suggestionId: string) => Promise<boolean>;
}

/** Dismissible operator recommendations posted by diagnose sessions; renders nothing when none are active. */
export function WorkflowSuggestionsSection({ item, onDismiss }: WorkflowSuggestionsSectionProps) {
  const { track } = useAnalytics();
  const enabled = useFeatureFlag(DIAGNOSE_SESSION_PANEL_FLAG);
  const [dismissedLocally, setDismissedLocally] = useState<Set<string>>(new Set());
  const [failedId, setFailedId] = useState<string | null>(null);

  const active = (item.workflowSuggestions ?? []).filter((w) => !w.dismissed && !dismissedLocally.has(w.suggestionId));
  if (!enabled || active.length === 0) return null;

  const dismiss = async (suggestionId: string) => {
    setFailedId(null);
    try {
      const ok = await onDismiss(item.id, suggestionId);
      if (!ok) throw new Error("dismiss failed");
      setDismissedLocally((prev) => new Set(prev).add(suggestionId));
    } catch {
      setFailedId(suggestionId);
    }
  };

  return (
    <section aria-label="Workflow suggestions" data-testid="workflow-suggestions">
      {active.map((w) => (
        <div key={w.suggestionId} className={styles.suggestionCard} data-testid="workflow-suggestion-card">
          <span>{ACTION_LABELS[w.actionType] ?? w.actionType}</span>
          <span className={styles.suggestionText}>{w.recommendation}</span>
          {failedId === w.suggestionId && <span role="alert">Could not dismiss. Try again.</span>}
          <button type="button" className={styles.dismissButton} onClick={() => {
              track({ name: "backlog_dismiss_workflow_suggestion", category: "user_action", component: "WorkflowSuggestionsSection", labels: {} });
              void dismiss(w.suggestionId);
            }}>
            Dismiss
          </button>
        </div>
      ))}
    </section>
  );
}
