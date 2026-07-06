"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { useSelector } from "react-redux";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import type { RootState } from "@/lib/store/store";
import { GitHubAuthError, type GitHubRepo, type GitHubIssue } from "./useBacklogService";
import {
  getCachedRepos,
  setCachedRepos,
  getCachedIssues,
  setCachedIssues,
  getRecentRepos,
  addRecentRepo,
} from "@/lib/utils/issuePickerCache";

// ─── Types ───────────────────────────────────────────────────────────────────

export type PickerPhase = "repo" | "issue";

export interface UseGitHubIssuePickerOptions {
  searchGitHubRepos: (query: string, limit?: number) => Promise<GitHubRepo[]>;
  listGitHubIssues: (
    owner: string,
    repo: string,
    options?: { state?: string; search?: string; limit?: number }
  ) => Promise<GitHubIssue[]>;
  onSelect: (owner: string, repo: string, issue: GitHubIssue) => void;
}

export interface UseGitHubIssuePickerReturn {
  phase: PickerPhase;
  // Repo phase
  repoQuery: string;
  repos: GitHubRepo[];
  repoHistoryCount: number;
  reposLoading: boolean;
  selectedRepo: GitHubRepo | null;
  setRepoQuery: (q: string) => void;
  selectRepo: (repo: GitHubRepo) => void;
  // Issue phase
  issueSearch: string;
  issueState: "open" | "closed" | "all";
  issues: GitHubIssue[];
  issuesLoading: boolean;
  setIssueSearch: (s: string) => void;
  setIssueState: (s: "open" | "closed" | "all") => void;
  selectIssue: (issue: GitHubIssue) => void;
  // Two-level Escape: back to repo selection
  goBack: () => void;
  // Auth
  authError: boolean;
  reset: () => void;
}

// ─── Hook ────────────────────────────────────────────────────────────────────

const ISSUE_DEBOUNCE_MS = 150;

export function useGitHubIssuePicker({
  searchGitHubRepos,
  listGitHubIssues,
  onSelect,
}: UseGitHubIssuePickerOptions): UseGitHubIssuePickerReturn {
  const [phase, setPhase] = useState<PickerPhase>("repo");
  const [repoQuery, setRepoQuery] = useState("");
  const [repos, setRepos] = useState<GitHubRepo[]>([]);
  const [reposLoading, setReposLoading] = useState(false);
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [issueSearch, setIssueSearch] = useState("");
  const [issueState, setIssueState] = useState<"open" | "closed" | "all">("open");
  const [issues, setIssues] = useState<GitHubIssue[]>([]);
  const [issuesLoading, setIssuesLoading] = useState(false);
  const [authError, setAuthError] = useState(false);

  // In-memory cache of ALL fetched repos — filtered client-side as user types.
  const allReposRef = useRef<GitHubRepo[]>([]);
  const issueGenRef = useRef(0);

  // Pull local-path repos from Redux session list for the "local repos" tier.
  const localRepos = useSelector((state: RootState) => {
    const sessions = selectAllSessions(state);
    const seen = new Set<string>();
    const results: GitHubRepo[] = [];
    for (const s of sessions) {
      if (!s.path) continue;
      const parts = s.path.split("/");
      const repo = parts[parts.length - 1] ?? "";
      const owner = parts[parts.length - 2] ?? "";
      const key = `${owner}/${repo}`;
      if (seen.has(key)) continue;
      seen.add(key);
      results.push({ owner, repo, isLocal: true, localPath: s.path, description: s.title ?? "" });
    }
    return results;
  });

  // ─── Repo phase: one-time eager fetch, then filter client-side ─────────────

  // Step 1: fetch full repo list once on mount (or from localStorage cache).
  useEffect(() => {
    // Seed from localStorage cache immediately so the list appears before the
    // network response arrives.
    const cached = getCachedRepos();
    if (cached && cached.length > 0) {
      const mapped = cached.map((r) => ({ ...r, isLocal: false, localPath: "" }));
      allReposRef.current = mapped;
      setRepos(mapped);
    }

    let cancelled = false;
    setReposLoading(true);

    searchGitHubRepos("", 100)
      .then((results) => {
        if (cancelled) return;
        allReposRef.current = results;
        setRepos(results);
        setCachedRepos(
          results.map((r) => ({ owner: r.owner, repo: r.repo, description: r.description }))
        );
        setAuthError(false);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof GitHubAuthError) setAuthError(true);
      })
      .finally(() => {
        if (!cancelled) setReposLoading(false);
      });

    return () => { cancelled = true; };
  // Run once on mount — searchGitHubRepos identity is stable from the service hook.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ─── Issue phase: fetch on search/state/repo change ────────────────────────

  useEffect(() => {
    if (phase !== "issue" || !selectedRepo) return;

    const gen = ++issueGenRef.current;
    const { owner, repo } = selectedRepo;

    // Check cache (only for no-search case).
    if (issueSearch === "") {
      const cached = getCachedIssues(owner, repo, issueState);
      if (cached) {
        if (gen === issueGenRef.current) {
          setIssues(cached.map((c) => ({ ...c, isPR: c.isPR ?? false })));
        }
        return;
      }
    }

    let timer: ReturnType<typeof setTimeout> | null = null;

    timer = setTimeout(async () => {
      if (gen !== issueGenRef.current) return;
      setIssuesLoading(true);
      try {
        const results = await listGitHubIssues(owner, repo, {
          state: issueState,
          search: issueSearch,
          limit: 50,
        });
        if (gen !== issueGenRef.current) return;
        setIssues(results);
        if (issueSearch === "") {
          setCachedIssues(owner, repo, issueState, results);
        }
        setAuthError(false);
      } catch (err) {
        if (gen !== issueGenRef.current) return;
        if (err instanceof GitHubAuthError) {
          setAuthError(true);
        }
      } finally {
        if (gen === issueGenRef.current) setIssuesLoading(false);
      }
    }, ISSUE_DEBOUNCE_MS);

    return () => {
      if (timer !== null) clearTimeout(timer);
    };
  }, [phase, selectedRepo, issueSearch, issueState, listGitHubIssues]);

  // ─── Actions ────────────────────────────────────────────────────────────────

  const selectRepo = useCallback((repo: GitHubRepo) => {
    setSelectedRepo(repo);
    addRecentRepo(repo.owner, repo.repo);
    setIssues([]);
    setIssueSearch("");
    setPhase("issue");
  }, []);

  const selectIssue = useCallback(
    (issue: GitHubIssue) => {
      if (!selectedRepo) return;
      onSelect(selectedRepo.owner, selectedRepo.repo, issue);
    },
    [selectedRepo, onSelect]
  );

  const goBack = useCallback(() => {
    setPhase("repo");
    setSelectedRepo(null);
    setIssues([]);
    setIssueSearch("");
  }, []);

  const reset = useCallback(() => {
    setPhase("repo");
    setRepoQuery("");
    setRepos(allReposRef.current);
    setSelectedRepo(null);
    setIssues([]);
    setIssueSearch("");
    setIssueState("open");
    setAuthError(false);
    issueGenRef.current = 0;
  }, []);

  // Build the full merged + filtered + history-tiered repo list.
  // Returns [repos, historyCount] — historyCount is the number of leading history entries.
  const mergedRepos = useCallback(
    (fetched: GitHubRepo[]): [GitHubRepo[], number] => {
      // Enrich fetched repos with local path info from session list.
      const localMap = new Map(localRepos.map((r) => [`${r.owner}/${r.repo}`, r.localPath]));
      const enrichedFetched = fetched.map((r) => {
        const localPath = localMap.get(`${r.owner}/${r.repo}`);
        return localPath ? { ...r, isLocal: true, localPath } : r;
      });

      // Local-only: session repos not returned by the GitHub API.
      const fetchedKeys = new Set(fetched.map((r) => `${r.owner}/${r.repo}`));
      const localOnly = localRepos.filter((r) => !fetchedKeys.has(`${r.owner}/${r.repo}`));

      // Full candidate list.
      const all = [...enrichedFetched, ...localOnly];

      // Apply query filter.
      const q = repoQuery.trim().toLowerCase();
      const filtered =
        q === ""
          ? all
          : all.filter(
              (r) =>
                `${r.owner}/${r.repo}`.toLowerCase().includes(q) ||
                r.description?.toLowerCase().includes(q)
            );

      // History tier: recent repos (in recency order) that appear in filtered results.
      const recentList = getRecentRepos();
      const historyRepos = recentList
        .map((recent) =>
          filtered.find((f) => f.owner === recent.owner && f.repo === recent.repo)
        )
        .filter((r): r is GitHubRepo => r !== undefined);
      const historyKeySet = new Set(historyRepos.map((r) => `${r.owner}/${r.repo}`));
      const otherRepos = filtered.filter((r) => !historyKeySet.has(`${r.owner}/${r.repo}`));

      return [[...historyRepos, ...otherRepos], historyRepos.length];
    },
    [localRepos, repoQuery]
  );

  const [computedRepos, repoHistoryCount] = mergedRepos(repos);

  return {
    phase,
    repoQuery,
    repos: computedRepos,
    repoHistoryCount,
    reposLoading,
    selectedRepo,
    setRepoQuery,
    selectRepo,
    issueSearch,
    issueState,
    issues,
    issuesLoading,
    setIssueSearch,
    setIssueState,
    selectIssue,
    goBack,
    authError,
    reset,
  };
}
