// +feature: unfinished-github-prs
"use client";

import { useState, useCallback, useRef, useEffect, useMemo } from "react";
import Link from "next/link";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { useGitHubPRs } from "@/lib/hooks/useGitHubPRs";
import {
  GitHubUserService,
  type GitHubAccount,
  type GitHubCLIHost,
  StartGitHubDeviceAuthRequestSchema,
  PollGitHubDeviceAuthRequestSchema,
  RevokeGitHubTokenRequestSchema,
  AddGitHubAccountWithTokenRequestSchema,
  ListGitHubCLIHostsRequestSchema,
  AddGitHubAccountFromCLIRequestSchema,
  DeviceAuthStatus,
} from "@/gen/session/v1/github_user_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import * as styles from "./GitHubPRsSection.css";

function useGitHubUserClient() {
  return useMemo(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    return createClient(GitHubUserService, transport);
  }, []);
}

function prCheckChip(pr: UserPR): React.ReactNode {
  if (pr.isDraft) {
    return <span className={styles.chipDraft}>Draft</span>;
  }
  const conclusion = pr.checkConclusion;
  if (conclusion === "success" || conclusion === "completed") {
    return <span className={styles.chipSuccess}>✓ CI</span>;
  }
  if (conclusion === "failure" || conclusion === "error") {
    return <span className={styles.chipError}>✗ CI</span>;
  }
  return null;
}

function prReviewChip(pr: UserPR): React.ReactNode {
  if (pr.changesReqCount > 0) {
    return (
      <span className={styles.chipError}>
        {pr.changesReqCount} change{pr.changesReqCount > 1 ? "s" : ""} req
      </span>
    );
  }
  if (pr.approvedCount > 0) {
    return (
      <span className={styles.chipSuccess}>
        {pr.approvedCount} approved
      </span>
    );
  }
  return null;
}

interface PRCardProps {
  pr: UserPR;
}

function PRCard({ pr }: PRCardProps) {
  const hasSession = pr.sessionIds.length > 0;

  return (
    <div className={styles.prCard} data-testid="github-pr-card">
      <div className={styles.prHeader}>
        <a
          className={styles.prTitle}
          href={pr.htmlUrl}
          target="_blank"
          rel="noreferrer"
          aria-label={`PR #${pr.number}: ${pr.title}`}
        >
          {pr.title}
        </a>
        <div className={styles.chips}>
          {prCheckChip(pr)}
          {prReviewChip(pr)}
        </div>
      </div>
      <div className={styles.prMeta}>
        <span className={styles.prRepo}>
          #{pr.number}
        </span>
        <span className={styles.prBranch}>
          {pr.headRef} → {pr.baseRef}
        </span>
        {pr.localWorktreePath && (
          <span className={styles.worktreeLink} title={pr.localWorktreePath}>
            {pr.localWorktreePath.split("/").slice(-2).join("/")}
          </span>
        )}
      </div>
      <div className={styles.prActions}>
        {hasSession ? (
          <Link
            href={`/?session=${encodeURIComponent(pr.sessionIds[0])}`}
            className={styles.openSessionButton}
            data-testid="open-session-button"
          >
            Open Session
          </Link>
        ) : (
          <Link
            href={`/?pr=${encodeURIComponent(pr.htmlUrl)}`}
            className={styles.createSessionButton}
            data-testid="create-session-button"
          >
            + Session
          </Link>
        )}
      </div>
    </div>
  );
}

// --- Stats bar ---

interface StatsBarProps {
  prs: UserPR[];
}

function StatsBar({ prs }: StatsBarProps) {
  const ciFailures = prs.filter(
    (p) => p.checkConclusion === "failure" || p.checkConclusion === "error"
  ).length;
  const needsReview = prs.filter((p) => p.changesReqCount > 0).length;
  const withSessions = prs.filter((p) => p.sessionIds.length > 0).length;

  return (
    <div className={styles.statsBar} data-testid="github-prs-stats">
      <span className={styles.statItem}>
        <span className={styles.statCount}>{prs.length}</span> open
      </span>
      {ciFailures > 0 && (
        <span className={styles.statItem}>
          <span className={styles.statCountError}>{ciFailures}</span> CI failing
        </span>
      )}
      {needsReview > 0 && (
        <span className={styles.statItem}>
          <span className={styles.statCountWarning}>{needsReview}</span> changes requested
        </span>
      )}
      {withSessions > 0 && (
        <span className={styles.statItem}>
          <span className={styles.statCount}>{withSessions}</span> with sessions
        </span>
      )}
    </div>
  );
}

// --- Accounts bar ---

interface AccountsBarProps {
  accounts: GitHubAccount[];
  onDisconnect: (username: string, host: string) => void;
  onAddAccount: () => void;
}

function AccountsBar({ accounts, onDisconnect, onAddAccount }: AccountsBarProps) {
  return (
    <div className={styles.accountsRow} data-testid="github-accounts-row">
      {accounts.map((acc) => (
        <span
          key={`${acc.host || "github.com"}:${acc.username}`}
          className={acc.isEnvToken ? styles.accountChipEnv : styles.accountChip}
          title={acc.isEnvToken ? "Sourced from environment variable" : undefined}
        >
          @{acc.username}
          {acc.host && acc.host !== "github.com" && (
            <span className={styles.hostBadge}>({acc.host})</span>
          )}
          {!acc.isEnvToken && (
            <button
              className={styles.disconnectAccountButton}
              onClick={() => onDisconnect(acc.username, acc.host)}
              aria-label={`Disconnect ${acc.username}`}
              title="Disconnect this account"
            >
              ×
            </button>
          )}
        </span>
      ))}
      <button
        className={styles.addAccountButton}
        onClick={onAddAccount}
        data-testid="add-github-account-button"
      >
        + Add account
      </button>
    </div>
  );
}

// --- Device Auth Banner ---

type DeviceFlowPhase =
  | { kind: "idle" }
  | { kind: "starting" }
  | { kind: "waiting"; userCode: string; verificationUri: string; deviceCode: string }
  | { kind: "polling"; userCode: string; verificationUri: string; deviceCode: string }
  | { kind: "complete" }
  | { kind: "expired" }
  | { kind: "error"; message: string };

interface DeviceAuthBannerProps {
  errorMessage: string;
  onAuthComplete: () => void;
  onCancel?: () => void;
}

function DeviceAuthBanner({ errorMessage, onAuthComplete, onCancel }: DeviceAuthBannerProps) {
  const client = useGitHubUserClient();
  const [flow, setFlow] = useState<DeviceFlowPhase>({ kind: "idle" });
  const [host, setHost] = useState("");
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef(false);

  useEffect(() => {
    abortRef.current = false;
    return () => {
      abortRef.current = true;
      if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    };
  }, []);

  const handleConnect = useCallback(async () => {
    setFlow({ kind: "starting" });
    try {
      const res = await client.startGitHubDeviceAuth(
        create(StartGitHubDeviceAuthRequestSchema, { host: host.trim() })
      );
      if (abortRef.current) return;
      setFlow({
        kind: "waiting",
        userCode: res.userCode,
        verificationUri: res.verificationUri,
        deviceCode: res.deviceCode,
      });
    } catch (err) {
      if (!abortRef.current) {
        setFlow({ kind: "error", message: String(err) });
      }
    }
  }, [client, host]);

  const schedulePoll = useCallback(
    (deviceCode: string, userCode: string, verificationUri: string, intervalMs: number) => {
      if (abortRef.current) return;
      pollTimerRef.current = setTimeout(async () => {
        if (abortRef.current) return;
        try {
          const res = await client.pollGitHubDeviceAuth(
            create(PollGitHubDeviceAuthRequestSchema, { deviceCode })
          );
          if (abortRef.current) return;
          if (res.status === DeviceAuthStatus.COMPLETE) {
            setFlow({ kind: "complete" });
            onAuthComplete();
          } else if (res.status === DeviceAuthStatus.EXPIRED) {
            setFlow({ kind: "expired" });
          } else if (res.status === DeviceAuthStatus.ERROR) {
            setFlow({ kind: "error", message: res.error || "Unknown error" });
          } else {
            setFlow({ kind: "polling", userCode, verificationUri, deviceCode });
            schedulePoll(deviceCode, userCode, verificationUri, intervalMs);
          }
        } catch (err) {
          if (!abortRef.current) {
            setFlow({ kind: "error", message: String(err) });
          }
        }
      }, intervalMs);
    },
    [client, onAuthComplete]
  );

  const handleStartPolling = useCallback(() => {
    if (flow.kind !== "waiting") return;
    const { deviceCode, userCode, verificationUri } = flow;
    setFlow({ kind: "polling", userCode, verificationUri, deviceCode });
    schedulePoll(deviceCode, userCode, verificationUri, 5000);
  }, [flow, schedulePoll]);

  const handleReset = useCallback(() => {
    if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    setFlow({ kind: "idle" });
    onCancel?.();
  }, [onCancel]);

  if (flow.kind === "idle") {
    return (
      <div className={styles.authBanner} data-testid="github-auth-banner">
        <span className={styles.authBannerText}>
          {errorMessage || "GitHub authentication not yet configured."}
        </span>
        <input
          className={styles.hostInput}
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="github.com"
          aria-label="GitHub host"
          data-testid="github-host-input"
        />
        <button
          className={styles.connectButton}
          onClick={handleConnect}
          data-testid="github-connect-button"
        >
          Connect GitHub
        </button>
      </div>
    );
  }

  if (flow.kind === "starting") {
    return (
      <div className={styles.authBanner} data-testid="github-auth-banner">
        <span className={styles.authBannerText}>Starting device auth…</span>
      </div>
    );
  }

  if (flow.kind === "waiting" || flow.kind === "polling") {
    const isPolling = flow.kind === "polling";
    return (
      <div className={styles.deviceFlowCard} data-testid="github-device-flow">
        <p className={styles.deviceFlowInstructions}>
          {isPolling
            ? "Waiting for authorization…"
            : "Open the link below and enter your code:"}
        </p>
        <div className={styles.deviceFlowRow}>
          <a
            href={flow.verificationUri}
            target="_blank"
            rel="noreferrer"
            className={styles.verificationLink}
            data-testid="github-verification-link"
          >
            {flow.verificationUri}
          </a>
          <span className={styles.userCode} data-testid="github-user-code">
            {flow.userCode}
          </span>
        </div>
        {!isPolling && (
          <button
            className={styles.connectButton}
            onClick={handleStartPolling}
            data-testid="github-authorized-button"
          >
            I&apos;ve authorized — continue
          </button>
        )}
        {isPolling && <span className={styles.pollingIndicator}>Checking…</span>}
        <button className={styles.cancelButton} onClick={handleReset}>
          Cancel
        </button>
      </div>
    );
  }

  if (flow.kind === "complete") {
    return (
      <div className={styles.authBannerSuccess} data-testid="github-auth-success">
        GitHub connected successfully.
      </div>
    );
  }

  if (flow.kind === "expired") {
    return (
      <div className={styles.authBanner} data-testid="github-auth-banner">
        <span className={styles.authBannerText}>Device code expired.</span>
        <button className={styles.connectButton} onClick={handleReset}>
          Try again
        </button>
      </div>
    );
  }

  return (
    <div className={styles.authBanner} data-testid="github-auth-banner">
      <span className={styles.authBannerText}>
        Auth error: {flow.kind === "error" ? flow.message : ""}
      </span>
      <button className={styles.connectButton} onClick={handleReset}>
        Try again
      </button>
    </div>
  );
}

// --- Add account panel (tab switcher) ---

type AddAccountMode = "device" | "token";

function AddAccountPanel({ errorMessage, onAuthComplete, onCancel }: DeviceAuthBannerProps) {
  const [mode, setMode] = useState<AddAccountMode>("device");
  const [cliAvailable, setCliAvailable] = useState(false);
  const [manualExpanded, setManualExpanded] = useState(false);
  const modeChosenRef = useRef(false);

  const handleCliAvailabilityChange = useCallback((available: boolean) => {
    setCliAvailable(available);
    // Once CLI credentials are importable, prefer the token tab over device flow —
    // there's no reason to walk through device flow when credentials are already local.
    if (available && !modeChosenRef.current) {
      setMode("token");
    }
  }, []);

  const showManualAuth = !cliAvailable || manualExpanded;

  return (
    <div className={styles.addAccountPanel} data-testid="github-add-account-panel">
      <CLIImportSection onAuthComplete={onAuthComplete} onAvailabilityChange={handleCliAvailabilityChange} />
      {cliAvailable && !manualExpanded && (
        <button
          type="button"
          className={styles.cancelButton}
          onClick={() => setManualExpanded(true)}
          data-testid="github-add-account-manual-toggle"
        >
          Set up manually instead
        </button>
      )}
      {showManualAuth && (
        <>
          <div className={styles.authTabs} role="tablist">
            <button
              role="tab"
              aria-selected={mode === "device"}
              className={mode === "device" ? styles.authTabActive : styles.authTab}
              onClick={() => {
                modeChosenRef.current = true;
                setMode("device");
              }}
              data-testid="github-auth-tab-device"
            >
              Device flow
            </button>
            <button
              role="tab"
              aria-selected={mode === "token"}
              className={mode === "token" ? styles.authTabActive : styles.authTab}
              onClick={() => {
                modeChosenRef.current = true;
                setMode("token");
              }}
              data-testid="github-auth-tab-token"
            >
              Personal access token
            </button>
          </div>
          {mode === "device" ? (
            <DeviceAuthBanner
              errorMessage={errorMessage}
              onAuthComplete={onAuthComplete}
              onCancel={onCancel}
            />
          ) : (
            <TokenAuthForm onAuthComplete={onAuthComplete} onCancel={onCancel} />
          )}
        </>
      )}
    </div>
  );
}

// --- gh CLI host import ---

type CLIImportState =
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "ready"; hosts: GitHubCLIHost[] }
  | { kind: "error"; message: string };

function CLIImportSection({
  onAuthComplete,
  onAvailabilityChange,
}: {
  onAuthComplete: () => void;
  onAvailabilityChange?: (available: boolean) => void;
}) {
  const client = useGitHubUserClient();
  const [state, setState] = useState<CLIImportState>({ kind: "loading" });
  const [importingHost, setImportingHost] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    client
      .listGitHubCLIHosts(create(ListGitHubCLIHostsRequestSchema, {}))
      .then((res) => {
        if (cancelled) return;
        if (!res.ghAvailable || res.hosts.length === 0) {
          setState({ kind: "unavailable" });
          onAvailabilityChange?.(false);
        } else {
          setState({ kind: "ready", hosts: res.hosts });
          onAvailabilityChange?.(true);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setState({ kind: "unavailable" });
          onAvailabilityChange?.(false);
        }
      });
    return () => {
      cancelled = true;
    };
    // onAvailabilityChange is expected to be a stable setState-style callback from the parent
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client]);

  const handleImport = useCallback(
    async (host: string) => {
      setImportingHost(host);
      try {
        await client.addGitHubAccountFromCLI(
          create(AddGitHubAccountFromCLIRequestSchema, { host })
        );
        onAuthComplete();
      } catch (err) {
        setState({ kind: "error", message: String(err) });
      } finally {
        setImportingHost(null);
      }
    },
    [client, onAuthComplete]
  );

  if (state.kind === "loading" || state.kind === "unavailable") {
    return null;
  }

  return (
    <div className={styles.cliImportSection} data-testid="github-cli-import-section">
      <span className={styles.cliImportLabel}>Already logged in via gh CLI:</span>
      {state.kind === "ready" && (
        <div className={styles.cliImportHostList}>
          {state.hosts.map((h) => (
            <button
              key={h.host}
              type="button"
              className={styles.cliImportHostButton}
              disabled={h.alreadyAdded || importingHost === h.host}
              onClick={() => handleImport(h.host)}
              data-testid={`github-cli-import-host-${h.host}`}
            >
              {h.alreadyAdded
                ? `${h.host}${h.username ? ` (${h.username})` : ""} — connected`
                : importingHost === h.host
                  ? `Importing ${h.host}…`
                  : `Import ${h.host}${h.username ? ` (${h.username})` : ""}`}
            </button>
          ))}
        </div>
      )}
      {state.kind === "error" && (
        <span className={styles.authError} data-testid="github-cli-import-error">
          {state.message}
        </span>
      )}
      <span className={styles.cliImportDivider}>or paste a token manually</span>
    </div>
  );
}

interface TokenAuthFormProps {
  onAuthComplete: () => void;
  onCancel?: () => void;
}

function TokenAuthForm({ onAuthComplete, onCancel }: TokenAuthFormProps) {
  const client = useGitHubUserClient();
  const [host, setHost] = useState("");
  const [token, setToken] = useState("");
  const [status, setStatus] = useState<{ kind: "idle" | "submitting" | "error"; message?: string }>({
    kind: "idle",
  });

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setStatus({ kind: "submitting" });
      try {
        await client.addGitHubAccountWithToken(
          create(AddGitHubAccountWithTokenRequestSchema, {
            host: host.trim(),
            token: token.trim(),
          })
        );
        onAuthComplete();
      } catch (err) {
        setStatus({ kind: "error", message: String(err) });
      }
    },
    [client, host, token, onAuthComplete]
  );

  return (
    <form
      className={styles.deviceFlowCard}
      onSubmit={handleSubmit}
      data-testid="github-token-auth-form"
    >
      <input
        className={styles.hostInput}
        value={host}
        onChange={(e) => setHost(e.target.value)}
        placeholder="github.com"
        aria-label="GitHub host"
        data-testid="github-token-host-input"
      />
      <input
        type="password"
        className={styles.hostInput}
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="ghp_… or a GHES personal access token"
        aria-label="Personal access token"
        data-testid="github-token-input"
      />
      <button
        type="submit"
        className={styles.connectButton}
        disabled={status.kind === "submitting" || !token.trim()}
        data-testid="github-token-submit-button"
      >
        {status.kind === "submitting" ? "Validating…" : "Connect with token"}
      </button>
      {onCancel && (
        <button type="button" className={styles.cancelButton} onClick={onCancel}>
          Cancel
        </button>
      )}
      {status.kind === "error" && (
        <span className={styles.authError} data-testid="github-token-auth-error">
          {status.message}
        </span>
      )}
    </form>
  );
}

// --- Filter / sort bar ---

type FilterStatus =
  | "all"
  | "ci-failing"
  | "changes-requested"
  | "with-session"
  | "draft";

type SortBy = "updated-desc" | "updated-asc" | "repo" | "ci-status";

const STATUS_FILTERS: { value: FilterStatus; label: string }[] = [
  { value: "all", label: "All" },
  { value: "ci-failing", label: "CI failing" },
  { value: "changes-requested", label: "Changes req" },
  { value: "with-session", label: "Has session" },
  { value: "draft", label: "Draft" },
];

interface FilterBarProps {
  filter: FilterStatus;
  sort: SortBy;
  search: string;
  onFilter: (f: FilterStatus) => void;
  onSort: (s: SortBy) => void;
  onSearch: (q: string) => void;
}

function FilterBar({ filter, sort, search, onFilter, onSort, onSearch }: FilterBarProps) {
  return (
    <div className={styles.filterBar} data-testid="github-prs-filter-bar">
      <div className={styles.filterChipGroup}>
        {STATUS_FILTERS.map((f) => (
          <button
            key={f.value}
            className={filter === f.value ? styles.filterChipActive : styles.filterChip}
            onClick={() => onFilter(f.value)}
            data-testid={`filter-chip-${f.value}`}
          >
            {f.label}
          </button>
        ))}
      </div>
      <div className={styles.sortGroup}>
        <input
          type="search"
          className={styles.searchInput}
          placeholder="Search PRs…"
          value={search}
          onChange={(e) => onSearch(e.target.value)}
          aria-label="Search pull requests"
          data-testid="github-prs-search"
        />
        <span className={styles.sortLabel}>Sort:</span>
        <select
          className={styles.sortSelect}
          value={sort}
          onChange={(e) => onSort(e.target.value as SortBy)}
          aria-label="Sort pull requests"
          data-testid="github-prs-sort"
        >
          <option value="updated-desc">Updated ↓</option>
          <option value="updated-asc">Updated ↑</option>
          <option value="repo">Repo A–Z</option>
          <option value="ci-status">CI status</option>
        </select>
      </div>
    </div>
  );
}

function applyFilterSort(
  prs: UserPR[],
  filter: FilterStatus,
  sort: SortBy,
  search: string
): UserPR[] {
  let result = prs;

  if (search.trim()) {
    const q = search.trim().toLowerCase();
    result = result.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.headRef.toLowerCase().includes(q) ||
        `${p.owner}/${p.repo}`.toLowerCase().includes(q) ||
        String(p.number).includes(q)
    );
  }

  switch (filter) {
    case "ci-failing":
      result = result.filter(
        (p) => p.checkConclusion === "failure" || p.checkConclusion === "error"
      );
      break;
    case "changes-requested":
      result = result.filter((p) => p.changesReqCount > 0);
      break;
    case "with-session":
      result = result.filter((p) => p.sessionIds.length > 0);
      break;
    case "draft":
      result = result.filter((p) => p.isDraft);
      break;
  }

  const sorted = [...result];
  switch (sort) {
    case "updated-desc":
      sorted.sort(
        (a, b) =>
          Number(b.updatedAt?.seconds ?? 0n) - Number(a.updatedAt?.seconds ?? 0n)
      );
      break;
    case "updated-asc":
      sorted.sort(
        (a, b) =>
          Number(a.updatedAt?.seconds ?? 0n) - Number(b.updatedAt?.seconds ?? 0n)
      );
      break;
    case "repo":
      sorted.sort((a, b) =>
        `${a.owner}/${a.repo}`.localeCompare(`${b.owner}/${b.repo}`)
      );
      break;
    case "ci-status": {
      const rank = (p: UserPR) => {
        if (p.checkConclusion === "failure" || p.checkConclusion === "error") return 0;
        if (p.changesReqCount > 0) return 1;
        if (p.checkConclusion === "success") return 3;
        return 2;
      };
      sorted.sort((a, b) => rank(a) - rank(b));
      break;
    }
  }
  return sorted;
}

// --- PR list grouped by owner/repo ---

interface PRGroupedListProps {
  prs: UserPR[];
}

function PRGroupedList({ prs }: PRGroupedListProps) {
  const groups = useMemo(() => {
    const map = new Map<string, UserPR[]>();
    for (const pr of prs) {
      const key = `${pr.owner}/${pr.repo}`;
      const group = map.get(key) ?? [];
      group.push(pr);
      map.set(key, group);
    }
    return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [prs]);

  if (groups.length === 0) return null;

  if (groups.length === 1) {
    return (
      <div className={styles.repoGroupSection}>
        {groups[0][1].map((pr) => (
          <PRCard key={`${pr.owner}/${pr.repo}#${pr.number}`} pr={pr} />
        ))}
      </div>
    );
  }

  return (
    <>
      {groups.map(([repoKey, repoPRs]) => (
        <div key={repoKey} className={styles.repoGroupSection}>
          <div className={styles.repoGroupHeader}>{repoKey}</div>
          {repoPRs.map((pr) => (
            <PRCard key={`${pr.owner}/${pr.repo}#${pr.number}`} pr={pr} />
          ))}
        </div>
      ))}
    </>
  );
}

// --- Main section ---

/**
 * Displays the authenticated GitHub user's open pull requests.
 * Shows connected accounts, aggregated stats, and PRs grouped by repo.
 */
export function GitHubPRsSection() {
  const { prs, authState, refresh } = useGitHubPRs();
  const client = useGitHubUserClient();
  const [isOpen, setIsOpen] = useState(true);
  const [addingAccount, setAddingAccount] = useState(false);
  const [filterStatus, setFilterStatus] = useState<FilterStatus>("all");
  const [sortBy, setSortBy] = useState<SortBy>("updated-desc");
  const [searchQuery, setSearchQuery] = useState("");

  const visiblePRs = useMemo(
    () => applyFilterSort(prs, filterStatus, sortBy, searchQuery),
    [prs, filterStatus, sortBy, searchQuery]
  );

  const toggleOpen = useCallback(() => setIsOpen((v) => !v), []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggleOpen();
    }
  };

  const handleDisconnect = useCallback(
    async (username: string, host: string) => {
      await client.revokeGitHubToken(
        create(RevokeGitHubTokenRequestSchema, { username, host })
      );
      refresh();
    },
    [client, refresh]
  );

  const handleAddAccount = useCallback(() => {
    setAddingAccount(true);
  }, []);

  const handleAddAccountComplete = useCallback(() => {
    setAddingAccount(false);
    refresh();
  }, [refresh]);

  const handleAddAccountCancel = useCallback(() => {
    setAddingAccount(false);
  }, []);

  const authUnavailable = authState && !authState.available;
  const accounts = authState?.accounts ?? [];

  return (
    <section className={styles.section} aria-label="GitHub Pull Requests">
      <div
        role="button"
        tabIndex={0}
        className={styles.sectionHeader}
        onClick={toggleOpen}
        onKeyDown={handleKeyDown}
        aria-expanded={isOpen}
        aria-controls="github-prs-list"
      >
        <span
          className={`${styles.chevron} ${isOpen ? styles.chevronExpanded : ""}`}
          aria-hidden="true"
        >
          ▶
        </span>
        <span className={styles.sectionTitle}>GitHub Pull Requests</span>
        <span className={styles.badge}>{prs.length}</span>
      </div>

      {isOpen && (
        <div id="github-prs-list">
          {authUnavailable && !addingAccount ? (
            <AddAccountPanel
              errorMessage={authState?.errorMessage ?? ""}
              onAuthComplete={refresh}
            />
          ) : (
            <>
              {accounts.length > 0 && (
                <AccountsBar
                  accounts={accounts}
                  onDisconnect={handleDisconnect}
                  onAddAccount={handleAddAccount}
                />
              )}

              {addingAccount && (
                <AddAccountPanel
                  errorMessage=""
                  onAuthComplete={handleAddAccountComplete}
                  onCancel={handleAddAccountCancel}
                />
              )}

              {prs.length === 0 && !addingAccount ? (
                <div className={styles.empty}>
                  {authState === undefined
                    ? "Connecting to GitHub…"
                    : "No open pull requests found."}
                </div>
              ) : prs.length > 0 ? (
                <>
                  <StatsBar prs={prs} />
                  <FilterBar
                    filter={filterStatus}
                    sort={sortBy}
                    search={searchQuery}
                    onFilter={setFilterStatus}
                    onSort={setSortBy}
                    onSearch={setSearchQuery}
                  />
                  {visiblePRs.length === 0 ? (
                    <div className={styles.empty}>No PRs match the current filter.</div>
                  ) : (
                    <PRGroupedList prs={visiblePRs} />
                  )}
                </>
              ) : null}
            </>
          )}
        </div>
      )}
    </section>
  );
}
