"use client";

import { useEffect, useState } from "react";
import { TaggingRuleProto } from "@/gen/session/v1/types_pb";
import { TagInput } from "./TagInput";
import {
  formWrapper, formTitle, section, sectionTitle, formGrid, formGridFull,
  fieldLabel, fieldInput, fieldSelect, checkboxRow, errorBanner, actions, saveBtn, cancelBtn,
} from "./RuleBuilderForm.css";
import { fieldError, invalidInput } from "./TaggingRuleBuilderForm.css";

export const PATTERN_ERROR_ID = "tagging-rule-pattern-error";

type MatchField = "name" | "branch" | "path" | "program";

const MATCH_FIELDS: { value: MatchField; label: string }[] = [
  { value: "name", label: "Name" },
  { value: "branch", label: "Branch" },
  { value: "path", label: "Path" },
  { value: "program", label: "Program" },
];

interface TaggingRuleBuilderFormProps {
  editRule?: TaggingRuleProto | null;
  onSave: (rule: Partial<TaggingRuleProto> & { id: string }) => Promise<void>;
  onCancel: () => void;
}

/** Validates a regex pattern, returning a human-readable error message or null if valid. */
function validatePattern(pattern: string): string | null {
  if (!pattern) return null;
  try {
    // eslint-disable-next-line no-new
    new RegExp(pattern);
    return null;
  } catch (e) {
    return e instanceof Error ? e.message : "Invalid regex pattern";
  }
}

/** Reads whichever pattern field applies to `matchField` from an existing rule. */
function patternForField(rule: TaggingRuleProto, field: MatchField): string {
  switch (field) {
    case "name": return rule.namePattern;
    case "branch": return rule.branchPattern;
    case "path": return rule.pathPattern;
    case "program": return rule.programPattern;
  }
}

/**
 * TaggingRuleBuilderForm — a new form built for the tagging-rule CRUD tab
 * (ux.md Surface 4/5), scoped to single-field matching (Task 6.1.1b: one
 * "match against" selector + one pattern field, not all four ANDed patterns
 * at once — multi-field rules remain editable via the API directly).
 *
 * Deliberately a separate component from RuleBuilderForm.tsx rather than an
 * extension of it: RuleBuilderForm is tightly coupled to ApprovalRuleProto's
 * shape (decision/risk/tool-target fields that don't apply here). It reuses
 * RuleBuilderForm.css's field-row classes for visual consistency instead.
 */
export function TaggingRuleBuilderForm({ editRule, onSave, onCancel }: TaggingRuleBuilderFormProps) {
  const [name, setName] = useState("");
  const [matchField, setMatchField] = useState<MatchField>("branch");
  const [pattern, setPattern] = useState("");
  const [requiredTags, setRequiredTags] = useState<string[]>([]);
  const [outputTag, setOutputTag] = useState("");
  const [priority, setPriority] = useState(50);
  const [enabled, setEnabled] = useState(true);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [patternError, setPatternError] = useState<string | null>(null);
  const [patternTouched, setPatternTouched] = useState(false);

  useEffect(() => {
    if (!editRule) return;
    setName(editRule.name);
    const field = (["name", "branch", "path", "program"] as MatchField[]).find(
      (f) => patternForField(editRule, f)
    ) ?? "branch";
    setMatchField(field);
    setPattern(patternForField(editRule, field));
    setRequiredTags(editRule.requiredTags ?? []);
    setOutputTag(editRule.outputTag ?? "");
    setPriority(editRule.priority ?? 50);
    setEnabled(editRule.enabled ?? true);
  }, [editRule]);

  function runPatternValidation(value: string): boolean {
    const err = validatePattern(value);
    setPatternError(err);
    return err === null;
  }

  function handlePatternChange(value: string) {
    setPattern(value);
    if (patternTouched) runPatternValidation(value);
  }

  function handlePatternBlur() {
    setPatternTouched(true);
    runPatternValidation(pattern);
  }

  async function handleSave() {
    if (!name.trim()) { setFormError("Name is required."); return; }
    if (!outputTag.trim()) { setFormError("Output tag is required."); return; }
    setPatternTouched(true);
    if (!runPatternValidation(pattern)) {
      setFormError(null);
      document.getElementById("tagging-rule-pattern-input")?.focus();
      return;
    }

    setFormError(null);
    setSaving(true);
    try {
      const rulePayload = {
        name: name.trim(),
        namePattern: matchField === "name" ? pattern : "",
        branchPattern: matchField === "branch" ? pattern : "",
        pathPattern: matchField === "path" ? pattern : "",
        programPattern: matchField === "program" ? pattern : "",
        requiredTags,
        outputTag: outputTag.trim(),
        priority,
        enabled,
      };
      const id = editRule?.id ?? `user-${Date.now()}`;
      await onSave({ id, ...rulePayload });
    } catch (e) {
      setFormError(e instanceof Error ? e.message : "Failed to save rule.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className={formWrapper}>
      <h3 className={formTitle}>
        {editRule ? `Editing: ${editRule.name}` : "New Tagging Rule"}
      </h3>

      {formError && <div className={errorBanner} role="alert">{formError}</div>}

      <div className={section}>
        <p className={sectionTitle}>Match</p>
        <div className={formGrid}>
          <label className={fieldLabel} htmlFor="tagging-rule-match-field-select">
            Match against
            <select
              id="tagging-rule-match-field-select"
              className={fieldSelect}
              data-testid="tagging-rule-match-field-select"
              value={matchField}
              onChange={(e) => setMatchField(e.target.value as MatchField)}
            >
              {MATCH_FIELDS.map((f) => (
                <option key={f.value} value={f.value}>{f.label}</option>
              ))}
            </select>
          </label>
          <label className={fieldLabel} htmlFor="tagging-rule-pattern-input">
            Pattern (regex)
            <input
              id="tagging-rule-pattern-input"
              className={`${fieldInput} ${patternError ? invalidInput : ""}`}
              data-testid="tagging-rule-pattern-input"
              value={pattern}
              onChange={(e) => handlePatternChange(e.target.value)}
              onBlur={handlePatternBlur}
              aria-invalid={patternError ? "true" : undefined}
              aria-describedby={patternError ? PATTERN_ERROR_ID : undefined}
              placeholder={matchField === "branch" ? "e.g. ^(bugfix|fix)/" : "e.g. ^claude$"}
            />
            {patternError && (
              <p id={PATTERN_ERROR_ID} className={fieldError} data-testid="tagging-rule-pattern-error">
                Invalid regex: {patternError}
              </p>
            )}
          </label>
        </div>
      </div>

      <div className={section}>
        <p className={sectionTitle}>Output</p>
        <div className={formGrid}>
          <label className={fieldLabel}>
            Requires tags (optional)
            <TagInput value={requiredTags} onChange={setRequiredTags} placeholder="e.g. Bugfix" />
          </label>
          <label className={fieldLabel} htmlFor="tagging-rule-output-tag-input">
            Output tag
            <input
              id="tagging-rule-output-tag-input"
              className={fieldInput}
              data-testid="tagging-rule-output-tag-input"
              value={outputTag}
              onChange={(e) => setOutputTag(e.target.value)}
              placeholder="e.g. Bugfix"
            />
          </label>
        </div>
      </div>

      <div className={section}>
        <p className={sectionTitle}>Details</p>
        <div className={formGrid}>
          <label className={`${fieldLabel} ${formGridFull}`}>
            Name *
            <input
              className={fieldInput}
              data-testid="tagging-rule-name-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Bugfix branch"
            />
          </label>
          <label className={fieldLabel}>
            Priority
            <input
              className={fieldInput}
              type="number"
              min={1}
              max={9999}
              value={priority}
              onChange={(e) => setPriority(Number(e.target.value))}
            />
          </label>
          <label className={checkboxRow}>
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
            Rule enabled
          </label>
        </div>
      </div>

      <div className={actions}>
        <button className={saveBtn} onClick={handleSave} disabled={saving}>
          {saving ? "Saving…" : "Save Rule"}
        </button>
        <button className={cancelBtn} onClick={onCancel}>Cancel</button>
      </div>
    </div>
  );
}
