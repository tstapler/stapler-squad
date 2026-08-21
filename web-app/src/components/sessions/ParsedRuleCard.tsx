"use client";

import { type ParsedRuleResult } from "@/gen/session/v1/session_pb";
import { AutoDecision } from "@/gen/session/v1/types_pb";
import {
  ruleCard,
  ruleCardHeader,
  ruleCardName,
  decisionBadge,
  statusBadge,
  matchChips,
  matchChip,
  errorList,
  errorListItem,
} from "./ImportRulesModal.css";

// +feature: approval-rules:yaml-import

export type ParsedRuleCardStatus = "valid" | "error" | "overwrite" | "skip";

interface ParsedRuleCardProps {
  result: ParsedRuleResult;
  status: ParsedRuleCardStatus;
}

function decisionLabel(d: AutoDecision): string {
  switch (d) {
    case AutoDecision.ALLOW: return "Allow";
    case AutoDecision.DENY:  return "Deny";
    default:                 return "Escalate";
  }
}

function decisionVariant(d: AutoDecision): "allow" | "deny" | "escalate" | "unknown" {
  switch (d) {
    case AutoDecision.ALLOW: return "allow";
    case AutoDecision.DENY:  return "deny";
    case AutoDecision.ESCALATE: return "escalate";
    default: return "unknown";
  }
}

export function ParsedRuleCard({ result, status }: ParsedRuleCardProps) {
  const rule = result.rule;

  return (
    <div
      className={ruleCard({ status })}
      data-testid={`parsed-rule-card-${status}`}
    >
      <div className={ruleCardHeader}>
        <span className={ruleCardName}>{result.originalName || rule?.name || "(unnamed)"}</span>
        {rule && (
          <span className={decisionBadge({ decision: decisionVariant(rule.decision) })}>
            {decisionLabel(rule.decision)}
          </span>
        )}
        {status === "overwrite" && (
          <span className={statusBadge({ type: "overwrite" })} data-testid="overwrite-badge">
            will overwrite
          </span>
        )}
        {status === "skip" && (
          <span className={statusBadge({ type: "skip" })} data-testid="skip-badge">
            will skip
          </span>
        )}
      </div>

      {rule && (rule.toolName || (rule.programs && rule.programs.length > 0) || rule.toolPattern || rule.commandPattern || rule.filePattern) && (
        <div className={matchChips}>
          {rule.toolName && <code className={matchChip}>{rule.toolName}</code>}
          {rule.toolPattern && <code className={matchChip}>pattern: {rule.toolPattern}</code>}
          {rule.programs && rule.programs.length > 0 && (
            <code className={matchChip}>programs: {rule.programs.join(", ")}</code>
          )}
          {rule.subcommands && rule.subcommands.length > 0 && (
            <code className={matchChip}>sub: {rule.subcommands.join(", ")}</code>
          )}
          {rule.commandPattern && <code className={matchChip}>cmd: {rule.commandPattern}</code>}
          {rule.filePattern && <code className={matchChip}>file: {rule.filePattern}</code>}
        </div>
      )}

      {status === "error" && result.errors.length > 0 && (
        <ul className={errorList} data-testid="error-list">
          {result.errors.map((err, i) => (
            <li key={i} className={errorListItem}>{err}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
