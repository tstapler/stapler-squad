/**
 * GitHub URL Parser - TypeScript implementation mirroring github/url_parser.go
 * Parses GitHub URLs and shorthand references into structured data
 */

/**
 * Types of GitHub references that can be parsed
 */
export enum RefType {
  PR = 'PR',
  Branch = 'Branch',
  Repo = 'Repository',
}

/**
 * Parsed GitHub reference structure
 */
export interface ParsedGitHubRef {
  type: RefType;
  owner: string;
  repo: string;
  prNumber?: number;
  branch?: string;
  originalUrl: string;
}

// Regex patterns for parsing GitHub URLs
// Full GitHub URL patterns
const prURLPattern = /^(?:https?:\/\/)?github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)(?:\/.*)?$/;
const branchURLPattern = /^(?:https?:\/\/)?github\.com\/([^/]+)\/([^/]+)\/tree\/(.+)$/;
const repoURLPattern = /^(?:https?:\/\/)?github\.com\/([^/]+)\/([^/]+?)(?:\.git)?\/?$/;

// Shorthand patterns
const shorthandBranchPattern = /^([^/:]+)\/([^/:]+):(.+)$/;
const shorthandRepoPattern = /^([^/:]+)\/([^/:]+)$/;

/**
 * Checks if a string could be a valid GitHub username or repo name
 * GitHub names can contain alphanumeric, hyphens, underscores, and dots
 * They can't start with a hyphen
 */
function isValidGitHubName(name: string): boolean {
  if (!name || name.length > 100) {
    return false;
  }

  // Can't start with a hyphen
  if (name[0] === '-') {
    return false;
  }

  // Must contain only valid characters
  for (const char of name) {
    const code = char.charCodeAt(0);
    const isLowerCase = code >= 97 && code <= 122; // a-z
    const isUpperCase = code >= 65 && code <= 90; // A-Z
    const isDigit = code >= 48 && code <= 57; // 0-9
    const isSpecial = char === '-' || char === '_' || char === '.';

    if (!isLowerCase && !isUpperCase && !isDigit && !isSpecial) {
      return false;
    }
  }

  return true;
}

/**
 * Parses a GitHub URL or shorthand reference into a ParsedGitHubRef
 *
 * Supported formats:
 * - https://github.com/owner/repo/pull/123
 * - https://github.com/owner/repo/tree/branch-name
 * - https://github.com/owner/repo
 * - github.com/owner/repo
 * - owner/repo:branch-name
 * - owner/repo
 *
 * @param input - GitHub URL or shorthand reference
 * @returns Parsed reference or null if invalid
 */
export function parseGitHubRef(input: string): ParsedGitHubRef | null {
  const trimmed = input.trim();
  if (!trimmed) {
    return null;
  }

  // Try PR URL first
  const prMatch = trimmed.match(prURLPattern);
  if (prMatch) {
    const prNum = parseInt(prMatch[3], 10);
    if (isNaN(prNum)) {
      return null;
    }
    return {
      type: RefType.PR,
      owner: prMatch[1],
      repo: prMatch[2],
      prNumber: prNum,
      originalUrl: trimmed,
    };
  }

  // Try branch URL
  const branchMatch = trimmed.match(branchURLPattern);
  if (branchMatch) {
    return {
      type: RefType.Branch,
      owner: branchMatch[1],
      repo: branchMatch[2],
      branch: branchMatch[3],
      originalUrl: trimmed,
    };
  }

  // Try repo URL (full URL)
  const repoMatch = trimmed.match(repoURLPattern);
  if (repoMatch) {
    return {
      type: RefType.Repo,
      owner: repoMatch[1],
      repo: repoMatch[2],
      originalUrl: trimmed,
    };
  }

  // Try shorthand with branch (owner/repo:branch)
  const shorthandBranchMatch = trimmed.match(shorthandBranchPattern);
  if (shorthandBranchMatch) {
    return {
      type: RefType.Branch,
      owner: shorthandBranchMatch[1],
      repo: shorthandBranchMatch[2],
      branch: shorthandBranchMatch[3],
      originalUrl: trimmed,
    };
  }

  // Try shorthand repo (owner/repo)
  // Only match if it looks like a GitHub reference (contains exactly one slash)
  const shorthandRepoMatch = trimmed.match(shorthandRepoPattern);
  if (shorthandRepoMatch) {
    const owner = shorthandRepoMatch[1];
    const repo = shorthandRepoMatch[2];
    if (isValidGitHubName(owner) && isValidGitHubName(repo)) {
      return {
        type: RefType.Repo,
        owner,
        repo,
        originalUrl: trimmed,
      };
    }
  }

  return null;
}

/**
 * Quick check if the input string looks like a GitHub URL or reference
 * This is a fast check that doesn't validate the full format
 *
 * @param input - String to check
 * @returns true if it looks like a GitHub reference
 */
export function isGitHubRef(input: string): boolean {
  const trimmed = input.trim();
  if (!trimmed) {
    return false;
  }

  // Check for explicit GitHub URLs
  if (trimmed.includes('github.com/')) {
    return true;
  }

  // Check for shorthand formats (owner/repo or owner/repo:branch)
  // Must have exactly one "/" and optionally one ":"
  const slashCount = (trimmed.match(/\//g) || []).length;
  if (slashCount === 1) {
    const parts = trimmed.split('/', 2);
    if (parts.length === 2 && isValidGitHubName(parts[0])) {
      // Remove any :branch suffix for repo name validation
      const repo = parts[1].split(':', 2)[0];
      if (isValidGitHubName(repo)) {
        return true;
      }
    }
  }

  return false;
}

/**
 * Get the HTTPS clone URL for the repository
 * @param ref - Parsed GitHub reference
 * @returns Clone URL (e.g., "https://github.com/owner/repo.git")
 */
export function getCloneUrl(ref: ParsedGitHubRef): string {
  return `https://github.com/${ref.owner}/${ref.repo}.git`;
}

/**
 * Get the human-readable GitHub URL
 * @param ref - Parsed GitHub reference
 * @returns HTML URL for viewing in browser
 */
export function getHtmlUrl(ref: ParsedGitHubRef): string {
  switch (ref.type) {
    case RefType.PR:
      return `https://github.com/${ref.owner}/${ref.repo}/pull/${ref.prNumber}`;
    case RefType.Branch:
      return `https://github.com/${ref.owner}/${ref.repo}/tree/${ref.branch}`;
    default:
      return `https://github.com/${ref.owner}/${ref.repo}`;
  }
}

/**
 * Get a human-readable name for the reference
 * @param ref - Parsed GitHub reference
 * @returns Display name (e.g., "owner/repo#123", "owner/repo:branch", "owner/repo")
 */
export function getDisplayName(ref: ParsedGitHubRef): string {
  switch (ref.type) {
    case RefType.PR:
      return `${ref.owner}/${ref.repo}#${ref.prNumber}`;
    case RefType.Branch:
      return `${ref.owner}/${ref.repo}:${ref.branch}`;
    default:
      return `${ref.owner}/${ref.repo}`;
  }
}

/**
 * Get a suggested session name based on the reference
 * @param ref - Parsed GitHub reference
 * @returns Suggested session name
 */
export function getSuggestedSessionName(ref: ParsedGitHubRef): string {
  switch (ref.type) {
    case RefType.PR:
      return `pr-${ref.prNumber}-${ref.repo}`;
    case RefType.Branch: {
      // Sanitize branch name for session name
      const branch = ref.branch!.replace(/\//g, '-');
      return `${ref.repo}-${branch}`;
    }
    default:
      return ref.repo;
  }
}

/**
 * Get the repository full name in "owner/repo" format
 * @param ref - Parsed GitHub reference
 * @returns Repository full name
 */
export function getRepoFullName(ref: ParsedGitHubRef): string {
  return `${ref.owner}/${ref.repo}`;
}
