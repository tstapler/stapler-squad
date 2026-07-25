// TTL cache for the GitHub issue picker — keyed under the page's origin so
// multiple instances (dev / prod) never share stale data.

const TTL_MS = 5 * 60 * 1000; // 5 minutes

function cacheKey(suffix: string): string {
  if (typeof window === "undefined") return `ssq:ssr:${suffix}`;
  return `ssq:${window.location.origin}:${suffix}`;
}

interface CacheEntry<T> {
  data: T;
  expiry: number;
}

function readEntry<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return null;
    const entry = JSON.parse(raw) as CacheEntry<T>;
    if (Date.now() > entry.expiry) {
      localStorage.removeItem(key);
      return null;
    }
    return entry.data;
  } catch {
    return null;
  }
}

function writeEntry<T>(key: string, data: T): void {
  try {
    const entry: CacheEntry<T> = { data, expiry: Date.now() + TTL_MS };
    localStorage.setItem(key, JSON.stringify(entry));
  } catch {
    // localStorage full or unavailable — silently skip
  }
}

// ─── Repo list cache ────────────────────────────────────────────────────────

const REPOS_KEY = () => cacheKey("github:repos");

export interface CachedRepoEntry {
  owner: string;
  repo: string;
  description: string;
}

export function getCachedRepos(): CachedRepoEntry[] | null {
  return readEntry<CachedRepoEntry[]>(REPOS_KEY());
}

export function setCachedRepos(repos: CachedRepoEntry[]): void {
  writeEntry(REPOS_KEY(), repos);
}

// ─── Issue list cache ───────────────────────────────────────────────────────

function issuesKey(owner: string, repo: string, state: string): string {
  return cacheKey(`github:issues:${owner}/${repo}:${state}`);
}

export interface CachedIssueEntry {
  number: number;
  title: string;
  body?: string;
  author?: string;
  state: string;
  url: string;
  labels: string[];
  createdAt?: string;
  updatedAt?: string;
  isPR?: boolean;
}

export function getCachedIssues(
  owner: string,
  repo: string,
  state: string
): CachedIssueEntry[] | null {
  return readEntry<CachedIssueEntry[]>(issuesKey(owner, repo, state));
}

export function setCachedIssues(
  owner: string,
  repo: string,
  state: string,
  issues: CachedIssueEntry[]
): void {
  writeEntry(issuesKey(owner, repo, state), issues);
}

// ─── Recent repos (multi-entry history, up to 5) ───────────────────────────

const RECENT_REPOS_KEY = () => cacheKey("github:recentRepos");
const RECENT_REPOS_MAX = 5;

export interface RecentRepo {
  owner: string;
  repo: string;
}

export function getRecentRepos(): RecentRepo[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(RECENT_REPOS_KEY());
    if (!raw) return [];
    return JSON.parse(raw) as RecentRepo[];
  } catch {
    return [];
  }
}

export function addRecentRepo(owner: string, repo: string): void {
  if (typeof window === "undefined") return;
  try {
    const existing = getRecentRepos();
    const key = `${owner}/${repo}`;
    const filtered = existing.filter((r) => `${r.owner}/${r.repo}` !== key);
    const updated = [{ owner, repo }, ...filtered].slice(0, RECENT_REPOS_MAX);
    localStorage.setItem(RECENT_REPOS_KEY(), JSON.stringify(updated));
  } catch {
    // ignore
  }
}
