import { ApprovalRuleProto } from "@/gen/session/v1/types_pb";

export interface RuleDescription {
  primary: string;
  secondary?: string;
  isStructured: boolean;
  isRegex: boolean;
}

const TOOL_CATEGORY_LABELS: Record<string, string> = {
  "builtin": "built-in Claude Code",
  "builtin-agent": "agent/planning built-in",
  "mcp": "MCP",
  "mcp-read": "read-only MCP",
  "mcp-write": "write MCP",
};

function toolCategoryLabel(cat: string): string {
  return TOOL_CATEGORY_LABELS[cat] ?? cat;
}

function pythonModeDescription(modes: string[]): string {
  const labels: Record<string, string> = {
    script: "a .py script",
    module: "-m module",
    inline: "inline -c code",
    version: "-V version check",
  };
  const mapped = modes.map((m) => labels[m] ?? m);
  if (mapped.length === 0) return "any invocation";
  if (mapped.length === 1) return mapped[0];
  if (mapped.length === 2) return `${mapped[0]} or ${mapped[1]}`;
  return `${mapped.slice(0, -1).join(", ")}, or ${mapped[mapped.length - 1]}`;
}

export function describeRule(rule: ApprovalRuleProto): RuleDescription {
  const programs = rule.programs ?? [];
  const subcommands = rule.subcommands ?? [];
  const blocked = rule.blockedSubcommands ?? [];
  const requiredFlags = rule.requiredFlags ?? [];
  const forbiddenFlags = rule.forbiddenFlags ?? [];
  const pythonModes = rule.pythonModes ?? [];

  const hasStructured =
    programs.length > 0 ||
    subcommands.length > 0 ||
    blocked.length > 0 ||
    requiredFlags.length > 0 ||
    forbiddenFlags.length > 0 ||
    pythonModes.length > 0 ||
    rule.safePythonImportsOnly;

  if (hasStructured) {
    const progStr = programs.join("/") || "any program";
    let primary: string;

    if (pythonModes.length > 0) {
      primary = `${progStr} running ${pythonModeDescription(pythonModes)}`;
      if (rule.safePythonImportsOnly) {
        primary += " (stdlib imports only)";
      }
    } else if (subcommands.length > 0) {
      const subStr = subcommands.join(", ");
      const flagStr = requiredFlags.length > 0 ? ` with ${requiredFlags.join(" or ")}` : "";
      primary = `${progStr} ${subStr}${flagStr}`;
    } else if (requiredFlags.length > 0) {
      primary = `${progStr} with ${requiredFlags.join(" or ")}`;
    } else {
      primary = `${progStr} (any subcommand)`;
    }

    let secondary: string | undefined;
    const parts: string[] = [];
    if (blocked.length > 0) parts.push(`blocked: ${blocked.join(", ")}`);
    if (forbiddenFlags.length > 0) parts.push(`not: ${forbiddenFlags.join(", ")}`);
    if (parts.length > 0) secondary = parts.join(" · ");

    return { primary, secondary, isStructured: true, isRegex: false };
  }

  if (rule.toolName) {
    return { primary: `Tool: ${rule.toolName}`, isStructured: false, isRegex: false };
  }
  if (rule.toolCategory) {
    return { primary: `Any ${toolCategoryLabel(rule.toolCategory)} tool`, isStructured: false, isRegex: false };
  }
  if (rule.toolPattern) {
    return { primary: `Tools matching: ${rule.toolPattern}`, isStructured: false, isRegex: true };
  }
  if (rule.commandPattern) {
    return { primary: `Pattern: ${rule.commandPattern}`, isStructured: false, isRegex: true };
  }
  if (rule.filePattern) {
    return { primary: `File pattern: ${rule.filePattern}`, isStructured: false, isRegex: true };
  }

  return { primary: "(no match criteria)", isStructured: false, isRegex: false };
}
