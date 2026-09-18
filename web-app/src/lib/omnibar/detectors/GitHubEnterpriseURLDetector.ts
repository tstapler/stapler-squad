/**
 * GitHubEnterpriseURLDetector matches GitHub PR/branch/repo URLs against a
 * caller-supplied list of configured GHES hostnames — the same URL shapes
 * the static GitHubPRDetector/GitHubBranchDetector/GitHubRepoDetector match
 * for github.com. Priority 15 — after GitHubPRDetector (10), before
 * GitHubBranchDetector (20).
 *
 * The host list starts empty and the detector is registered synchronously as
 * part of createDefaultRegistry() — see detector.ts — so the registry always
 * has an enterprise-detector slot. OmnibarContext calls setHosts() once the
 * enterprise host list loads (or changes) instead of swapping the detector
 * instance via unregister/register, which previously left a window (before
 * the async host-loading effect first ran) where no enterprise detector was
 * registered at all and GHES URLs fell through to the SessionSearch
 * catch-all.
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

  constructor(hosts: string[] = []) {
    this.hosts = hosts.filter(Boolean);
  }

  /** Replace the configured host list in place (called as hosts load/change). */
  setHosts(hosts: string[]): void {
    this.hosts = hosts.filter(Boolean);
  }

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();

    for (const host of this.hosts) {
      const result = this.detectForHost(trimmed, host);
      if (result) return result;
    }

    return null;
  }

  private detectForHost(trimmed: string, host: string): DetectionResult | null {
    const h = escapeRegExp(host);

    const prMatch = trimmed.match(new RegExp(`^https?://${h}/([^/]+)/([^/]+)/pull/(\\d+)`, "i"));
    if (prMatch) {
      const [, owner, repo, prNumber] = prMatch;
      const gitHubRef: GitHubRef = { owner, repo, prNumber: parseInt(prNumber, 10) };
      return {
        type: InputType.GitHubPR,
        confidence: 1.0,
        parsedValue: trimmed,
        suggestedName: `${owner}-${repo}-pr-${prNumber}`,
        gitHubRef,
      };
    }

    const branchMatch = trimmed.match(new RegExp(`^https?://${h}/([^/]+)/([^/]+)/tree/(.+)`, "i"));
    if (branchMatch) {
      const [, owner, repo, branch] = branchMatch;
      const gitHubRef: GitHubRef = { owner, repo, branch };
      return {
        type: InputType.GitHubBranch,
        confidence: 1.0,
        parsedValue: trimmed,
        suggestedName: `${owner}-${repo}-${branch.replace(/\//g, "-")}`,
        branch,
        gitHubRef,
      };
    }

    const repoMatch = trimmed.match(new RegExp(`^https?://${h}/([^/]+)/([^/]+)/?$`, "i"));
    if (repoMatch) {
      const [, owner, repo] = repoMatch;
      const cleanRepo = repo.replace(/\.git$/, "");
      const gitHubRef: GitHubRef = { owner, repo: cleanRepo };
      return {
        type: InputType.GitHubRepo,
        confidence: 1.0,
        parsedValue: trimmed,
        suggestedName: cleanRepo,
        gitHubRef,
      };
    }

    return null;
  }
}
