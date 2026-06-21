/**
 * Input type detection types for the omnibar
 * Ported from Go implementation at ui/overlay/omnibar/types.go
 */

export enum InputType {
  Unknown = "unknown",
  Command = "command",
  LocalPath = "local_path",
  PathWithBranch = "path_with_branch",
  GitHubPR = "github_pr",
  GitHubBranch = "github_branch",
  GitHubRepo = "github_repo",
  GitHubShorthand = "github_shorthand",
  SessionSearch = "session_search",
  NewSession = "new_session",
  SpawnShell = "spawn_shell",
  Workflow = "workflow",
  Alias = "alias",
  AliasNotFound = "alias_not_found",
  AliasBrowse = "alias_browse",
}

export interface InputTypeInfo {
  label: string;
  icon: string;
  description: string;
}

export const INPUT_TYPE_INFO: Record<InputType, InputTypeInfo> = {
  [InputType.Unknown]: {
    label: "Unknown",
    icon: "❓",
    description: "Enter a local path or GitHub URL",
  },
  [InputType.Command]: {
    label: "Command",
    icon: "⌨️",
    description: "Run an omnibar command",
  },
  [InputType.LocalPath]: {
    label: "Local Path",
    icon: "📁",
    description: "Local filesystem path",
  },
  [InputType.PathWithBranch]: {
    label: "Path + Branch",
    icon: "🌿",
    description: "Local path with branch (path@branch)",
  },
  [InputType.GitHubPR]: {
    label: "GitHub PR",
    icon: "🔀",
    description: "Pull request URL",
  },
  [InputType.GitHubBranch]: {
    label: "GitHub Branch",
    icon: "🌱",
    description: "Branch URL",
  },
  [InputType.GitHubRepo]: {
    label: "GitHub Repo",
    icon: "📦",
    description: "Repository URL",
  },
  [InputType.GitHubShorthand]: {
    label: "GitHub",
    icon: "📦",
    description: "owner/repo shorthand",
  },
  [InputType.SessionSearch]: {
    label: "Search Sessions",
    icon: "🔍",
    description: "Search existing sessions",
  },
  [InputType.NewSession]: {
    label: "New Session",
    icon: "✨",
    description: "Create a new session",
  },
  [InputType.SpawnShell]: {
    label: "Spawn Shell",
    icon: ">_",
    description: "Open the new shell dialog",
  },
  [InputType.Workflow]: {
    label: "Workflow",
    icon: "⚡",
    description: "Quick workflow invocation",
  },
  [InputType.Alias]: {
    label: "Alias",
    icon: "@",
    description: "Named session preset",
  },
  [InputType.AliasNotFound]: {
    label: "Alias Not Found",
    icon: "@",
    description: "No alias with that name",
  },
  [InputType.AliasBrowse]: {
    label: "Alias Browse",
    icon: "@",
    description: "Browse or filter aliases",
  },
};

export interface GitHubRef {
  owner: string;
  repo: string;
  branch?: string;
  prNumber?: number;
}

export interface DetectionResult {
  type: InputType;
  confidence: number;
  parsedValue: string;
  suggestedName: string;
  localPath?: string;
  branch?: string;
  gitHubRef?: GitHubRef;
  metadata?: Record<string, unknown>;
}

export interface ValidationResult {
  valid: boolean;
  errorMessage?: string;
  warnings: string[];
  isGitRepo?: boolean;
  expandedPath?: string;
  requiresClone?: boolean;
}

export interface SessionSource {
  type: InputType;
  localPath: string;
  branch?: string;
  gitHubRef?: GitHubRef;
  suggestedName: string;
  requiresClone: boolean;
}
