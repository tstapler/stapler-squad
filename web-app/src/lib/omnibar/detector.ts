/**
 * Input type detection engine for the omnibar
 * Ported from Go implementation at ui/overlay/omnibar/detector.go
 */

import { InputType, DetectionResult, GitHubRef } from "./types";

export interface Detector {
  name: string;
  priority: number;
  detect(input: string): DetectionResult | null;
}

/**
 * GitHub PR URL detector
 * Matches: https://github.com/owner/repo/pull/123
 */
class GitHubPRDetector implements Detector {
  name = "GitHubPR";
  priority = 10;

  private pattern = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/i;

  detect(input: string): DetectionResult | null {
    const match = input.trim().match(this.pattern);
    if (!match) return null;

    const [, owner, repo, prNumber] = match;
    const gitHubRef: GitHubRef = {
      owner,
      repo,
      prNumber: parseInt(prNumber, 10),
    };

    return {
      type: InputType.GitHubPR,
      confidence: 1.0,
      parsedValue: input.trim(),
      suggestedName: `pr-${prNumber}-${repo}`,
      gitHubRef,
    };
  }
}

/**
 * GitHub Branch URL detector
 * Matches: https://github.com/owner/repo/tree/branch-name
 */
class GitHubBranchDetector implements Detector {
  name = "GitHubBranch";
  priority = 20;

  private pattern = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/tree\/(.+)/i;

  detect(input: string): DetectionResult | null {
    const match = input.trim().match(this.pattern);
    if (!match) return null;

    const [, owner, repo, branch] = match;
    const gitHubRef: GitHubRef = {
      owner,
      repo,
      branch,
    };

    return {
      type: InputType.GitHubBranch,
      confidence: 1.0,
      parsedValue: input.trim(),
      suggestedName: `${repo}-${branch.replace(/\//g, "-")}`,
      branch,
      gitHubRef,
    };
  }
}

/**
 * GitHub Repository URL detector
 * Matches: https://github.com/owner/repo
 */
class GitHubRepoDetector implements Detector {
  name = "GitHubRepo";
  priority = 30;

  private pattern = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/?$/i;

  detect(input: string): DetectionResult | null {
    const match = input.trim().match(this.pattern);
    if (!match) return null;

    const [, owner, repo] = match;
    // Clean repo name (remove .git suffix if present)
    const cleanRepo = repo.replace(/\.git$/, "");
    const gitHubRef: GitHubRef = {
      owner,
      repo: cleanRepo,
    };

    return {
      type: InputType.GitHubRepo,
      confidence: 1.0,
      parsedValue: input.trim(),
      suggestedName: cleanRepo,
      gitHubRef,
    };
  }
}

/**
 * GitHub Shorthand detector
 * Matches: owner/repo or owner/repo:branch
 */
class GitHubShorthandDetector implements Detector {
  name = "GitHubShorthand";
  priority = 40;

  // Match owner/repo or owner/repo:branch (but not paths starting with / or ~)
  private pattern = /^([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_.-]+)(?::([a-zA-Z0-9_/.-]+))?$/;

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();

    // Skip if it looks like a local path
    if (trimmed.startsWith("/") || trimmed.startsWith("~") || trimmed.startsWith(".")) {
      return null;
    }

    const match = trimmed.match(this.pattern);
    if (!match) return null;

    const [, owner, repo, branch] = match;
    const gitHubRef: GitHubRef = {
      owner,
      repo,
      branch,
    };

    const suggestedName = branch
      ? `${repo}-${branch.replace(/\//g, "-")}`
      : repo;

    return {
      type: InputType.GitHubShorthand,
      confidence: 0.9,
      parsedValue: trimmed,
      suggestedName,
      branch,
      gitHubRef,
    };
  }
}

/**
 * Path with Branch detector
 * Matches: /path/to/repo@branch-name or ~/path@branch
 */
class PathWithBranchDetector implements Detector {
  name = "PathWithBranch";
  priority = 50;

  // Valid git branch name: alphanumeric, hyphens, underscores, forward slashes
  private pattern = /^(.+)@([a-zA-Z0-9_/.-]+)$/;

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();

    // Must contain @ but not be a URL
    if (!trimmed.includes("@") || trimmed.includes("://")) {
      return null;
    }

    const match = trimmed.match(this.pattern);
    if (!match) return null;

    const [, path, branch] = match;

    // Validate branch name
    if (!this.isValidBranchName(branch)) {
      return null;
    }

    // Extract suggested name from path
    const pathParts = path.split("/").filter(Boolean);
    const dirName = pathParts[pathParts.length - 1] || "session";
    const suggestedName = `${dirName}-${branch.replace(/\//g, "-")}`;

    return {
      type: InputType.PathWithBranch,
      confidence: 0.9,
      parsedValue: trimmed,
      suggestedName,
      localPath: path,
      branch,
    };
  }

  private isValidBranchName(name: string): boolean {
    if (!name || name.length > 255) return false;
    if (name.startsWith(".") || name.startsWith("-")) return false;
    if (name.endsWith(".lock") || name.endsWith(".")) return false;
    if (name.includes("..") || name.includes("//")) return false;
    if (/[~^:?*\[\\\s]/.test(name)) return false;
    return true;
  }
}

/**
 * Local Path detector (catch-all)
 * Matches: /path/to/dir, ~/path, ./path
 */
class LocalPathDetector implements Detector {
  name = "LocalPath";
  priority = 100;

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();

    if (!trimmed) return null;

    // Check for path-like patterns
    const isAbsolute = trimmed.startsWith("/");
    const hasTilde = trimmed.startsWith("~/");
    const isRelative = trimmed.startsWith("./") || trimmed.startsWith("../");

    // Also accept paths with multiple slashes that aren't GitHub shorthand
    const hasMultipleSlashes = (trimmed.match(/\//g) || []).length > 1;

    if (!isAbsolute && !hasTilde && !isRelative && !hasMultipleSlashes) {
      return null;
    }

    // Skip if contains @ (handled by PathWithBranch detector)
    if (trimmed.includes("@") && !trimmed.includes("://")) {
      return null;
    }

    // Skip URLs
    if (trimmed.includes("://")) {
      return null;
    }

    // Extract suggested name from path
    const pathParts = trimmed.split("/").filter(Boolean);
    const suggestedName = pathParts[pathParts.length - 1] || "session";

    return {
      type: InputType.LocalPath,
      confidence: 0.8,
      parsedValue: trimmed,
      suggestedName,
      localPath: trimmed,
    };
  }
}

/**
 * DetectorRegistry manages detectors and orchestrates detection
 */
export class DetectorRegistry {
  private detectors: Detector[] = [];

  register(detector: Detector): void {
    this.detectors.push(detector);
    // Sort by priority (lower = higher priority)
    this.detectors.sort((a, b) => a.priority - b.priority);
  }

  detect(input: string): DetectionResult {
    for (const detector of this.detectors) {
      const result = detector.detect(input);
      if (result) {
        return result;
      }
    }

    return {
      type: InputType.Unknown,
      confidence: 0,
      parsedValue: input,
      suggestedName: "",
    };
  }

  detectAll(input: string): DetectionResult[] {
    const results: DetectionResult[] = [];
    for (const detector of this.detectors) {
      const result = detector.detect(input);
      if (result) {
        results.push(result);
      }
    }
    return results;
  }
}

// Create default registry with all detectors
export function createDefaultRegistry(): DetectorRegistry {
  const registry = new DetectorRegistry();
  registry.register(new GitHubPRDetector());
  registry.register(new GitHubBranchDetector());
  registry.register(new GitHubRepoDetector());
  registry.register(new GitHubShorthandDetector());
  registry.register(new PathWithBranchDetector());
  registry.register(new LocalPathDetector());
  return registry;
}

// Singleton default registry
let defaultRegistry: DetectorRegistry | null = null;

export function getDefaultRegistry(): DetectorRegistry {
  if (!defaultRegistry) {
    defaultRegistry = createDefaultRegistry();
  }
  return defaultRegistry;
}

/**
 * Detect input type using the default registry
 */
export function detect(input: string): DetectionResult {
  return getDefaultRegistry().detect(input);
}
