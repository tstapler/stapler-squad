import { AutoDecision } from "@/gen/session/v1/types_pb";

export interface RuleTemplate {
  id: string;
  title: string;
  description: string;
  icon: string;
  decision: AutoDecision;
  riskLevel: string;
  priority: number;
  programs?: string[];
  subcommands?: string[];
  blockedSubcommands?: string[];
  requiredFlags?: string[];
  forbiddenFlags?: string[];
  pythonModes?: string[];
  safePythonImportsOnly?: boolean;
  toolName?: string;
  toolCategory?: string;
  commandPattern?: string;
  reason?: string;
  alternative?: string;
}

export const RULE_TEMPLATES: RuleTemplate[] = [
  {
    id: "python-script-module",
    title: "Python script/module",
    description: "Allow python3 running a .py file or -m module. Inline -c still escalates.",
    icon: "🐍",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    programs: ["python", "python3", "pypy", "pypy3"],
    pythonModes: ["script", "module", "version"],
    reason: "Python running a project script or module.",
  },
  {
    id: "python-inline-stdlib",
    title: "Python inline — stdlib only",
    description: "Allow python -c when all imports are from the safe stdlib (json, os, sys…).",
    icon: "🐍",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    programs: ["python", "python3", "pypy", "pypy3"],
    pythonModes: ["inline"],
    safePythonImportsOnly: true,
    reason: "Inline Python using only safe stdlib modules.",
  },
  {
    id: "python-inline-any",
    title: "Python inline — any imports",
    description: "Escalate python -c with any imports for manual review.",
    icon: "🐍",
    decision: AutoDecision.ESCALATE,
    riskLevel: "medium",
    priority: 50,
    programs: ["python", "python3", "pypy", "pypy3"],
    pythonModes: ["inline"],
    reason: "Inline Python with non-stdlib imports can make network calls or run arbitrary code.",
    alternative: "Write the code to a .py file and run it with python script.py instead.",
  },
  {
    id: "git-read",
    title: "Git read-only",
    description: "Allow read-only git commands: log, status, diff, show, branch, etc.",
    icon: "📖",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    programs: ["git"],
    subcommands: ["log", "status", "diff", "show", "branch", "remote", "fetch", "tag", "describe", "ls-files"],
    reason: "Read-only git operations pose no risk.",
  },
  {
    id: "git-push",
    title: "Git push",
    description: "Escalate all git push operations for manual review.",
    icon: "🚀",
    decision: AutoDecision.ESCALATE,
    riskLevel: "high",
    priority: 50,
    programs: ["git"],
    subcommands: ["push"],
    reason: "git push modifies remote state and should be reviewed.",
  },
  {
    id: "git-reset-hard",
    title: "Git reset --hard",
    description: "Deny git reset --hard — discards uncommitted changes irreversibly.",
    icon: "🚫",
    decision: AutoDecision.DENY,
    riskLevel: "high",
    priority: 1000,
    programs: ["git"],
    subcommands: ["reset"],
    requiredFlags: ["--hard"],
    reason: "git reset --hard discards uncommitted changes and cannot be undone.",
    alternative: "Use git stash to save changes, or git reset HEAD~1 to keep changes staged.",
  },
  {
    id: "npm-install",
    title: "npm / yarn / pnpm install",
    description: "Allow standard package install operations.",
    icon: "📦",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    programs: ["npm", "npx", "yarn", "pnpm"],
    reason: "Node.js package management and script execution.",
  },
  {
    id: "npm-publish",
    title: "npm publish",
    description: "Escalate npm publish and credential operations for review.",
    icon: "📤",
    decision: AutoDecision.ESCALATE,
    riskLevel: "high",
    priority: 500,
    programs: ["npm"],
    subcommands: ["publish", "adduser", "login", "logout"],
    reason: "npm publish/credential operations affect the public registry and should be reviewed.",
  },
  {
    id: "docker-read",
    title: "Docker read-only",
    description: "Allow read-only Docker inspection: ps, logs, inspect, images.",
    icon: "🐳",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    programs: ["docker"],
    subcommands: ["ps", "images", "logs", "inspect", "info", "version", "stats", "top"],
    reason: "Read-only Docker inspection commands.",
  },
  {
    id: "docker-write",
    title: "Docker write operations",
    description: "Escalate docker run, exec, build, rm and other mutating commands.",
    icon: "🐳",
    decision: AutoDecision.ESCALATE,
    riskLevel: "medium",
    priority: 50,
    programs: ["docker"],
    subcommands: ["run", "exec", "build", "push", "pull", "rm", "stop", "start", "restart", "compose"],
    reason: "Docker operations that create, modify, or remove containers should be reviewed.",
  },
  {
    id: "mcp-read-tools",
    title: "MCP read-only tools",
    description: "Allow all read-only MCP tools (context7, filesystem reads, etc.).",
    icon: "🔌",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    toolCategory: "mcp-read",
    reason: "Read-only MCP tools pose no risk.",
  },
  {
    id: "file-editing-tools",
    title: "File editing tools",
    description: "Allow Claude Code's core file editing tools (Edit, Write, MultiEdit).",
    icon: "✏️",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    priority: 100,
    toolCategory: "builtin",
    reason: "Core Claude Code file editing tools.",
  },
  {
    id: "escalate-unknown",
    title: "Escalate unknown program",
    description: "Catch-all: escalate any Bash command not matched by other rules.",
    icon: "❓",
    decision: AutoDecision.ESCALATE,
    riskLevel: "medium",
    priority: 1,
    toolName: "Bash",
    commandPattern: ".*",
    reason: "No specific rule matched this program. Review before allowing.",
  },
];
