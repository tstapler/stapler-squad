"use client";

import { useState, useEffect } from "react";
import { ApprovalRuleProto, AutoDecision } from "@/gen/session/v1/types_pb";
import { RuleBuilderPrefill } from "@/lib/ruleBuilderPrefill";
import { RuleTemplate } from "@/lib/ruleTemplates";
import { TagInput } from "./TagInput";
import { RulePreview } from "./RulePreview";
import { RuleCriteria } from "@/lib/rulePreview";
import {
  formWrapper, formTitle, modeToggle, modeBtn, modeBtnActive,
  section, sectionTitle, formGrid, formGridFull, fieldLabel, fieldInput, fieldSelect,
  decisionSegment, decisionBtn, decisionAllow, decisionDeny, decisionEscalate,
  checkboxRow, priorityHint, priorityWarning, errorBanner, actions, saveBtn, cancelBtn,
  advancedToggle, radioGroup, radioRow, pythonSection, pythonSectionTitle, checkboxGrid,
} from "./RuleBuilderForm.css";

type Mode = "structured" | "regex";
type ToolTarget = "name" | "category" | "pattern";

const PYTHON_PROGRAMS = new Set(["python", "python2", "python3", "pypy", "pypy3"]);
function isPythonProgram(p: string) {
  return PYTHON_PROGRAMS.has(p.toLowerCase()) || p.toLowerCase().startsWith("python");
}

const PYTHON_MODES = [
  { value: "script", label: "Script (.py file)" },
  { value: "module", label: "Module (-m)" },
  { value: "inline", label: "Inline (-c)" },
  { value: "version", label: "Version (-V)" },
];

const TOOL_CATEGORIES = [
  { value: "", label: "— select category —" },
  { value: "builtin", label: "Built-in tools (Read, Edit, Bash…)" },
  { value: "builtin-agent", label: "Agent/planning tools" },
  { value: "mcp-read", label: "MCP read-only tools" },
  { value: "mcp-write", label: "MCP write tools" },
  { value: "mcp", label: "Any MCP tool" },
];

function defaultPriority(decision: AutoDecision): number {
  switch (decision) {
    case AutoDecision.DENY: return 950;
    case AutoDecision.ALLOW: return 100;
    default: return 450;
  }
}

interface RuleBuilderFormProps {
  editRule?: ApprovalRuleProto | null;
  prefill?: RuleBuilderPrefill | null;
  templateSeed?: RuleTemplate | null;
  onSave: (rule: Partial<ApprovalRuleProto> & { id: string }) => Promise<void>;
  onCancel: () => void;
}

export function RuleBuilderForm({ editRule, prefill, templateSeed, onSave, onCancel }: RuleBuilderFormProps) {
  const [mode, setMode] = useState<Mode>("structured");
  const [toolTarget, setToolTarget] = useState<ToolTarget>("name");
  const [name, setName] = useState("");
  const [toolName, setToolName] = useState("");
  const [toolCategory, setToolCategory] = useState("");
  const [toolPattern, setToolPattern] = useState("");
  const [programs, setPrograms] = useState<string[]>([]);
  const [subcommands, setSubcommands] = useState<string[]>([]);
  const [blockedSubcommands, setBlockedSubcommands] = useState<string[]>([]);
  const [requiredFlags, setRequiredFlags] = useState<string[]>([]);
  const [forbiddenFlags, setForbiddenFlags] = useState<string[]>([]);
  const [requiredFlagPrefixes, setRequiredFlagPrefixes] = useState<string[]>([]);
  const [pythonModes, setPythonModes] = useState<string[]>([]);
  const [safePythonImportsOnly, setSafePythonImportsOnly] = useState(false);
  const [commandPattern, setCommandPattern] = useState("");
  const [filePattern, setFilePattern] = useState("");
  const [decision, setDecision] = useState<AutoDecision>(AutoDecision.ESCALATE);
  const [riskLevel, setRiskLevel] = useState("medium");
  const [reason, setReason] = useState("");
  const [alternative, setAlternative] = useState("");
  const [priority, setPriority] = useState(450);
  const [enabled, setEnabled] = useState(true);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const showPythonSection = programs.some(isPythonProgram);
  const inlineChecked = pythonModes.includes("inline");

  // Seed from template
  useEffect(() => {
    if (!templateSeed) return;
    setName("");
    if (templateSeed.toolCategory) { setToolTarget("category"); setToolCategory(templateSeed.toolCategory); }
    else if (templateSeed.toolName) { setToolTarget("name"); setToolName(templateSeed.toolName); }
    else if (templateSeed.commandPattern) { setMode("regex"); setCommandPattern(templateSeed.commandPattern); }
    else setToolTarget("name");
    setPrograms(templateSeed.programs ?? []);
    setSubcommands(templateSeed.subcommands ?? []);
    setBlockedSubcommands(templateSeed.blockedSubcommands ?? []);
    setRequiredFlags(templateSeed.requiredFlags ?? []);
    setForbiddenFlags(templateSeed.forbiddenFlags ?? []);
    setPythonModes(templateSeed.pythonModes ?? []);
    setSafePythonImportsOnly(templateSeed.safePythonImportsOnly ?? false);
    setDecision(templateSeed.decision);
    setRiskLevel(templateSeed.riskLevel);
    setPriority(templateSeed.priority);
    setReason(templateSeed.reason ?? "");
    setAlternative(templateSeed.alternative ?? "");
    setEnabled(true);
  }, [templateSeed]);

  // Seed from URL prefill (analytics deep-link)
  useEffect(() => {
    if (!prefill) return;
    if (prefill.programs?.length) setPrograms(prefill.programs);
    if (prefill.subcommands?.length) setSubcommands(prefill.subcommands);
    if (prefill.toolName) { setToolTarget("name"); setToolName(prefill.toolName); }
    if (prefill.toolCategory) { setToolTarget("category"); setToolCategory(prefill.toolCategory); }
    if (prefill.suggestedDecision !== undefined) {
      setDecision(prefill.suggestedDecision as AutoDecision);
      setPriority(defaultPriority(prefill.suggestedDecision as AutoDecision));
    }
  }, [prefill]);

  // Seed from edit rule
  useEffect(() => {
    if (!editRule) return;
    const hasCriteria = (editRule.programs?.length ?? 0) > 0 ||
      (editRule.subcommands?.length ?? 0) > 0 ||
      (editRule.pythonModes?.length ?? 0) > 0;
    setMode(hasCriteria ? "structured" : (editRule.commandPattern ? "regex" : "structured"));
    setName(editRule.name);
    setToolName(editRule.toolName ?? "");
    setToolCategory(editRule.toolCategory ?? "");
    setToolPattern(editRule.toolPattern ?? "");
    if (editRule.toolCategory) setToolTarget("category");
    else if (editRule.toolPattern) setToolTarget("pattern");
    else setToolTarget("name");
    setPrograms(editRule.programs ?? []);
    setSubcommands(editRule.subcommands ?? []);
    setBlockedSubcommands(editRule.blockedSubcommands ?? []);
    setRequiredFlags(editRule.requiredFlags ?? []);
    setForbiddenFlags(editRule.forbiddenFlags ?? []);
    setRequiredFlagPrefixes(editRule.requiredFlagPrefixes ?? []);
    setPythonModes(editRule.pythonModes ?? []);
    setSafePythonImportsOnly(editRule.safePythonImportsOnly ?? false);
    setCommandPattern(editRule.commandPattern ?? "");
    setFilePattern(editRule.filePattern ?? "");
    setDecision(editRule.decision ?? AutoDecision.ESCALATE);
    setRiskLevel(editRule.riskLevel ?? "medium");
    setReason(editRule.reason ?? "");
    setAlternative(editRule.alternative ?? "");
    setPriority(editRule.priority ?? 10);
    setEnabled(editRule.enabled ?? true);
  }, [editRule]);

  function handleDecisionChange(d: AutoDecision) {
    if (d === decision) return;
    // Only auto-default priority when the value equals the previous decision's default,
    // so manually-edited priorities are not overwritten.
    if (priority === defaultPriority(decision)) {
      setPriority(defaultPriority(d));
    }
    setDecision(d);
  }

  function togglePythonMode(m: string) {
    setPythonModes((prev) =>
      prev.includes(m) ? prev.filter((x) => x !== m) : [...prev, m]
    );
  }

  function handleModeSwitch(next: Mode) {
    if (next === mode) return;
    if (next === "regex" && (programs.length > 0 || subcommands.length > 0)) {
      if (!window.confirm("Switch to Regex mode? Structured criteria will be cleared.")) return;
      setPrograms([]); setSubcommands([]); setBlockedSubcommands([]);
      setRequiredFlags([]); setForbiddenFlags([]); setRequiredFlagPrefixes([]);
      setPythonModes([]); setSafePythonImportsOnly(false);
    }
    if (next === "structured" && commandPattern) {
      if (!window.confirm("Switch to Structured mode? The regex pattern will be cleared.")) return;
      setCommandPattern("");
    }
    setMode(next);
  }

  async function handleSave() {
    if (!name.trim()) { setFormError("Name is required."); return; }

    const hasToolTarget = toolName || toolCategory || toolPattern;
    const hasCriteria = programs.length > 0 || subcommands.length > 0 ||
      requiredFlags.length > 0 || forbiddenFlags.length > 0 ||
      pythonModes.length > 0 || blockedSubcommands.length > 0;
    const hasPattern = commandPattern || filePattern;

    if (!hasToolTarget && !hasCriteria && !hasPattern) {
      setFormError("At least one match criterion is required (tool, program, or pattern).");
      return;
    }

    setFormError(null);
    setSaving(true);
    try {
      const id = editRule?.id ?? `user-${Date.now()}`;
      await onSave({
        id,
        name: name.trim(),
        toolName: toolTarget === "name" ? toolName : "",
        toolCategory: toolTarget === "category" ? toolCategory : "",
        toolPattern: toolTarget === "pattern" ? toolPattern : "",
        commandPattern: mode === "regex" ? commandPattern : "",
        filePattern,
        decision,
        riskLevel,
        reason,
        alternative,
        priority,
        enabled,
        programs: mode === "structured" ? programs : [],
        subcommands: mode === "structured" ? subcommands : [],
        blockedSubcommands: mode === "structured" ? blockedSubcommands : [],
        requiredFlags: mode === "structured" ? requiredFlags : [],
        forbiddenFlags: mode === "structured" ? forbiddenFlags : [],
        requiredFlagPrefixes: mode === "structured" ? requiredFlagPrefixes : [],
        pythonModes: mode === "structured" ? pythonModes : [],
        safePythonImportsOnly: mode === "structured" ? safePythonImportsOnly : false,
      });
    } catch (e) {
      setFormError(e instanceof Error ? e.message : "Failed to save rule.");
    } finally {
      setSaving(false);
    }
  }

  const previewCriteria: RuleCriteria = {
    programs, subcommands, blockedSubcommands,
    requiredFlags, forbiddenFlags, requiredFlagPrefixes, pythonModes,
  };

  const isPrefilled = !editRule && (
    (prefill?.programs?.length ?? 0) > 0 ||
    (prefill?.subcommands?.length ?? 0) > 0 ||
    !!prefill?.toolName || !!prefill?.toolCategory
  );

  return (
    <div className={formWrapper}>
      <h3 className={formTitle}>
        {editRule ? `Editing: ${editRule.name}` : "New Rule"}
      </h3>

      {/* Mode toggle */}
      <div className={modeToggle}>
        <button className={`${modeBtn} ${mode === "structured" ? modeBtnActive : ""}`} onClick={() => handleModeSwitch("structured")}>
          Structured
        </button>
        <button className={`${modeBtn} ${mode === "regex" ? modeBtnActive : ""}`} onClick={() => handleModeSwitch("regex")}>
          Regex / Advanced
        </button>
      </div>

      {formError && <div className={errorBanner}>{formError}</div>}

      {/* Tool target */}
      <div className={section}>
        <p className={sectionTitle}>Tool</p>
        <div className={radioGroup}>
          {(["name", "category", "pattern"] as ToolTarget[]).map((t) => (
            <label key={t} className={radioRow}>
              <input type="radio" name="toolTarget" value={t} checked={toolTarget === t} onChange={() => setToolTarget(t)} />
              {t === "name" ? "Exact tool name" : t === "category" ? "Tool category" : "Tool pattern (regex)"}
            </label>
          ))}
        </div>
        {toolTarget === "name" && (
          <input className={fieldInput} value={toolName} onChange={(e) => setToolName(e.target.value)} placeholder='e.g. Bash, Read, Edit' />
        )}
        {toolTarget === "category" && (
          <select className={fieldSelect} value={toolCategory} onChange={(e) => setToolCategory(e.target.value)}>
            {TOOL_CATEGORIES.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
          </select>
        )}
        {toolTarget === "pattern" && (
          <input className={fieldInput} value={toolPattern} onChange={(e) => setToolPattern(e.target.value)} placeholder='e.g. Read|Glob|Grep' />
        )}
      </div>

      {/* Structured criteria */}
      {mode === "structured" && (
        <div className={section}>
          <p className={sectionTitle}>Command Criteria</p>
          <div className={formGrid}>
            <label className={fieldLabel}>
              Programs
              <TagInput value={programs} onChange={setPrograms} placeholder="e.g. git, python3" isPrefilled={isPrefilled} />
            </label>
            <label className={fieldLabel}>
              Subcommands (allow-list)
              <TagInput value={subcommands} onChange={setSubcommands} placeholder="e.g. push, pull" isPrefilled={isPrefilled} />
            </label>
            <label className={fieldLabel}>
              Blocked subcommands
              <TagInput value={blockedSubcommands} onChange={setBlockedSubcommands} placeholder="e.g. reset, rebase" />
            </label>
            <label className={fieldLabel}>
              Required flags
              <TagInput value={requiredFlags} onChange={setRequiredFlags} placeholder="e.g. --hard, -f" />
            </label>
            <label className={fieldLabel}>
              Forbidden flags
              <TagInput value={forbiddenFlags} onChange={setForbiddenFlags} placeholder="e.g. --force, --no-verify" />
            </label>
          </div>

          {showAdvanced && (
            <div className={formGrid}>
              <label className={fieldLabel}>
                Required flag prefixes
                <TagInput value={requiredFlagPrefixes} onChange={setRequiredFlagPrefixes} placeholder="e.g. -i (matches -i, -i.bak)" />
              </label>
            </div>
          )}
          <button className={advancedToggle} type="button" onClick={() => setShowAdvanced((v) => !v)}>
            {showAdvanced ? "▲ Hide advanced" : "▼ Advanced options"}
          </button>

          {/* Python mode section */}
          {showPythonSection && (
            <div className={pythonSection}>
              <p className={pythonSectionTitle}>Python invocation mode</p>
              <div className={checkboxGrid}>
                {PYTHON_MODES.map((m) => (
                  <label key={m.value} className={checkboxRow}>
                    <input type="checkbox" checked={pythonModes.includes(m.value)} onChange={() => togglePythonMode(m.value)} />
                    {m.label}
                  </label>
                ))}
              </div>
              {inlineChecked && (
                <label className={checkboxRow} style={{ marginTop: "8px" }}>
                  <input type="checkbox" checked={safePythonImportsOnly} onChange={(e) => setSafePythonImportsOnly(e.target.checked)} />
                  Require safe stdlib imports only (json, os, sys, re…)
                </label>
              )}
            </div>
          )}

          <RulePreview criteria={previewCriteria} showSafePythonNotice={safePythonImportsOnly} />
        </div>
      )}

      {/* Regex mode */}
      {mode === "regex" && (
        <div className={section}>
          <p className={sectionTitle}>Regex patterns</p>
          <div className={formGrid}>
            <label className={fieldLabel}>
              Command pattern
              <input className={fieldInput} value={commandPattern} onChange={(e) => setCommandPattern(e.target.value)} placeholder="e.g. ^git push" />
            </label>
            <label className={fieldLabel}>
              File pattern
              <input className={fieldInput} value={filePattern} onChange={(e) => setFilePattern(e.target.value)} placeholder="e.g. \.env$" />
            </label>
          </div>
        </div>
      )}

      {/* Decision */}
      <div className={section}>
        <p className={sectionTitle}>Decision</p>
        <div className={decisionSegment}>
          {([
            [AutoDecision.ALLOW, "Auto-Allow", decisionAllow],
            [AutoDecision.DENY, "Auto-Deny", decisionDeny],
            [AutoDecision.ESCALATE, "Escalate", decisionEscalate],
          ] as [AutoDecision, string, string][]).map(([d, label, cls]) => (
            <button key={d} type="button" className={`${decisionBtn} ${decision === d ? cls : ""}`} onClick={() => handleDecisionChange(d)}>
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* Metadata */}
      <div className={section}>
        <p className={sectionTitle}>Details</p>
        <div className={formGrid}>
          <label className={`${fieldLabel} ${formGridFull}`}>
            Name *
            <input className={fieldInput} value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Allow git log" />
          </label>
          <label className={fieldLabel}>
            Risk level
            <select className={fieldSelect} value={riskLevel} onChange={(e) => setRiskLevel(e.target.value)}>
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
              <option value="critical">Critical</option>
            </select>
          </label>
          <label className={fieldLabel}>
            Priority
            <input className={fieldInput} type="number" min={1} max={9999} value={priority} onChange={(e) => setPriority(Number(e.target.value))} />
            <span className={priorityHint}>1000+ deny first · 500+ escalate before allow · 100–499 allow tier · 1–99 after built-ins</span>
            {priority < 100 && decision === AutoDecision.DENY && (
              <span className={priorityWarning}>Warning: priority below 100 means seed allow rules (at 100) will fire first.</span>
            )}
          </label>
          <label className={`${fieldLabel} ${formGridFull}`}>
            Reason (shown to Claude when denied/escalated)
            <input className={fieldInput} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="e.g. This operation modifies remote state." />
          </label>
          <label className={`${fieldLabel} ${formGridFull}`}>
            Alternative suggestion
            <input className={fieldInput} value={alternative} onChange={(e) => setAlternative(e.target.value)} placeholder="e.g. Use the Edit tool instead." />
          </label>
          <label className={checkboxRow}>
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
            Rule enabled
          </label>
        </div>
      </div>

      <div className={actions}>
        <button className={saveBtn} onClick={handleSave} disabled={saving}>
          {saving ? "Saving…" : editRule ? "Update Rule" : "Save Rule"}
        </button>
        <button className={cancelBtn} onClick={onCancel}>Cancel</button>
      </div>
    </div>
  );
}
