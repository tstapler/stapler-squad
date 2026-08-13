"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { useApprovalRules } from "@/lib/hooks/useApprovalRules";
import { ApprovalRuleProto, AutoDecision, SuggestedRuleProto } from "@/gen/session/v1/types_pb";
import {
  card,
  cardHeader,
  cardTitle,
  confidenceBadge,
  explanationBlock,
  sourceCommandsDetails,
  sourceCommandsSummary,
  sourceCommandsPre,
  conflictBanner,
  shadowBanner,
  fieldGrid,
  fieldRow,
  fieldLabel,
  fieldInput,
  fieldSelect,
  fieldTextarea,
  actions,
  acceptButton,
  discardButton,
} from "./SuggestedRuleCard.css";

// ── Local form state ──────────────────────────────────────────────────────────

interface RuleFormState {
  name: string;
  toolName: string;
  toolPattern: string;
  commandPattern: string;
  filePattern: string;
  decision: AutoDecision;
  reason: string;
  alternative: string;
  priority: number;
}

function stateFromSuggestion(s: SuggestedRuleProto): RuleFormState {
  return {
    name: s.name,
    toolName: s.toolName,
    toolPattern: s.toolPattern,
    commandPattern: s.commandPattern,
    filePattern: s.filePattern,
    decision: s.decision !== AutoDecision.UNSPECIFIED ? s.decision : AutoDecision.ALLOW,
    reason: s.reason,
    alternative: s.alternative,
    priority: s.priority > 0 ? s.priority : 100,
  };
}

// ── Confidence helpers ────────────────────────────────────────────────────────

type ConfidenceLevel = "high" | "medium" | "low";

function confidenceLevel(c: number): ConfidenceLevel {
  if (c >= 0.8) return "high";
  if (c >= 0.5) return "medium";
  return "low";
}

const CONFIDENCE_LABELS: Record<ConfidenceLevel, string> = {
  high: "High",
  medium: "Medium",
  low: "Low",
};

// ── Props ─────────────────────────────────────────────────────────────────────

export interface SuggestedRuleCardProps {
  suggestion: SuggestedRuleProto;
  /** Called with the saved rule after successful upsert. */
  onAccept: (savedRule: ApprovalRuleProto) => void;
  onDiscard: () => void;
  /** If true, render the card in a skeleton/disabled state while a generation is in flight. */
  loading?: boolean;
}

// ── Component ─────────────────────────────────────────────────────────────────

/**
 * SuggestedRuleCard renders a single AI-proposed rule with all fields editable.
 *
 * The user can review AI metadata (confidence, explanation, source commands,
 * conflict warnings), edit any field, then "Accept & Save" (calls upsertRule)
 * or "Discard" (calls onDiscard).
 */
export function SuggestedRuleCard({
  suggestion,
  onAccept,
  onDiscard,
  loading = false,
}: SuggestedRuleCardProps) {
  const { upsertRule, rules } = useApprovalRules();

  // Keep a ref to the latest rules so handleAccept never uses a stale closure snapshot.
  const rulesRef = useRef(rules);
  useEffect(() => {
    rulesRef.current = rules;
  }, [rules]);

  const [form, setForm] = useState<RuleFormState>(() => stateFromSuggestion(suggestion));
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const setField = useCallback(
    <K extends keyof RuleFormState>(key: K, value: RuleFormState[K]) => {
      setForm((prev) => ({ ...prev, [key]: value }));
    },
    []
  );

  const handleAccept = useCallback(async () => {
    if (!form.name.trim()) return;
    setSaving(true);
    setSaveError(null);
    try {
      const id = `user-${Date.now()}`;
      await upsertRule({
        id,
        name: form.name,
        toolName: form.toolName,
        toolPattern: form.toolPattern,
        commandPattern: form.commandPattern,
        filePattern: form.filePattern,
        decision: form.decision,
        riskLevel: suggestion.riskLevel,
        reason: form.reason,
        alternative: form.alternative,
        priority: form.priority,
        enabled: true,
        source: "user",
      });
      // Find the just-upserted rule in the latest rules list (via ref to avoid stale closure).
      const saved = rulesRef.current.find((r) => r.name === form.name && r.source === "user");
      if (saved) {
        onAccept(saved);
      } else {
        // Fallback: construct a minimal ApprovalRuleProto-shaped object.
        onAccept({
          id,
          name: form.name,
          toolName: form.toolName,
          toolPattern: form.toolPattern,
          commandPattern: form.commandPattern,
          filePattern: form.filePattern,
          decision: form.decision,
          riskLevel: suggestion.riskLevel,
          reason: form.reason,
          alternative: form.alternative,
          priority: form.priority,
          enabled: true,
          source: "user",
        } as unknown as ApprovalRuleProto);
      }
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Failed to save rule.");
    } finally {
      setSaving(false);
    }
  }, [form, suggestion.riskLevel, upsertRule, onAccept]);

  const level = confidenceLevel(suggestion.confidence);
  const confidencePct = Math.round(suggestion.confidence * 100);
  const sourceCommandsToShow = suggestion.sourceCommands.slice(0, 5);

  if (loading) {
    return (
      <div className={card} aria-busy="true">
        <p style={{ color: "inherit", opacity: 0.5 }}>Generating suggestion…</p>
      </div>
    );
  }

  return (
    <div className={card} data-testid="suggested-rule-card">
      {/* ── Header: title + confidence badge ── */}
      <div className={cardHeader}>
        <h3 className={cardTitle}>AI Rule Suggestion</h3>
        <span
          className={confidenceBadge({ level })}
          data-testid="confidence-badge"
          data-level={level}
          aria-label={`Confidence: ${CONFIDENCE_LABELS[level]} (${confidencePct}%)`}
        >
          {CONFIDENCE_LABELS[level]} — {confidencePct}% confidence
        </span>
      </div>

      {/* ── Explanation ── */}
      {suggestion.explanation && (
        <p className={explanationBlock}>{suggestion.explanation}</p>
      )}

      {/* ── Source commands (collapsible) ── */}
      {sourceCommandsToShow.length > 0 && (
        <details className={sourceCommandsDetails}>
          <summary className={sourceCommandsSummary}>
            Source commands ({sourceCommandsToShow.length})
          </summary>
          <pre className={sourceCommandsPre}>
            {sourceCommandsToShow.join("\n")}
          </pre>
        </details>
      )}

      {/* ── Conflict warnings ── */}
      {suggestion.shadowedByRuleIds.length > 0 && (
        <div
          className={conflictBanner}
          role="alert"
          data-testid="conflict-banner"
        >
          May be shadowed by higher-priority rule(s):{" "}
          {suggestion.shadowedByRuleIds.join(", ")}
        </div>
      )}
      {suggestion.shadowsRuleIds.length > 0 && (
        <div className={shadowBanner} role="status" data-testid="shadow-banner">
          May suppress lower-priority rule(s): {suggestion.shadowsRuleIds.join(", ")}
        </div>
      )}

      {/* ── Editable form fields ── */}
      <div className={fieldGrid}>
        <label className={fieldRow}>
          <span className={fieldLabel}>Name *</span>
          <input
            className={fieldInput}
            type="text"
            value={form.name}
            onChange={(e) => setField("name", e.target.value)}
            required
            aria-label="Name"
            data-testid="field-name"
          />
        </label>

        <label className={fieldRow}>
          <span className={fieldLabel}>Tool Name</span>
          <input
            className={fieldInput}
            type="text"
            value={form.toolName}
            onChange={(e) => setField("toolName", e.target.value)}
            aria-label="Tool Name"
            data-testid="field-tool-name"
          />
        </label>

        <label className={fieldRow}>
          <span className={fieldLabel}>Tool Pattern</span>
          <input
            className={fieldInput}
            type="text"
            value={form.toolPattern}
            onChange={(e) => setField("toolPattern", e.target.value)}
            aria-label="Tool Pattern"
            data-testid="field-tool-pattern"
          />
        </label>

        <label className={fieldRow}>
          <span className={fieldLabel}>Command Pattern</span>
          <input
            className={fieldInput}
            type="text"
            value={form.commandPattern}
            onChange={(e) => setField("commandPattern", e.target.value)}
            aria-label="Command Pattern"
            data-testid="field-command-pattern"
          />
        </label>

        <label className={fieldRow}>
          <span className={fieldLabel}>File Pattern</span>
          <input
            className={fieldInput}
            type="text"
            value={form.filePattern}
            onChange={(e) => setField("filePattern", e.target.value)}
            aria-label="File Pattern"
            data-testid="field-file-pattern"
          />
        </label>

        <label className={fieldRow}>
          <span className={fieldLabel}>Decision</span>
          <select
            className={fieldSelect}
            value={form.decision}
            onChange={(e) => setField("decision", Number(e.target.value) as AutoDecision)}
            aria-label="Decision"
            data-testid="field-decision"
          >
            <option value={AutoDecision.ALLOW}>Auto-Allow</option>
            <option value={AutoDecision.DENY}>Auto-Deny</option>
            <option value={AutoDecision.ESCALATE}>Escalate</option>
          </select>
        </label>

        <label className={fieldRow}>
          <span className={fieldLabel}>Priority</span>
          <input
            className={fieldInput}
            type="number"
            min={1}
            max={999}
            value={form.priority}
            onChange={(e) => setField("priority", Math.max(1, Math.min(999, Number(e.target.value))))}
            aria-label="Priority"
            data-testid="field-priority"
          />
        </label>
      </div>

      {/* Reason and alternative span full width */}
      <label className={fieldRow}>
        <span className={fieldLabel}>Reason</span>
        <textarea
          className={fieldTextarea}
          value={form.reason}
          onChange={(e) => setField("reason", e.target.value)}
          aria-label="Reason"
          data-testid="field-reason"
        />
      </label>

      <label className={fieldRow}>
        <span className={fieldLabel}>Alternative</span>
        <textarea
          className={fieldTextarea}
          value={form.alternative}
          onChange={(e) => setField("alternative", e.target.value)}
          aria-label="Alternative"
          data-testid="field-alternative"
        />
      </label>

      {/* ── Save error ── */}
      {saveError && (
        <p role="alert" style={{ color: "var(--error-text, red)", fontSize: 13, margin: 0 }}>
          {saveError}
        </p>
      )}

      {/* ── Action buttons ── */}
      <div className={actions}>
        <button
          className={discardButton}
          type="button"
          onClick={onDiscard}
          disabled={saving}
          data-testid="discard-rule"
        >
          Discard
        </button>
        <button
          className={acceptButton}
          type="button"
          onClick={handleAccept}
          disabled={saving || !form.name.trim()}
          data-testid="accept-rule"
        >
          {saving ? "Saving…" : "Accept & Save"}
        </button>
      </div>
    </div>
  );
}
