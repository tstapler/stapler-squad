"use client";

import { useEffect, useMemo, useState } from "react";
import { useApprovalRules } from "@/lib/hooks/useApprovalRules";
import { useApprovalAnalytics } from "@/lib/hooks/useApprovalAnalytics";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { useExportRules } from "@/lib/hooks/useExportRules";
import { ApprovalRuleProto, AutoDecision, SuggestionSource } from "@/gen/session/v1/types_pb";

type SortKey = "name" | "decision" | "priority" | "hits";
type SortDir = "asc" | "desc";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import { ImportRulesModal } from "./ImportRulesModal";
import { RuleBuilderPrefill } from "@/lib/ruleBuilderPrefill";
import { RuleTemplate } from "@/lib/ruleTemplates";
import { RuleBuilderForm } from "@/components/rules/RuleBuilderForm";
import { TemplateLibrary } from "@/components/rules/TemplateLibrary";
import { MatchDescription } from "@/components/rules/MatchDescription";
import { SeverityBadge } from "./SeverityBadge";
import {
  panel, header, titleRow, title, subtitle, refreshButton,
  analyticsBar, analyticsTotal, analyticsRate, rateAllow, rateManual, analyticsTopTool,
  tabs, tab, tabActive, tabLabelFull, tabLabelShort,
  error as errorClass, retryButton,
  loading as loadingClass, empty,
  tableWrapper, table, th, td, tdCenter, row, rowDisabled,
  ruleName, ruleReason, ruleAlt, matchChip,
  decisionBadge, decisionAllow, decisionDeny, decisionEscalate,
  sourceBadge, configFileBadge, toggle, toggleOn, toggleOff, deleteButton, builtInBadge,
  addButton, mobileAddFab, headerButtonsHiddenOnMobile,
  formSection,
  generateButtonRow, generateButton, cancelGenerateButton,
  generateErrorBanner, dismissErrorButton, suggestionsContainer,
  ruleModalContent, rowCount,
  searchBar, thSortable, hitBadge, hitBadgeActive,
  configFileHint,
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
    case "config":          return "Config File";
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

  // ── Epic 6: command-sample generate hook (for the in-form "Generate from command" section) ─
  const {
    suggestions: cmdSuggestions,
    loading: cmdLoading,
    generate: cmdGenerate,
    cancel: cmdCancel,
    clear: cmdClear,
  } = useGenerateRule();

  const [cmdSampleText, setCmdSampleText] = useState("");

  const [sourceFilter, setSourceFilter] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("priority");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [importModalOpen, setImportModalOpen] = useState(false);

  // ── URL param pre-fill ───────────────────────────────────────────────────
  const urlPrefill = useMemo<RuleBuilderPrefill | null>(() => {
    if (typeof window === "undefined") return null;
    const p = new URLSearchParams(window.location.search);
    const tool = p.get("tool");
    const program = p.get("program");
    const subcommand = p.get("subcommand");
    const open = p.get("open");
    if (tool) return { toolName: tool, initialName: `Allow ${tool}` };
    if (program) {
      const name = subcommand ? `Allow ${program} ${subcommand}` : `Allow ${program}`;
      return { programs: [program], subcommands: subcommand ? [subcommand] : undefined, initialName: name };
    }
    if (open) return {};
    return null;
  }, []);

  const hasUrlParams = urlPrefill !== null;
  const [showBuilder, setShowBuilder] = useState(!!prefill || hasUrlParams);
  const [editingRule, setEditingRule] = useState<ApprovalRuleProto | null>(null);
  const [templateSeed, setTemplateSeed] = useState<RuleTemplate | null>(null);
  const [showTemplates, setShowTemplates] = useState(false);

  // ── Escape key to close builder ──────────────────────────────────────────
  useEffect(() => {
    if (!showBuilder) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") { setShowBuilder(false); setEditingRule(null); setTemplateSeed(null); }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [showBuilder]);

  // Merge prop prefill, URL prefill, and cmd suggestions into effective prefill
  const cmdSuggestion = cmdSuggestions[0] ?? null;
  const effectivePrefill: RuleBuilderPrefill | null = (() => {
    const base = prefill ?? urlPrefill;
    if (!cmdSuggestion) return base;
    return { ...base, commandPattern: cmdSuggestion.commandPattern, isAiGenerated: true };
  })();

  // ── hit counts from analytics ─────────────────────────────────────────────

  const hitCountByRuleId = useMemo(() => {
    const m = new Map<string, number>();
    for (const r of summary?.topTriggeredRules ?? []) {
      m.set(r.ruleId, (m.get(r.ruleId) ?? 0) + r.count);
    }
    return m;
  }, [summary]);

  // ── sort toggle ───────────────────────────────────────────────────────────

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(key === "priority" || key === "hits" ? "desc" : "asc");
    }
  }

  function sortIcon(key: SortKey) {
    if (sortKey !== key) return " ↕";
    return sortDir === "asc" ? " ↑" : " ↓";
  }

  // ── filter + sort ─────────────────────────────────────────────────────────

  const visibleRules = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    let filtered = sourceFilter === "all" ? rules : rules.filter((r) => r.source === sourceFilter);
    if (q) {
      filtered = filtered.filter((r) =>
        r.name.toLowerCase().includes(q) ||
        r.reason.toLowerCase().includes(q) ||
        r.toolName.toLowerCase().includes(q) ||
        r.toolPattern.toLowerCase().includes(q) ||
        r.commandPattern.toLowerCase().includes(q) ||
        r.programs.some((p) => p.toLowerCase().includes(q))
      );
    }
    return [...filtered].sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "name":     cmp = a.name.localeCompare(b.name); break;
        case "decision": cmp = a.decision - b.decision; break;
        case "priority": cmp = a.priority - b.priority; break;
        case "hits":     cmp = (hitCountByRuleId.get(a.id) ?? 0) - (hitCountByRuleId.get(b.id) ?? 0); break;
      }
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [rules, sourceFilter, searchQuery, sortKey, sortDir, hitCountByRuleId]);

  // ── save ──────────────────────────────────────────────────────────────────

  const handleSave = async (rule: Partial<ApprovalRuleProto> & { id: string }) => {
    await upsertRule(rule);
    setShowBuilder(false);
    setEditingRule(null);
    setTemplateSeed(null);
  };

  const handleCancel = () => {
    setShowBuilder(false);
    setEditingRule(null);
    setTemplateSeed(null);
    cmdClear();
    setCmdSampleText("");
  };

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

      {/* ── Search ── */}
      <input
        className={searchBar}
        type="search"
        placeholder="Search by name, tool, program, pattern…"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
        aria-label="Search rules"
      />

      {/* ── Source filter tabs ── */}
      <div className={tabs}>
        {(["all", "user", "config", "seed", "claude-settings"] as const).map((src) => {
          const count = src === "all" ? rules.length : rules.filter((r) => r.source === src).length;
          const fullLabel = src === "all" ? "All" : sourceLabel(src);
          const shortLabel = src === "claude-settings" ? "Settings" : src === "config" ? "Config" : fullLabel;
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
      {/* ── Config file path hint (shown when viewing config tab) ── */}
      {sourceFilter === "config" && (
        <div className={configFileHint}>
          Stored in ~/.config/stapler-squad/shared_rules.yaml
        </div>
      )}

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
                  onClick={() => { setTemplateSeed(null); setEditingRule(null); setShowBuilder(true); }}
                >
                  Add Rule
                </button>
                {" "}using the builder below or{" "}
                <button
                  style={{ background: "none", border: "none", cursor: "pointer", color: "inherit", textDecoration: "underline", padding: 0 }}
                  onClick={() => setImportModalOpen(true)}
                >
                  Import YAML
                </button>
                {" "}to import from a file.
              </p>
            )}
            {sourceFilter === "seed" && (
              <p>No built-in rules are available in this workspace.</p>
            )}
            {sourceFilter === "claude-settings" && (
              <p>No rules from your ~/.claude/settings.json file were found.</p>
            )}
            {sourceFilter === "config" && (
              <p>No rules in your config file yet. Use the &quot;→ Config&quot; button on a custom rule to copy it to <code>~/.config/stapler-squad/shared_rules.yaml</code>, or create a new rule and select &quot;Save to Config File&quot;.</p>
            )}
          </div>
        ) : (
          <table className={table}>
            <thead>
              <tr>
                <th className={`${th} ${thSortable}`} onClick={() => handleSort("name")}>
                  Name{sortIcon("name")}
                </th>
                <th className={th}>Match</th>
                <th className={`${th} ${thSortable}`} onClick={() => handleSort("decision")}>
                  Decision{sortIcon("decision")}
                </th>
                <th className={th}>Risk</th>
                <th className={th}>Source</th>
                <th
                  className={`${th} ${thSortable}`}
                  title="Higher priority fires first. Custom rules default to 10; built-in rules default to 100–1000."
                  onClick={() => handleSort("priority")}
                >
                  Priority ⓘ{sortIcon("priority")}
                </th>
                <th
                  className={`${th} ${thSortable}`}
                  title="Number of times this rule fired in the last 7 days"
                  onClick={() => handleSort("hits")}
                >
                  Hits (7d){sortIcon("hits")}
                </th>
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
                    <SeverityBadge riskLevel={rule.riskLevel} compact />
                  </td>
                  <td className={td}>
                    <span
                      className={rule.source === "config" ? configFileBadge : sourceBadge}
                      title={
                        rule.source === "seed"
                          ? "These rules ship with stapler-squad and cannot be deleted"
                          : rule.source === "claude-settings"
                          ? "These rules come from your ~/.claude/settings.json file"
                          : rule.source === "config"
                          ? "This rule is stored in ~/.config/stapler-squad/shared_rules.yaml and can be shared"
                          : undefined
                      }
                    >
                      {sourceLabel(rule.source)}
                    </span>
                  </td>
                  <td className={`${td} ${tdCenter}`}>{rule.priority}</td>
                  <td className={`${td} ${tdCenter}`}>
                    {(() => {
                      const hits = hitCountByRuleId.get(rule.id) ?? 0;
                      return hits > 0
                        ? <span className={`${hitBadge} ${hitBadgeActive}`}>{hits.toLocaleString()}</span>
                        : <span className={hitBadge}>—</span>;
                    })()}
                  </td>
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
          {(sourceFilter !== "all" || searchQuery.trim()) && ` (filtered from ${rules.length} total)`}
        </div>
      )}

      {/* ── Mobile FAB ── */}
      <button
        className={mobileAddFab}
        onClick={() => { setTemplateSeed(null); setEditingRule(null); setShowBuilder(true); }}
        aria-label="Add rule"
        data-testid="add-rule-fab"
      >
        +
      </button>

      {/* ── Rule Builder ── */}
      <div className={formSection} id="rule-builder" {...(showBuilder ? { role: "dialog" } : {})}>
        {!showBuilder ? (
          <div style={{ display: "flex", gap: "8px", flexWrap: "wrap" }}>
            <button data-testid="add-rule-button" className={addButton} onClick={() => { setTemplateSeed(null); setEditingRule(null); setShowBuilder(true); }}>
              + Add Custom Rule
            </button>
            <button
              className={addButton}
              style={{ background: "transparent", border: "1px solid var(--border-color)", color: "inherit" }}
              onClick={() => setShowTemplates(true)}
            >
              Start from Template
            </button>
          </div>
        ) : (
          <>
            <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 8 }}>
              <button
                aria-label="Close dialog"
                onClick={handleCancel}
                style={{ background: "none", border: "none", cursor: "pointer", fontSize: 18, color: "var(--text-secondary)", lineHeight: 1 }}
              >
                ×
              </button>
            </div>
            {/* ── Epic 6: Generate from command section ── */}
            <details data-testid="generate-from-command-details" style={{ marginBottom: 12 }}>
              <summary style={{ cursor: "pointer", fontSize: 13, color: "var(--text-secondary)" }}>Generate from command sample</summary>
              <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 8 }}>
                <textarea
                  data-testid="command-sample-textarea"
                  value={cmdSampleText}
                  onChange={(e) => setCmdSampleText(e.target.value)}
                  placeholder="Paste a command you ran, e.g. git push origin main"
                  rows={3}
                  style={{ width: "100%", fontFamily: "monospace", fontSize: 12, padding: "6px 8px", borderRadius: 6, border: "1px solid var(--border-color)", background: "var(--input-background)", color: "var(--input-text)", resize: "vertical" }}
                />
                <div style={{ display: "flex", gap: 8 }}>
                  <button
                    data-testid="command-sample-generate-button"
                    disabled={!cmdSampleText.trim() || cmdLoading}
                    onClick={() => { void cmdGenerate({ source: SuggestionSource.COMMAND_SAMPLE, commandSample: cmdSampleText }); }}
                    style={{ padding: "4px 12px", fontSize: 12, borderRadius: 6, border: "none", cursor: cmdSampleText.trim() ? "pointer" : "default", background: "var(--primary)", color: "var(--primary-text)" }}
                  >
                    {cmdLoading ? "Generating…" : "Generate"}
                  </button>
                  {cmdLoading && (
                    <button
                      onClick={cmdCancel}
                      style={{ padding: "4px 12px", fontSize: 12, borderRadius: 6, border: "1px solid var(--border-color)", background: "none", cursor: "pointer" }}
                    >
                      Cancel
                    </button>
                  )}
                </div>
              </div>
            </details>
            <RuleBuilderForm
              editRule={editingRule}
              prefill={showBuilder ? effectivePrefill : null}
              templateSeed={templateSeed}
              onSave={handleSave}
              onCancel={handleCancel}
              subcommandStats={summary?.commandSubcommandStats ?? []}
            />
          </>
        )}
      </div>

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
