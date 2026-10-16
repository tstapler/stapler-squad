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
  File = 'File',
  Commit = 'Commit',
  Issue = 'Issue',
  Compare = 'Compare',
  Release = 'Release',
}

/**
 * Parsed GitHub reference structure
 */
export interface ParsedGitHubRef {
  type: RefType;
  owner: string;
  repo: string;
  prNumber?: number;
  issueNumber?: number;
  branch?: string;
  commitSHA?: string;
  filePath?: string;
  lineStart?: number;
  lineEnd?: number;
  baseBranch?: string;
  headBranch?: string;
  tag?: string;
  originalUrl: string;
  /** GitHub Enterprise host that owns this ref, e.g. "github.netflix.net". Unset means github.com. */
  host?: string;
}

const GITHUB_COM = "github.com";

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Per-host URL regex patterns, mirroring the github.com patterns but built
 * against an arbitrary hostname so GitHub Enterprise URLs (github.netflix.net
 * and friends) are recognized the same way. SSH/shorthand forms are handled
 * separately since SSH URLs embed the host directly and shorthand refs
 * (owner/repo) have no host component at all.
 */
interface HostURLPatterns {
  prURLPattern: RegExp;
  branchURLPattern: RegExp;
  fileURLPattern: RegExp;
  commitURLPattern: RegExp;
  issueURLPattern: RegExp;
  compareURLPattern: RegExp;
  releaseURLPattern: RegExp;
  repoURLPattern: RegExp;
  sshURLPattern: RegExp;
  sshProtocolURLPattern: RegExp;
}

function buildHostURLPatterns(host: string): HostURLPatterns {
  const h = escapeRegExp(host);
  return {
    // https://<host>/owner/repo/pull/123
    prURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+)/pull/(\\d+)(?:/.*)?$`),
    // https://<host>/owner/repo/tree/branch-name
    branchURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+)/tree/(.+)$`),
    // https://<host>/owner/repo/blob/branch/path/to/file.go#L10-L20
    fileURLPattern: new RegExp(
      `^(?:https?://)?${h}/([^/]+)/([^/]+)/blob/([^/]+)/(.+?)(?:#L(\\d+)(?:-L(\\d+))?)?$`
    ),
    // https://<host>/owner/repo/commit/abc123def456
    commitURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+)/commit/([a-fA-F0-9]+)(?:/.*)?$`),
    // https://<host>/owner/repo/issues/42
    issueURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+)/issues/(\\d+)(?:/.*)?$`),
    // https://<host>/owner/repo/compare/main...feature
    compareURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+)/compare/([^.]+)\\.\\.\\.(.+)$`),
    // https://<host>/owner/repo/releases/tag/v1.0.0
    releaseURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+)/releases/tag/(.+)$`),
    // https://<host>/owner/repo or <host>/owner/repo
    repoURLPattern: new RegExp(`^(?:https?://)?${h}/([^/]+)/([^/]+?)(?:\\.git)?/?$`),
    // git@<host>:owner/repo.git
    sshURLPattern: new RegExp(`^git@${h}:([^/]+)/([^/]+?)(?:\\.git)?$`),
    // ssh://git@<host>/owner/repo.git
    sshProtocolURLPattern: new RegExp(`^ssh://git@${h}/([^/]+)/([^/]+?)(?:\\.git)?$`),
  };
}

const hostPatternCache = new Map<string, HostURLPatterns>();

function patternsForHost(host: string): HostURLPatterns {
  let patterns = hostPatternCache.get(host);
  if (!patterns) {
    patterns = buildHostURLPatterns(host);
    hostPatternCache.set(host, patterns);
  }
  return patterns;
}

// Shorthand patterns
// owner/repo:branch
const shorthandBranchPattern = /^([^/:]+)\/([^/:]+):(.+)$/;

// owner/repo (no colon, no https)
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
 * - https://github.com/owner/repo/blob/branch/path/to/file.go
 * - https://github.com/owner/repo/blob/branch/file.go#L10 (with line number)
 * - https://github.com/owner/repo/blob/branch/file.go#L10-L20 (with line range)
 * - https://github.com/owner/repo/commit/abc123
 * - https://github.com/owner/repo/issues/42
 * - https://github.com/owner/repo/compare/main...feature
 * - https://github.com/owner/repo/releases/tag/v1.0.0
 * - https://github.com/owner/repo
 * - github.com/owner/repo
 * - git@github.com:owner/repo.git (SSH)
 * - ssh://git@github.com/owner/repo (SSH protocol)
 * - owner/repo:branch-name
 * - owner/repo
 *
 * Tries every URL-based pattern (PR, file, commit, issue, compare, release,
 * branch, repo, SSH) against a single host. Shorthand forms (owner/repo) have
 * no host component and are handled by the caller once, not per-host.
 */
function parseGitHubRefForHost(trimmed: string, host: string): ParsedGitHubRef | null {
  const p = patternsForHost(host);
  const hostField = host === GITHUB_COM ? {} : { host };

  const prMatch = trimmed.match(p.prURLPattern);
  if (prMatch) {
    const prNum = parseInt(prMatch[3], 10);
    if (isNaN(prNum)) return null;
    return { type: RefType.PR, owner: prMatch[1], repo: prMatch[2], prNumber: prNum, originalUrl: trimmed, ...hostField };
  }

  // File/blob URL must be tried before branch URL since it's more specific
  const fileMatch = trimmed.match(p.fileURLPattern);
  if (fileMatch) {
    const ref: ParsedGitHubRef = {
      type: RefType.File,
      owner: fileMatch[1],
      repo: fileMatch[2],
      branch: fileMatch[3],
      filePath: fileMatch[4],
      originalUrl: trimmed,
      ...hostField,
    };
    if (fileMatch[5]) ref.lineStart = parseInt(fileMatch[5], 10);
    if (fileMatch[6]) ref.lineEnd = parseInt(fileMatch[6], 10);
    return ref;
  }

  const commitMatch = trimmed.match(p.commitURLPattern);
  if (commitMatch) {
    return { type: RefType.Commit, owner: commitMatch[1], repo: commitMatch[2], commitSHA: commitMatch[3], originalUrl: trimmed, ...hostField };
  }

  const issueMatch = trimmed.match(p.issueURLPattern);
  if (issueMatch) {
    const issueNum = parseInt(issueMatch[3], 10);
    if (isNaN(issueNum)) return null;
    return { type: RefType.Issue, owner: issueMatch[1], repo: issueMatch[2], issueNumber: issueNum, originalUrl: trimmed, ...hostField };
  }

  const compareMatch = trimmed.match(p.compareURLPattern);
  if (compareMatch) {
    return { type: RefType.Compare, owner: compareMatch[1], repo: compareMatch[2], baseBranch: compareMatch[3], headBranch: compareMatch[4], originalUrl: trimmed, ...hostField };
  }

  const releaseMatch = trimmed.match(p.releaseURLPattern);
  if (releaseMatch) {
    return { type: RefType.Release, owner: releaseMatch[1], repo: releaseMatch[2], tag: releaseMatch[3], originalUrl: trimmed, ...hostField };
  }

  const branchMatch = trimmed.match(p.branchURLPattern);
  if (branchMatch) {
    return { type: RefType.Branch, owner: branchMatch[1], repo: branchMatch[2], branch: branchMatch[3], originalUrl: trimmed, ...hostField };
  }

  const repoMatch = trimmed.match(p.repoURLPattern);
  if (repoMatch) {
    return { type: RefType.Repo, owner: repoMatch[1], repo: repoMatch[2], originalUrl: trimmed, ...hostField };
  }

  // git@<host>:owner/repo.git
  const sshMatch = trimmed.match(p.sshURLPattern);
  if (sshMatch) {
    return { type: RefType.Repo, owner: sshMatch[1], repo: sshMatch[2], originalUrl: trimmed, ...hostField };
  }

  // ssh://git@<host>/owner/repo.git
  const sshProtocolMatch = trimmed.match(p.sshProtocolURLPattern);
  if (sshProtocolMatch) {
    return { type: RefType.Repo, owner: sshProtocolMatch[1], repo: sshProtocolMatch[2], originalUrl: trimmed, ...hostField };
  }

  return null;
}

/**
 * @param input - GitHub URL or shorthand reference
 * @param enterpriseHosts - Configured GitHub Enterprise hostnames (e.g. from
 *   useGitHubEnterpriseHosts()) to recognize in addition to github.com.
 * @returns Parsed reference or null if invalid
 */
export function parseGitHubRef(
  input: string,
  enterpriseHosts: string[] = []
): ParsedGitHubRef | null {
  const trimmed = input.trim();
  if (!trimmed) {
    return null;
  }

  const hosts = [GITHUB_COM, ...enterpriseHosts.filter(Boolean)];
  for (const host of hosts) {
    const ref = parseGitHubRefForHost(trimmed, host);
    if (ref) return ref;
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
 * @param enterpriseHosts - Configured GitHub Enterprise hostnames (e.g. from
 *   useGitHubEnterpriseHosts()) to recognize in addition to github.com.
 * @returns true if it looks like a GitHub reference
 */
export function isGitHubRef(input: string, enterpriseHosts: string[] = []): boolean {
  const trimmed = input.trim();
  if (!trimmed) {
    return false;
  }

  const hosts = [GITHUB_COM, ...enterpriseHosts.filter(Boolean)];
  for (const host of hosts) {
    // Check for explicit HTTPS URLs
    if (trimmed.includes(`${host}/`)) {
      return true;
    }

    // Check for SSH URL format (git@<host>:owner/repo)
    if (trimmed.startsWith(`git@${host}:`)) {
      return true;
    }

    // Check for SSH protocol URL (ssh://git@<host>/)
    if (trimmed.startsWith(`ssh://git@${host}/`)) {
      return true;
    }
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
 * @param host - GitHub(.com) or GitHub Enterprise host; defaults to ref.host
 *   (set by parseGitHubRef when the ref came from a GHE URL) or "github.com".
 * @returns Clone URL (e.g., "https://github.com/owner/repo.git")
 */
export function getCloneUrl(ref: ParsedGitHubRef, host: string = ref.host || GITHUB_COM): string {
  return `https://${host}/${ref.owner}/${ref.repo}.git`;
}

/**
 * Get the human-readable GitHub URL
 * @param ref - Parsed GitHub reference
 * @param host - GitHub(.com) or GitHub Enterprise host; defaults to ref.host
 *   (set by parseGitHubRef when the ref came from a GHE URL) or "github.com".
 * @returns HTML URL for viewing in browser
 */
export function getHtmlUrl(ref: ParsedGitHubRef, host: string = ref.host || GITHUB_COM): string {
  switch (ref.type) {
    case RefType.PR:
      return `https://${host}/${ref.owner}/${ref.repo}/pull/${ref.prNumber}`;
    case RefType.Branch:
      return `https://${host}/${ref.owner}/${ref.repo}/tree/${ref.branch}`;
    case RefType.File: {
      let url = `https://${host}/${ref.owner}/${ref.repo}/blob/${ref.branch}/${ref.filePath}`;
      if (ref.lineStart) {
        if (ref.lineEnd && ref.lineEnd !== ref.lineStart) {
          url += `#L${ref.lineStart}-L${ref.lineEnd}`;
        } else {
          url += `#L${ref.lineStart}`;
        }
      }
      return url;
    }
    case RefType.Commit:
      return `https://${host}/${ref.owner}/${ref.repo}/commit/${ref.commitSHA}`;
    case RefType.Issue:
      return `https://${host}/${ref.owner}/${ref.repo}/issues/${ref.issueNumber}`;
    case RefType.Compare:
      return `https://${host}/${ref.owner}/${ref.repo}/compare/${ref.baseBranch}...${ref.headBranch}`;
    case RefType.Release:
      return `https://${host}/${ref.owner}/${ref.repo}/releases/tag/${ref.tag}`;
    default:
      return `https://${host}/${ref.owner}/${ref.repo}`;
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
    case RefType.File: {
      let name = `${ref.owner}/${ref.repo}:${ref.branch}/${ref.filePath}`;
      if (ref.lineStart) {
        if (ref.lineEnd && ref.lineEnd !== ref.lineStart) {
          name += `#L${ref.lineStart}-L${ref.lineEnd}`;
        } else {
          name += `#L${ref.lineStart}`;
        }
      }
      return name;
    }
    case RefType.Commit: {
      let shortSHA = ref.commitSHA || '';
      if (shortSHA.length > 7) {
        shortSHA = shortSHA.substring(0, 7);
      }
      return `${ref.owner}/${ref.repo}@${shortSHA}`;
    }
    case RefType.Issue:
      return `${ref.owner}/${ref.repo}#${ref.issueNumber} (issue)`;
    case RefType.Compare:
      return `${ref.owner}/${ref.repo}: ${ref.baseBranch}...${ref.headBranch}`;
    case RefType.Release:
      return `${ref.owner}/${ref.repo}@${ref.tag}`;
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
      return `${ref.owner}-${ref.repo}-pr-${ref.prNumber}`;
    case RefType.Branch: {
      // Sanitize branch name for session name
      const branch = (ref.branch || '').replace(/\//g, '-');
      return `${ref.owner}-${ref.repo}-${branch}`;
    }
    case RefType.File: {
      // Use the file name as part of the session name
      const parts = (ref.filePath || '').split('/');
      let fileName = parts[parts.length - 1];
      // Remove extension for cleaner name
      const extIndex = fileName.lastIndexOf('.');
      if (extIndex > 0) {
        fileName = fileName.substring(0, extIndex);
      }
      const branch = (ref.branch || '').replace(/\//g, '-');
      return `${ref.owner}-${ref.repo}-${branch}-${fileName}`;
    }
    case RefType.Commit: {
      let shortSHA = ref.commitSHA || '';
      if (shortSHA.length > 7) {
        shortSHA = shortSHA.substring(0, 7);
      }
      return `${ref.owner}-${ref.repo}-commit-${shortSHA}`;
    }
    case RefType.Issue:
      return `${ref.owner}-${ref.repo}-issue-${ref.issueNumber}`;
    case RefType.Compare: {
      const base = (ref.baseBranch || '').replace(/\//g, '-');
      const head = (ref.headBranch || '').replace(/\//g, '-');
      return `${ref.owner}-${ref.repo}-${base}-vs-${head}`;
    }
    case RefType.Release: {
      const tag = (ref.tag || '').replace(/\//g, '-');
      return `${ref.owner}-${ref.repo}-${tag}`;
    }
    default:
      return `${ref.owner}-${ref.repo}`;
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
