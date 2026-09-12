"use client";

import { useState } from "react";
import { useTaggingRules } from "@/lib/hooks/useTaggingRules";
import { TaggingRuleProto } from "@/gen/session/v1/types_pb";
import { TaggingRuleBuilderForm } from "@/components/rules/TaggingRuleBuilderForm";
import {
  loading as loadingClass, empty, tableWrapper, table, th, td, tdCenter, row, rowDisabled,
  ruleName, sourceBadge, toggle, toggleOn, toggleOff, deleteButton, builtInBadge,
  addButton, formSection, hitBadge, hitBadgeActive, rowCount,
} from "./ApprovalRulesPanel.css";

/** Describes which single field a rule matches against, for display purposes. */
function matchDescription(rule: TaggingRuleProto): string {
  if (rule.namePattern) return `name: ${rule.namePattern}`;
  if (rule.branchPattern) return `branch: ${rule.branchPattern}`;
  if (rule.pathPattern) return `path: ${rule.pathPattern}`;
  if (rule.programPattern) return `program: ${rule.programPattern}`;
  return "—";
}

/**
 * TaggingRulesPanel — the "Tagging Rules" tab's content (ux.md Surface 4).
 * A sibling to ApprovalRulesPanel rather than an extension of it: tagging
 * rules have a different shape (single match-field pattern, output tag) than
 * approval rules (decision/risk/tool-target), so sharing one table/form would
 * require branching most of its rendering logic. Table styling is reused
 * verbatim from ApprovalRulesPanel.css for visual parity, per Task 6.1.1a.
 */
export function TaggingRulesPanel() {
  const { rules, loading, upsertRule, deleteRule } = useTaggingRules();
  const [showBuilder, setShowBuilder] = useState(false);
  const [editingRule, setEditingRule] = useState<TaggingRuleProto | null>(null);

  const handleSave = async (rule: Partial<TaggingRuleProto> & { id: string }) => {
    await upsertRule(rule);
    setShowBuilder(false);
    setEditingRule(null);
  };

  const handleCancel = () => {
    setShowBuilder(false);
    setEditingRule(null);
  };

  const handleEdit = (rule: TaggingRuleProto) => {
    setEditingRule(rule);
    setShowBuilder(true);
  };

  const handleToggle = async (rule: TaggingRuleProto) => {
    if (rule.source !== "user") return;
    await upsertRule({
      id: rule.id,
      name: rule.name,
      namePattern: rule.namePattern,
      branchPattern: rule.branchPattern,
      pathPattern: rule.pathPattern,
      programPattern: rule.programPattern,
      requiredTags: rule.requiredTags,
      outputTag: rule.outputTag,
      priority: rule.priority,
      enabled: !rule.enabled,
    });
  };

  return (
    <div data-testid="tagging-rules-panel">
      <div className={tableWrapper}>
        {loading && rules.length === 0 ? (
          <div className={loadingClass}>Loading tagging rules…</div>
        ) : rules.length === 0 ? (
          <div className={empty} data-testid="tagging-rules-empty-state">
            <p>No tagging rules configured. Sessions will rely on LLM classification only.</p>
          </div>
        ) : (
          <table className={table}>
            <thead>
              <tr>
                <th className={th}>Name</th>
                <th className={th}>Match</th>
                <th className={th}>Output Tag</th>
                <th className={th}>Priority</th>
                <th
                  className={th}
                  data-testid="tagging-rule-fire-count-header"
                  title="Number of times this rule fired in the last 7 days"
                >
                  Fires (7d)
                </th>
                <th className={th}>Enabled</th>
                <th className={th}></th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr key={rule.id} className={`${row} ${!rule.enabled ? rowDisabled : ""}`}>
                  <td className={td}>
                    <span className={ruleName}>{rule.name || rule.id}</span>
                    {rule.source === "seed" && (
                      <span className={sourceBadge} title="Built-in">ⓘ Built-in</span>
                    )}
                  </td>
                  <td className={td}>{matchDescription(rule)}</td>
                  <td className={td}>{rule.outputTag}</td>
                  <td className={`${td} ${tdCenter}`}>{rule.priority}</td>
                  <td className={`${td} ${tdCenter}`}>
                    {rule.fireCount7d > 0
                      ? <span className={`${hitBadge} ${hitBadgeActive}`}>{rule.fireCount7d.toLocaleString()}</span>
                      : <span className={hitBadge}>—</span>}
                  </td>
                  <td className={`${td} ${tdCenter}`}>
                    {rule.source === "user" ? (
                      <button
                        className={`${toggle} ${rule.enabled ? toggleOn : toggleOff}`}
                        onClick={() => handleToggle(rule)}
                        aria-label={`${rule.enabled ? "Disable" : "Enable"} tagging rule`}
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
                          aria-label={`Edit tagging rule ${rule.name}`}
                        >
                          Edit
                        </button>
                        <button
                          className={deleteButton}
                          onClick={() => deleteRule(rule.id)}
                          aria-label={`Delete tagging rule ${rule.name}`}
                          title="Delete tagging rule"
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

      {rules.length > 0 && (
        <div className={rowCount}>
          {rules.length} tagging rule{rules.length !== 1 ? "s" : ""}
        </div>
      )}

      <div className={formSection} id="tagging-rule-builder">
        {!showBuilder ? (
          <button
            data-testid="add-tagging-rule-button"
            className={addButton}
            onClick={() => { setEditingRule(null); setShowBuilder(true); }}
          >
            + Add Tagging Rule
          </button>
        ) : (
          <TaggingRuleBuilderForm editRule={editingRule} onSave={handleSave} onCancel={handleCancel} />
        )}
      </div>
    </div>
  );
}
