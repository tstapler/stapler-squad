/**
 * GitHubEnterpriseURLDetector matches GitHub PR/branch/repo URLs against a
 * caller-supplied list of configured GHES hostnames (github.com is handled by
 * the static GitHubPRDetector/GitHubBranchDetector/GitHubRepoDetector).
 * Priority 15 — after GitHubPRDetector (10), before GitHubBranchDetector (20).
 */

import { Detector } from "../detector";
import { DetectionResult, GitHubRef, InputType } from "../types";

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export class GitHubEnterpriseURLDetector implements Detector {
  name = "GitHubEnterpriseURL";
  priority = 15;

  private hosts: string[];

  constructor(hosts: string[]) {
    this.hosts = hosts.filter(Boolean);
  }

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();

    for (const host of this.hosts) {
      const hostPattern = escapeRegExp(host);

      const prMatch = trimmed.match(
        new RegExp(`^https?://${hostPattern}/([^/]+)/([^/]+)/pull/(\\d+)`, "i")
      );
      if (prMatch) {
        const [, owner, repo, prNumber] = prMatch;
        const gitHubRef: GitHubRef = {
          owner,
          repo,
          prNumber: parseInt(prNumber, 10),
        };
        return {
          type: InputType.GitHubPR,
          confidence: 1.0,
          parsedValue: trimmed,
          suggestedName: `${owner}-${repo}-pr-${prNumber}`,
          gitHubRef,
        };
      }

      const branchMatch = trimmed.match(
        new RegExp(`^https?://${hostPattern}/([^/]+)/([^/]+)/tree/(.+)`, "i")
      );
      if (branchMatch) {
        const [, owner, repo, branch] = branchMatch;
        const gitHubRef: GitHubRef = {
          owner,
          repo,
          branch,
        };
        return {
          type: InputType.GitHubBranch,
          confidence: 1.0,
          parsedValue: trimmed,
          suggestedName: `${owner}-${repo}-${branch.replace(/\//g, "-")}`,
          branch,
          gitHubRef,
        };
      }

      const repoMatch = trimmed.match(
        new RegExp(`^https?://${hostPattern}/([^/]+)/([^/]+)/?$`, "i")
      );
      if (repoMatch) {
        const [, owner, repo] = repoMatch;
        const cleanRepo = repo.replace(/\.git$/, "");
        const gitHubRef: GitHubRef = {
          owner,
          repo: cleanRepo,
        };
        return {
          type: InputType.GitHubRepo,
          confidence: 1.0,
          parsedValue: trimmed,
          suggestedName: cleanRepo,
          gitHubRef,
        };
      }
    }

    return null;
  }
}
