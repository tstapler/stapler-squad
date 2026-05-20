"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useApprovalRules } from "@/lib/hooks/useApprovalRules";
import { useApprovalAnalytics } from "@/lib/hooks/useApprovalAnalytics";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { ApprovalRuleProto, AutoDecision, SuggestionSource } from "@/gen/session/v1/types_pb";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import {
  panel, header, titleRow, title, subtitle, refreshButton,
  analyticsBar, analyticsTotal, analyticsRate, rateAllow, rateManual, analyticsTopTool,
  tabs, tab, tabActive,
  error as errorClass, retryButton,
  loading as loadingClass, empty,
  tableWrapper, table, th, td, tdCenter, row, rowDisabled,
  ruleName, ruleReason, ruleAlt, matchInfo, matchChip,
  decisionBadge, decisionAllow, decisionDeny, decisionEscalate,
  sourceBadge, toggle, toggleOn, toggleOff, deleteButton,
  formSection, addButton, form as formClass, formTitle, formError as formErrorClass, formGrid, label, input, select,
  formActions, saveButton, cancelButton,
  generateButtonRow, generateButton, cancelGenerateButton,
  generateErrorBanner, dismissErrorButton, suggestionsContainer,
  commandSampleDetails, commandSampleSummary, commandSampleBody,
  commandSampleTextarea, commandSampleActions, aiGeneratedBadge,
} from "./ApprovalRulesPanel.css";

// ── helpers ──────────────────────────────────────────────────────────────────

/** Escape all regex metacharacters in a literal string for safe interpolation into patterns. */
function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function decisionLabel(d: AutoDecision): string {
  switch (d) {
    case AutoDecision.ALLOW: return "Auto-Allow";
    case AutoDecision.DENY:  return "Auto-Deny";
    default:                 return "Escalate";
  }
}

function decisionClass(d: AutoDecision): string {
  switch (d) {
    case AutoDecision.ALLOW: return decisionAllow;
    case AutoDecision.DENY:  return decisionDeny;
    default:                 return decisionEscalate;
  }
}

function sourceLabel(s: string): string {
  switch (s) {
    case "user":            return "Custom";
    case "seed":            return "Built-in";
    case "claude-settings": return "Claude Settings";
    default:                return s;
  }
}

// ── empty form state ──────────────────────────────────────────────────────────

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
  enabled: boolean;
}

const emptyForm: RuleFormState = {
  name: "",
  toolName: "",
  toolPattern: "",
  commandPattern: "",
  filePattern: "",
  decision: AutoDecision.ALLOW,
  reason: "",
  alternative: "",
  priority: 10,
  enabled: true,
};

// ── component ─────────────────────────────────────────────────────────────────

/**
 * ApprovalRulesPanel shows the list of auto-approval rules and lets users
 * create, toggle, and delete custom rules.
 *
 * Built-in (seed) and claude-settings rules are shown read-only.
 */
export function ApprovalRulesPanel() {
  const { rules, loading, error, upsertRule, deleteRule, refresh } = useApprovalRules();
  const { summary, loading: analyticsLoading } = useApprovalAnalytics({ windowDays: 7 });

  // ── Epic 3: panel-level "Generate Suggestions" hook ─────────────────────
  const {
    suggestions,
    loading: genLoading,
    error: genError,
    generate,
    cancel,
    clear,
  } = useGenerateRule();

  // ── Epic 6: command-sample generate hook (separate instance) ─────────────
  const {
    suggestions: cmdSuggestions,
    loading: cmdGenLoading,
    generate: cmdGenerate,
    clear: cmdGenClear,
  } = useGenerateRule();

  const formSectionRef = useRef<HTMLDivElement>(null);

  const [sourceFilter, setSourceFilter] = useState<string>("all");
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<RuleFormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [aiPrefilled, setAiPrefilled] = useState(false);
  const [cmdSampleValue, setCmdSampleValue] = useState("");

  // Track which form fields the user has manually edited (not overwritten by AI pre-fill).
  const touchedFieldsRef = useRef<Set<keyof RuleFormState>>(new Set());

  // ── URL param pre-fill (from analytics "Add rule →" links) ───────────────
  // Runs once on mount (client only). Reads window.location.search directly to
  // avoid useSearchParams + Suspense complications in the static export.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const tool = params.get("tool");
    const program = params.get("program");
    const subcommand = params.get("subcommand");
    if (!tool && !program) return;

    const prefill: Partial<RuleFormState> = {};
    if (tool) {
      prefill.toolName = tool;
      prefill.name = `Allow ${tool}`;
    } else if (program) {
      // Escape regex metacharacters so values like "docker-compose" or "node.js"
      // produce valid patterns. Use (?:^|\s) / (?:\s|$) word boundaries because
      // \b does not match at hyphen boundaries.
      const esc = escapeRegex(program);
      prefill.toolName = "Bash";
      if (subcommand) {
        const escSub = escapeRegex(subcommand);
        prefill.commandPattern = `(?:^|\\s)${esc}(?:\\s|$).*(?:^|\\s)${escSub}(?:\\s|$)`;
        prefill.name = `Allow ${program} ${subcommand}`;
      } else {
        prefill.commandPattern = `(?:^|\\s)${esc}(?:\\s|$)`;
        prefill.name = `Allow ${program}`;
      }
    }

    setShowForm(true);
    setForm({ ...emptyForm, ...prefill });
    setFormError(null);
    setAiPrefilled(false);
    setCmdSampleValue("");
    touchedFieldsRef.current = new Set();
    cmdGenClear();

    setTimeout(() => {
      formSectionRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 100);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── filter ────────────────────────────────────────────────────────────────

  const visibleRules = sourceFilter === "all"
    ? rules
    : rules.filter((r) => r.source === sourceFilter);

  // ── save handler ──────────────────────────────────────────────────────────

  const handleSave = async () => {
    if (!form.name.trim()) {
      setFormError("Name is required.");
      return;
    }
    if (!form.toolName && !form.toolPattern && !form.commandPattern && !form.filePattern) {
      setFormError("At least one of Tool Name, Tool Pattern, Command Pattern, or File Pattern is required.");
      return;
    }
    setFormError(null);
    setSaving(true);
    try {
      const id = `user-${Date.now()}`;
      await upsertRule({ id, ...form, riskLevel: "" });
      setForm(emptyForm);
      setShowForm(false);
    } catch (e) {
      setFormError(e instanceof Error ? e.message : "Failed to save rule.");
    } finally {
      setSaving(false);
    }
  };

  // ── toggle enabled ────────────────────────────────────────────────────────

  const handleToggle = async (rule: ApprovalRuleProto) => {
    if (rule.source !== "user") return;
    try {
      await upsertRule({
        id: rule.id,
        name: rule.name,
        toolName: rule.toolName,
        toolPattern: rule.toolPattern,
        commandPattern: rule.commandPattern,
        filePattern: rule.filePattern,
        decision: rule.decision,
        riskLevel: rule.riskLevel,
        reason: rule.reason,
        alternative: rule.alternative,
        priority: rule.priority,
        enabled: !rule.enabled,
      });
    } catch (e) {
      console.error("Failed to toggle rule:", e);
    }
  };

  // ── open/close form ───────────────────────────────────────────────────────

  const openForm = () => {
    setShowForm(true);
    setForm(emptyForm);
    setFormError(null);
    setAiPrefilled(false);
    setCmdSampleValue("");
    touchedFieldsRef.current = new Set();
    cmdGenClear();
  };

  const closeForm = () => {
    setShowForm(false);
    setForm(emptyForm);
    setFormError(null);
    setAiPrefilled(false);
    setCmdSampleValue("");
    touchedFieldsRef.current = new Set();
    cmdGenClear();
  };

  // ── Epic 6: pre-fill form from command-sample suggestion ──────────────────
  // useEffect ensures prefill runs after the form is open (showForm=true) and
  // only when cmdSuggestions actually changes.
  useEffect(() => {
    if (!showForm) return;
    if (cmdSuggestions.length === 0) return;
    const suggestion = cmdSuggestions[0];
    const touched = touchedFieldsRef.current;
    setForm((prev) => ({
      name:           touched.has("name")           ? prev.name           : suggestion.name || prev.name,
      toolName:       touched.has("toolName")       ? prev.toolName       : suggestion.toolName || prev.toolName,
      toolPattern:    touched.has("toolPattern")    ? prev.toolPattern    : suggestion.toolPattern || prev.toolPattern,
      commandPattern: touched.has("commandPattern") ? prev.commandPattern : suggestion.commandPattern || prev.commandPattern,
      filePattern:    touched.has("filePattern")    ? prev.filePattern    : suggestion.filePattern || prev.filePattern,
      decision:       touched.has("decision")       ? prev.decision       : (suggestion.decision !== AutoDecision.UNSPECIFIED ? suggestion.decision : prev.decision),
      reason:         touched.has("reason")         ? prev.reason         : suggestion.reason || prev.reason,
      alternative:    touched.has("alternative")    ? prev.alternative    : suggestion.alternative || prev.alternative,
      priority:       touched.has("priority")       ? prev.priority       : (suggestion.priority > 0 ? suggestion.priority : prev.priority),
      enabled:        prev.enabled,
    }));
    setAiPrefilled(true);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cmdSuggestions, showForm]);

  // ── Epic 6: handle manual field changes (mark touched) ───────────────────

  const setFormField = <K extends keyof RuleFormState>(key: K, value: RuleFormState[K]) => {
    touchedFieldsRef.current.add(key);
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  // ── Epic 3: handle suggestion cards ──────────────────────────────────────

  const [dismissedIndices, setDismissedIndices] = useState<Set<number>>(new Set());

  const handleSuggestionAccept = (_savedRule: ApprovalRuleProto) => {
    clear();
    refresh();
    setDismissedIndices(new Set());
  };

  const handleSuggestionDiscard = (idx: number) => {
    setDismissedIndices((prev) => new Set([...prev, idx]));
  };

  const visibleSuggestions = suggestions.filter((_, idx) => !dismissedIndices.has(idx));

  // ── aria-live announcement for generate state changes ─────────────────────

  const genLiveMessage = useMemo(() => {
    if (genLoading) return "Generating rule suggestions…";
    if (suggestions.length > 0) return "Rule suggestions ready.";
    return "";
  }, [genLoading, suggestions.length]);

  // ── analytics summary bar ─────────────────────────────────────────────────

  const autoAllowRate = summary ? Math.round(summary.autoApproveRate * 100) : null;
  const manualRate    = summary ? Math.round(summary.manualReviewRate * 100) : null;
  const total         = summary ? summary.totalDecisions : null;

  // ── render ────────────────────────────────────────────────────────────────

  return (
    <div className={panel}>
      {/* ── Header ── */}
      <div className={header}>
        <div className={titleRow}>
          <h2 className={title}>Approval Rules</h2>
          <div className={generateButtonRow}>
            {/* Epic 3: Generate Suggestions button */}
            <button
              onClick={() => {
                setDismissedIndices(new Set());
                void generate({ source: SuggestionSource.ANALYTICS_GAPS });
              }}
              className={generateButton}
              disabled={genLoading}
              data-testid="generate-suggestions"
            >
              {genLoading ? "Generating…" : "Generate Suggestions"}
            </button>
            {genLoading && (
              <button
                onClick={cancel}
                className={cancelGenerateButton}
                data-testid="cancel-generate-button"
              >
                Cancel
              </button>
            )}
            <button
              onClick={refresh}
              className={refreshButton}
              disabled={loading}
              aria-label="Refresh rules"
            >
              {loading ? "⟳" : "↻"}
            </button>
          </div>
        </div>
        <p className={subtitle}>
          Rules are evaluated in priority order before requests reach the manual review queue.
        </p>
      </div>

      {/* ── Accessible live region for generation state ── */}
      <span
        aria-live="polite"
        aria-atomic="true"
        style={{ position: "absolute", width: 1, height: 1, padding: 0, overflow: "hidden", clip: "rect(0,0,0,0)", whiteSpace: "nowrap", border: 0 }}
      >
        {genLiveMessage}
      </span>

      {/* ── Generate error banner ── */}
      {genError && (
        <div className={generateErrorBanner} role="alert" data-testid="generate-error-banner">
          <span>Failed to generate suggestions: {genError.message}</span>
          <button
            className={dismissErrorButton}
            onClick={clear}
            aria-label="Dismiss error"
            data-testid="dismiss-error-button"
          >
            ×
          </button>
        </div>
      )}

      {/* ── 7-day analytics summary ── */}
      {!analyticsLoading && summary && total !== null && total > 0 && (
        <div className={analyticsBar}>
          <span className={analyticsTotal}>{total} decisions (last 7 days)</span>
          <span className={`${analyticsRate} ${rateAllow}`}>
            {autoAllowRate}% auto-allowed
          </span>
          <span className={`${analyticsRate} ${rateManual}`}>
            {manualRate}% manual review
          </span>
          {summary.topTools.length > 0 && (
            <span className={analyticsTopTool}>
              Top tool: {summary.topTools[0].toolName}
            </span>
          )}
        </div>
      )}

      {/* ── Suggested rule cards (Epic 3) ── */}
      {visibleSuggestions.length > 0 && (
        <div className={suggestionsContainer} data-testid="suggestions-container">
          {visibleSuggestions.map((suggestion, i) => (
            <SuggestedRuleCard
              key={i}
              suggestion={suggestion}
              onAccept={handleSuggestionAccept}
              onDiscard={() => handleSuggestionDiscard(suggestions.indexOf(suggestion))}
            />
          ))}
        </div>
      )}

      {/* ── Source filter tabs ── */}
      <div className={tabs}>
        {["all", "user", "seed", "claude-settings"].map((src) => {
          const count = src === "all" ? rules.length : rules.filter((r) => r.source === src).length;
          return (
            <button
              key={src}
              className={`${tab} ${sourceFilter === src ? tabActive : ""}`}
              onClick={() => setSourceFilter(src)}
            >
              {src === "all" ? "All" : sourceLabel(src)}
              {" "}({count})
            </button>
          );
        })}
      </div>

      {/* ── Error ── */}
      {error && (
        <div className={errorClass}>
          Failed to load rules: {error.message}
          <button onClick={refresh} className={retryButton}>Retry</button>
        </div>
      )}

      {/* ── Rules table ── */}
      <div className={tableWrapper}>
        {loading && visibleRules.length === 0 ? (
          <div className={loadingClass}>Loading rules…</div>
        ) : visibleRules.length === 0 ? (
          <div className={empty}>
            No rules found.{" "}
            {sourceFilter === "all" || sourceFilter === "user"
              ? "Add a custom rule below."
              : ""}
          </div>
        ) : (
          <table className={table}>
            <thead>
              <tr>
                <th className={th}>Name</th>
                <th className={th}>Match</th>
                <th className={th}>Decision</th>
                <th className={th}>Source</th>
                <th className={th}>Priority</th>
                <th className={th}>Enabled</th>
                <th className={th}></th>
              </tr>
            </thead>
            <tbody>
              {visibleRules.map((rule) => (
                <tr key={rule.id} className={`${row} ${!rule.enabled ? rowDisabled : ""}`}>
                  <td className={td}>
                    <span className={ruleName}>{rule.name || rule.id}</span>
                    {rule.reason && (
                      <span className={ruleReason}>{rule.reason}</span>
                    )}
                    {rule.alternative && (
                      <span className={ruleAlt}>Alt: {rule.alternative}</span>
                    )}
                  </td>
                  <td className={td}>
                    <div className={matchInfo}>
                      {rule.toolName && <code className={matchChip}>{rule.toolName}</code>}
                      {rule.commandPattern && <code className={matchChip}>{rule.commandPattern}</code>}
                      {rule.toolPattern && <code className={matchChip}>{rule.toolPattern}</code>}
                      {rule.filePattern && <code className={matchChip}>{rule.filePattern}</code>}
                    </div>
                  </td>
                  <td className={td}>
                    <span className={`${decisionBadge} ${decisionClass(rule.decision)}`}>
                      {decisionLabel(rule.decision)}
                    </span>
                  </td>
                  <td className={td}>
                    <span className={sourceBadge}>{sourceLabel(rule.source)}</span>
                  </td>
                  <td className={`${td} ${tdCenter}`}>{rule.priority}</td>
                  <td className={`${td} ${tdCenter}`}>
                    <button
                      className={`${toggle} ${rule.enabled ? toggleOn : toggleOff}`}
                      onClick={() => handleToggle(rule)}
                      disabled={rule.source !== "user"}
                      aria-label={rule.enabled ? "Disable rule" : "Enable rule"}
                      title={rule.source !== "user" ? "Built-in rules cannot be toggled" : undefined}
                    >
                      {rule.enabled ? "ON" : "OFF"}
                    </button>
                  </td>
                  <td className={`${td} ${tdCenter}`}>
                    {rule.source === "user" && (
                      <button
                        className={deleteButton}
                        onClick={() => deleteRule(rule.id)}
                        aria-label={`Delete rule ${rule.name}`}
                        title="Delete rule"
                      >
                        ✕
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* ── Add rule form ── */}
      <div className={formSection} ref={formSectionRef}>
        {!showForm ? (
          <button className={addButton} onClick={openForm}>
            + Add Custom Rule
          </button>
        ) : (
          <div className={formClass}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <h3 className={formTitle}>New Rule</h3>
              {aiPrefilled && (
                <span className={aiGeneratedBadge} data-testid="ai-generated-badge">
                  AI-generated — review before saving
                </span>
              )}
            </div>

            {/* ── Epic 6: Generate from command (collapsible) ── */}
            <details className={commandSampleDetails} data-testid="generate-from-command-details">
              <summary className={commandSampleSummary}>
                Generate from command (optional)
              </summary>
              <div className={commandSampleBody}>
                <textarea
                  className={commandSampleTextarea}
                  placeholder="Paste a raw command, e.g. git push origin main"
                  value={cmdSampleValue}
                  onChange={(e) => setCmdSampleValue(e.target.value)}
                  aria-label="Command sample"
                  data-testid="command-sample-textarea"
                  rows={2}
                />
                <div className={commandSampleActions}>
                  <button
                    className={generateButton}
                    type="button"
                    disabled={cmdGenLoading || !cmdSampleValue.trim()}
                    onClick={() => {
                      void cmdGenerate({
                        source: SuggestionSource.COMMAND_SAMPLE,
                        commandSample: cmdSampleValue.trim(),
                      });
                    }}
                    data-testid="command-sample-generate-button"
                  >
                    {cmdGenLoading ? "Generating…" : "Generate"}
                  </button>
                </div>
              </div>
            </details>

            {formError && <div className={formErrorClass}>{formError}</div>}

            <div className={formGrid}>
              <label className={label}>
                Name *
                <input
                  className={input}
                  value={form.name}
                  onChange={(e) => setFormField("name", e.target.value)}
                  placeholder="e.g. Allow git log"
                  data-testid="form-name-input"
                />
              </label>

              <label className={label}>
                Decision *
                <select
                  className={select}
                  value={form.decision}
                  onChange={(e) => setFormField("decision", Number(e.target.value) as AutoDecision)}
                >
                  <option value={AutoDecision.ALLOW}>Auto-Allow</option>
                  <option value={AutoDecision.DENY}>Auto-Deny</option>
                  <option value={AutoDecision.ESCALATE}>Escalate (manual)</option>
                </select>
              </label>

              <label className={label}>
                Tool Name
                <input
                  className={input}
                  value={form.toolName}
                  onChange={(e) => setFormField("toolName", e.target.value)}
                  placeholder="e.g. Bash"
                />
              </label>

              <label className={label}>
                Command Pattern (regex)
                <input
                  className={input}
                  value={form.commandPattern}
                  onChange={(e) => setFormField("commandPattern", e.target.value)}
                  placeholder="e.g. ^git log"
                  data-testid="form-command-pattern-input"
                />
              </label>

              <label className={label}>
                Tool Pattern (regex)
                <input
                  className={input}
                  value={form.toolPattern}
                  onChange={(e) => setFormField("toolPattern", e.target.value)}
                  placeholder="e.g. Read|Glob"
                />
              </label>

              <label className={label}>
                File Pattern (regex)
                <input
                  className={input}
                  value={form.filePattern}
                  onChange={(e) => setFormField("filePattern", e.target.value)}
                  placeholder="e.g. \.md$"
                />
              </label>

              <label className={label}>
                Reason
                <input
                  className={input}
                  value={form.reason}
                  onChange={(e) => setFormField("reason", e.target.value)}
                  placeholder="Shown to Claude when denied"
                />
              </label>

              <label className={label}>
                Alternative
                <input
                  className={input}
                  value={form.alternative}
                  onChange={(e) => setFormField("alternative", e.target.value)}
                  placeholder="Safer command suggestion"
                />
              </label>

              <label className={label}>
                Priority
                <input
                  className={input}
                  type="number"
                  min={1}
                  max={999}
                  value={form.priority}
                  onChange={(e) => setFormField("priority", Number(e.target.value))}
                />
              </label>
            </div>

            <div className={formActions}>
              <button
                className={saveButton}
                onClick={handleSave}
                disabled={saving}
              >
                {saving ? "Saving…" : "Save Rule"}
              </button>
              <button
                className={cancelButton}
                onClick={closeForm}
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
