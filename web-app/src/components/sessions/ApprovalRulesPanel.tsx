"use client";

import { useMemo, useEffect, useRef, useState } from "react";
import { useApprovalRules } from "@/lib/hooks/useApprovalRules";
import { useApprovalAnalytics } from "@/lib/hooks/useApprovalAnalytics";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { useExportRules } from "@/lib/hooks/useExportRules";
import { ApprovalRuleProto, AutoDecision, SuggestionSource } from "@/gen/session/v1/types_pb";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import { ImportRulesModal } from "./ImportRulesModal";
import { RuleBuilderPrefill } from "@/lib/ruleBuilderPrefill";
import { RuleTemplate } from "@/lib/ruleTemplates";
import { RuleBuilderForm } from "@/components/rules/RuleBuilderForm";
import { TemplateLibrary } from "@/components/rules/TemplateLibrary";
import { MatchDescription } from "@/components/rules/MatchDescription";
import {
  panel, header, titleRow, title, subtitle, refreshButton,
  analyticsBar, analyticsTotal, analyticsRate, rateAllow, rateManual, analyticsTopTool,
  tabs, tab, tabActive, tabLabelFull, tabLabelShort,
  error as errorClass, retryButton,
  loading as loadingClass, empty,
  tableWrapper, table, th, td, tdCenter, row, rowDisabled,
  ruleName, ruleReason, ruleAlt, matchChip,
  decisionBadge, decisionAllow, decisionDeny, decisionEscalate,
  sourceBadge, toggle, toggleOn, toggleOff, deleteButton, builtInBadge,
  addButton, mobileAddFab, headerButtonsHiddenOnMobile,
  formSection,
  generateButtonRow, generateButton, cancelGenerateButton,
  generateErrorBanner, dismissErrorButton, suggestionsContainer,
  ruleModalContent, rowCount,
} from "./ApprovalRulesPanel.css";

// ── types ────────────────────────────────────────────────────────────────────

/** Minimal form state shape used for URL-param pre-fill tracking. */
interface RuleFormState {
  name: string;
  toolName: string;
  commandPattern: string;
}

const emptyForm: RuleFormState = {
  name: "",
  toolName: "",
  commandPattern: "",
};

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

// ── component ─────────────────────────────────────────────────────────────────

interface ApprovalRulesPanelProps {
  prefill?: RuleBuilderPrefill | null;
}

/**
 * ApprovalRulesPanel shows the list of auto-approval rules and lets users
 * create, edit, toggle, and delete custom rules via the structured rule builder.
 */
export function ApprovalRulesPanel({ prefill }: ApprovalRulesPanelProps) {
  const { rules, loading, error, upsertRule, deleteRule, refresh } = useApprovalRules();
  const { summary, loading: analyticsLoading } = useApprovalAnalytics({ windowDays: 7 });
  const { exportRules, loading: exporting, error: exportError } = useExportRules();

  // ── Epic 3: panel-level "Generate Suggestions" hook ─────────────────────
  const {
    suggestions,
    loading: genLoading,
    error: genError,
    generate,
    cancel,
    clear,
  } = useGenerateRule();

  // ── Epic 6: command-sample generation hook (second useGenerateRule instance) ─
  const {
    suggestions: cmdSuggestions,
    loading: cmdLoading,
    generate: cmdGenerate,
    clear: cmdClear,
  } = useGenerateRule();

  const [sourceFilter, setSourceFilter] = useState<string>("all");
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [showBuilder, setShowBuilder] = useState(!!prefill);
  const [editingRule, setEditingRule] = useState<ApprovalRuleProto | null>(null);
  const [templateSeed, setTemplateSeed] = useState<RuleTemplate | null>(null);
  const [showTemplates, setShowTemplates] = useState(false);
  // Prefill derived from URL params, passed to RuleBuilderForm
  const [urlPrefill, setUrlPrefill] = useState<RuleBuilderPrefill | null>(null);

  // State and refs for URL-param pre-fill flow (mirrors old inline-form API).
  const setShowForm = setShowBuilder;
  const [, setForm] = useState<RuleFormState>(emptyForm);
  const [, setFormError] = useState<string | null>(null);
  const [, setAiPrefilled] = useState(false);
  const [, setCmdSampleValue] = useState("");
  const formSectionRef = useRef<HTMLElement | null>(null);

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
    const openParam = params.get("open");

    // Allow ?open=1 to open a blank dialog
    if (!tool && !program && !openParam) return;

    const builtPrefill: RuleBuilderPrefill = {};
    const formPrefill: Partial<RuleFormState> = {};
    if (tool) {
      builtPrefill.toolName = tool;
      builtPrefill.suggestedDecision = AutoDecision.ALLOW as number;
      builtPrefill.initialName = `Allow ${tool}`;
      formPrefill.toolName = tool;
      formPrefill.name = `Allow ${tool}`;
    } else if (program) {
      // Escape regex metacharacters so values like "docker-compose" or "node.js"
      // produce valid patterns. Use (?:^|\s) / (?:\s|$) word boundaries because
      // \b does not match at hyphen boundaries.
      const esc = escapeRegex(program);
      // Don't set toolName in builtPrefill for program-based prefill:
      // auto-name should derive name from programs array (e.g. "Allow git"), not toolName.
      builtPrefill.suggestedDecision = AutoDecision.ALLOW as number;
      formPrefill.toolName = "Bash";
      if (subcommand) {
        const escSub = escapeRegex(subcommand);
        const pattern = `(?:^|\\s)${esc}(?:\\s|$).*(?:^|\\s)${escSub}(?:\\s|$)`;
        builtPrefill.commandPattern = pattern;
        builtPrefill.programs = [program];
        builtPrefill.subcommands = [subcommand];
        builtPrefill.initialName = `Allow ${program} ${subcommand}`;
        formPrefill.commandPattern = pattern;
        formPrefill.name = `Allow ${program} ${subcommand}`;
      } else {
        const pattern = `(?:^|\\s)${esc}(?:\\s|$)`;
        builtPrefill.commandPattern = pattern;
        builtPrefill.programs = [program];
        builtPrefill.initialName = `Allow ${program}`;
        formPrefill.commandPattern = pattern;
        formPrefill.name = `Allow ${program}`;
      }
    }

    setUrlPrefill(Object.keys(builtPrefill).length > 0 ? builtPrefill : null);
    setShowForm(true);
    setForm({ ...emptyForm, ...formPrefill });
    setFormError(null);
    setAiPrefilled(false);
    setCmdSampleValue("");
    touchedFieldsRef.current = new Set();
    cmdClear();

    setTimeout(() => {
      formSectionRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 100);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Merge prop prefill with any URL-param derived prefill (prop takes priority).
  const effectivePrefill = prefill ?? null;

  // ── filter ────────────────────────────────────────────────────────────────

  const visibleRules = sourceFilter === "all"
    ? rules
    : rules.filter((r) => r.source === sourceFilter);

  // ── save ──────────────────────────────────────────────────────────────────

  const handleSave = async (rule: Partial<ApprovalRuleProto> & { id: string }) => {
    await upsertRule(rule);
    setShowBuilder(false);
    setEditingRule(null);
    setTemplateSeed(null);
    setUrlPrefill(null);
    cmdClear();
  };

  const handleCancel = () => {
    setShowBuilder(false);
    setEditingRule(null);
    setTemplateSeed(null);
    setUrlPrefill(null);
    cmdClear();
  };

  // Global Escape key handler to close the dialog
  useEffect(() => {
    if (!showBuilder) return;
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape") handleCancel();
    };
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showBuilder]);

  const handleEdit = (rule: ApprovalRuleProto) => {
    setEditingRule(rule);
    setTemplateSeed(null);
    setShowBuilder(true);
    setTimeout(() => {
      document.getElementById("rule-builder")?.scrollIntoView({ behavior: "smooth" });
    }, 50);
  };

  const handleTemplateSelect = (tpl: RuleTemplate) => {
    setTemplateSeed(tpl);
    setEditingRule(null);
    setShowBuilder(true);
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
        toolCategory: rule.toolCategory,
        commandPattern: rule.commandPattern,
        filePattern: rule.filePattern,
        decision: rule.decision,
        riskLevel: rule.riskLevel,
        reason: rule.reason,
        alternative: rule.alternative,
        priority: rule.priority,
        enabled: !rule.enabled,
        programs: rule.programs,
        subcommands: rule.subcommands,
        blockedSubcommands: rule.blockedSubcommands,
        requiredFlags: rule.requiredFlags,
        forbiddenFlags: rule.forbiddenFlags,
        requiredFlagPrefixes: rule.requiredFlagPrefixes,
        pythonModes: rule.pythonModes,
        safePythonImportsOnly: rule.safePythonImportsOnly,
      });
    } catch (e) {
      console.error("Failed to toggle rule:", e);
    }
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
            {/* Generate Suggestions button — hidden on mobile (low priority action) */}
            <button
              onClick={() => {
                setDismissedIndices(new Set());
                void generate({ source: SuggestionSource.ANALYTICS_GAPS });
              }}
              className={`${generateButton} ${headerButtonsHiddenOnMobile}`}
              disabled={genLoading}
              data-testid="generate-suggestions"
            >
              {genLoading ? "Generating…" : "Generate Suggestions"}
            </button>
            {genLoading && (
              <button
                onClick={cancel}
                className={`${cancelGenerateButton} ${headerButtonsHiddenOnMobile}`}
                data-testid="cancel-generate-button"
              >
                Cancel
              </button>
            )}
            {/* Export YAML button */}
            <button
              className={`${addButton} ${headerButtonsHiddenOnMobile}`}
              onClick={() => void exportRules()}
              disabled={exporting}
              data-testid="export-yaml-button"
            >
              {exporting ? "Exporting…" : "Export YAML"}
            </button>
            {/* Import YAML button */}
            <button
              className={`${addButton} ${headerButtonsHiddenOnMobile}`}
              onClick={() => setImportModalOpen(true)}
              data-testid="import-yaml-button"
            >
              Import YAML
            </button>
            <button
              onClick={refresh}
              className={refreshButton}
              disabled={loading}
              aria-label="Refresh rules"
              title="Refresh rules"
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

      {/* ── Export error banner ── */}
      {exportError && (
        <div className={generateErrorBanner} role="alert" data-testid="export-error-banner">
          <span>Export failed: {exportError.message}</span>
        </div>
      )}

      {/* ── 7-day analytics summary ── */}
      {!analyticsLoading && summary && total !== null && total > 0 && (
        <div className={analyticsBar}>
          <span className={analyticsTotal}>{total.toLocaleString()} decisions</span>
          <span style={{ color: "var(--text-muted)", fontSize: 12 }}>last 7 days</span>
          <span className={`${analyticsRate} ${rateAllow}`}>{autoAllowRate}% auto-allowed</span>
          <span className={`${analyticsRate} ${rateManual}`}>{manualRate}% manual review</span>
          {summary.topTools.length > 0 && (
            <span className={analyticsTopTool}>Top tool: {summary.topTools[0].toolName}</span>
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
        {(["all", "user", "seed", "claude-settings"] as const).map((src) => {
          const count = src === "all" ? rules.length : rules.filter((r) => r.source === src).length;
          const fullLabel = src === "all" ? "All" : sourceLabel(src);
          const shortLabel = src === "claude-settings" ? "Settings" : fullLabel;
          return (
            <button
              key={src}
              className={`${tab} ${sourceFilter === src ? tabActive : ""}`}
              onClick={() => setSourceFilter(src)}
            >
              <span className={tabLabelFull}>{fullLabel}</span>
              <span className={tabLabelShort}>{shortLabel}</span>
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
          <div className={empty} data-testid="empty-state">
            <p>
              Approval rules let you automatically allow or deny tool calls from Claude without manual review.
            </p>
            {(sourceFilter === "all" || sourceFilter === "user") && (
              <p>
                <button
                  style={{ background: "none", border: "none", cursor: "pointer", color: "inherit", textDecoration: "underline", padding: 0 }}
                  onClick={() => { setTemplateSeed(null); setEditingRule(null); setUrlPrefill(null); cmdClear(); setShowBuilder(true); }}
                >
                  Add Rule
                </button>
                {" "}or{" "}
                <button
                  style={{ background: "none", border: "none", cursor: "pointer", color: "inherit", textDecoration: "underline", padding: 0 }}
                  onClick={() => setImportModalOpen(true)}
                >
                  Import YAML
                </button>
                {" "}to get started.
              </p>
            )}
            {sourceFilter === "seed" && (
              <p>No built-in rules are available in this workspace.</p>
            )}
            {sourceFilter === "claude-settings" && (
              <p>No rules from your ~/.claude/settings.json file were found.</p>
            )}
          </div>
        ) : (
          <table className={table}>
            <thead>
              <tr>
                <th className={th}>Name</th>
                <th className={th}>Match</th>
                <th className={th}>Decision</th>
                <th className={th}>Source</th>
                <th className={th} title="Lower numbers run first. Custom rules (default: 10) are evaluated before built-in rules (default: 1000).">Priority ⓘ</th>
                <th className={th}>Enabled</th>
                <th className={th}></th>
              </tr>
            </thead>
            <tbody>
              {visibleRules.map((rule) => (
                <tr key={rule.id} className={`${row} ${!rule.enabled ? rowDisabled : ""}`}>
                  <td className={td}>
                    <span className={ruleName}>{rule.name || rule.id}</span>
                    {rule.reason && <span className={ruleReason}>{rule.reason}</span>}
                    {rule.alternative && <span className={ruleAlt}>Alt: {rule.alternative}</span>}
                  </td>
                  <td className={td}>
                    <MatchDescription rule={rule} matchChipClass={matchChip} />
                  </td>
                  <td className={td}>
                    <span className={`${decisionBadge} ${decisionClass(rule.decision)}`}>
                      {decisionLabel(rule.decision)}
                    </span>
                  </td>
                  <td className={td}>
                    <span
                      className={sourceBadge}
                      title={
                        rule.source === "seed"
                          ? "These rules ship with stapler-squad and cannot be deleted"
                          : rule.source === "claude-settings"
                          ? "These rules come from your ~/.claude/settings.json file"
                          : undefined
                      }
                    >
                      {sourceLabel(rule.source)}
                    </span>
                  </td>
                  <td className={`${td} ${tdCenter}`}>{rule.priority}</td>
                  <td className={`${td} ${tdCenter}`}>
                    {rule.source === "user" ? (
                      <button
                        className={`${toggle} ${rule.enabled ? toggleOn : toggleOff}`}
                        onClick={() => handleToggle(rule)}
                        aria-label={rule.enabled ? "Disable rule" : "Enable rule"}
                      >
                        {rule.enabled ? "ON" : "OFF"}
                      </button>
                    ) : (
                      <span className={builtInBadge} title="Built-in rules cannot be disabled">
                        Always on
                      </span>
                    )}
                  </td>
                  <td className={`${td} ${tdCenter}`} style={{ display: "flex", gap: "4px", alignItems: "center" }}>
                    {rule.source === "user" && (
                      <>
                        <button
                          style={{ fontSize: "0.75rem", padding: "2px 8px", cursor: "pointer", border: "1px solid var(--border-color)", borderRadius: "4px", background: "transparent", color: "inherit" }}
                          onClick={() => handleEdit(rule)}
                          aria-label={`Edit rule ${rule.name}`}
                        >
                          Edit
                        </button>
                        <button
                          className={deleteButton}
                          onClick={() => deleteRule(rule.id)}
                          aria-label={`Delete rule ${rule.name}`}
                          title="Delete rule"
                        >
                          ✕
                        </button>
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>


      {/* ── Row count indicator ── */}
      {visibleRules.length > 0 && (
        <div className={rowCount}>
          {visibleRules.length} rule{visibleRules.length !== 1 ? "s" : ""}
          {sourceFilter !== "all" && ` (filtered from ${rules.length} total)`}
        </div>
      )}

      {/* ── Mobile FAB ── */}
      <button
        className={mobileAddFab}
        onClick={() => { setTemplateSeed(null); setEditingRule(null); setUrlPrefill(null); cmdClear(); setShowBuilder(true); }}
        aria-label="Add rule"
        data-testid="add-rule-button"
      >
        +
      </button>

      {/* ── Rule Builder Dialog ── */}
      {showBuilder && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={editingRule ? `Edit rule: ${editingRule.name}` : "Add Rule"}
          style={{
            position: "fixed", top: 0, left: 0, right: 0, bottom: 0,
            background: "rgba(0,0,0,0.5)", zIndex: 1000,
            display: "flex", alignItems: "flex-start", justifyContent: "center",
            padding: "48px 16px",
            overflowY: "auto",
          }}
          onClick={(e) => { if (e.target === e.currentTarget) handleCancel(); }}
          onKeyDown={(e) => { if (e.key === "Escape") handleCancel(); }}
        >
          <div
            style={{
              background: "var(--card-background, #1a1a2e)", borderRadius: "8px",
              padding: "24px", maxWidth: "640px", width: "100%",
              position: "relative",
            }}
          >
            <button
              aria-label="Close dialog"
              onClick={handleCancel}
              style={{
                position: "absolute", top: "12px", right: "12px",
                background: "none", border: "none", cursor: "pointer",
                fontSize: "1.25rem", lineHeight: 1, color: "inherit",
              }}
            >
              ×
            </button>
            <RuleBuilderForm
              editRule={editingRule}
              prefill={urlPrefill ?? (showBuilder ? effectivePrefill : null)}
              templateSeed={templateSeed}
              onSave={handleSave}
              onCancel={handleCancel}
              onCmdGenerate={cmdGenerate}
              cmdSuggestions={cmdSuggestions}
              cmdLoading={cmdLoading}
              cmdClear={cmdClear}
            />
          </div>
        </div>
      )}

      <TemplateLibrary
        open={showTemplates}
        onClose={() => setShowTemplates(false)}
        onSelect={handleTemplateSelect}
      />

      {/* ── Import Rules Modal ── */}
      <ImportRulesModal
        open={importModalOpen}
        onClose={() => setImportModalOpen(false)}
        onApplied={() => { void refresh(); }}
        existingRules={rules}
      />
    </div>
  );
}
