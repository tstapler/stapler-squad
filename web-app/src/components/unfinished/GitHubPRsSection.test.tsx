/**
 * Tests for GitHubPRsSection's "Add account" UX.
 *
 * Covers:
 *  1. Auth-unavailable state renders both tabs (device flow + personal access token)
 *  2. Switching to the token tab renders the token form
 *  3. Submitting a valid token calls addGitHubAccountWithToken and refreshes
 *  4. Submitting an invalid token shows the error message from the RPC
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act, within } from "@testing-library/react";
import { GitHubPRsSection, type GitHubPRsSectionProps } from "./GitHubPRsSection";
import { usePRListFilters } from "@/lib/hooks/usePRListFilters";
import { AccountPollState, AccountPollStatusSchema } from "@/gen/session/v1/github_user_pb";
import { UserPRSchema, type UserPR } from "@/gen/session/v1/types_pb";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost",
  createAuthInterceptor: () => jest.fn(),
}));

jest.mock("./GitHubPRsSection.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") }
  );
});

const mockAddGitHubAccountWithToken = jest.fn();
const mockRevokeGitHubToken = jest.fn();

(createClient as jest.Mock).mockReturnValue({
  addGitHubAccountWithToken: mockAddGitHubAccountWithToken,
  revokeGitHubToken: mockRevokeGitHubToken,
  listGitHubCLIHosts: jest.fn().mockResolvedValue({ ghAvailable: false, hosts: [] }),
  addGitHubAccountFromCLI: jest.fn(),
});

(createConnectTransport as jest.Mock).mockReturnValue({});

let mockAuthState: { available: boolean; errorMessage?: string; accounts: unknown[] } | undefined;
const mockRefresh = jest.fn();

const noopFilters = {
  filterStatus: "all" as const,
  sortBy: "attention-first" as const,
  searchQuery: "",
  setFilterStatus: jest.fn(),
  setSortBy: jest.fn(),
  setSearchQuery: jest.fn(),
  clear: jest.fn(),
  isActive: false,
  apply: (prs: unknown[]) => prs as never[],
};

function renderSection() {
  return render(
    <GitHubPRsSection
      prs={[]}
      authState={mockAuthState as never}
      accountStatuses={[]}
      lastUpdatedAt={undefined}
      error={undefined}
      refreshing={false}
      refresh={mockRefresh}
      filters={noopFilters}
      attention={{ count: 0, degraded: false }}
    />
  );
}

describe("GitHubPRsSection add-account UX", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAuthState = { available: false, errorMessage: "", accounts: [] };
  });

  it("renders both the device-flow and personal-access-token tabs when auth is unavailable", () => {
    renderSection();

    expect(screen.getByTestId("github-auth-tab-device")).toBeInTheDocument();
    expect(screen.getByTestId("github-auth-tab-token")).toBeInTheDocument();
  });

  it("switches to the token form when the token tab is clicked", () => {
    renderSection();

    fireEvent.click(screen.getByTestId("github-auth-tab-token"));

    expect(screen.getByTestId("github-token-auth-form")).toBeInTheDocument();
  });

  it("submits a token and completes auth on success", async () => {
    mockAddGitHubAccountWithToken.mockResolvedValueOnce({});
    renderSection();

    fireEvent.click(screen.getByTestId("github-auth-tab-token"));
    fireEvent.change(screen.getByTestId("github-token-host-input"), {
      target: { value: "github.netflix.net" },
    });
    fireEvent.change(screen.getByTestId("github-token-input"), {
      target: { value: "ghp_validtoken" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("github-token-submit-button"));
    });

    await waitFor(() => expect(mockAddGitHubAccountWithToken).toHaveBeenCalledTimes(1));
    expect(mockRefresh).toHaveBeenCalled();
  });

  it("shows an error message when the token is rejected", async () => {
    mockAddGitHubAccountWithToken.mockRejectedValueOnce(
      new Error("[unauthenticated] token was rejected — check the token and host")
    );
    renderSection();

    fireEvent.click(screen.getByTestId("github-auth-tab-token"));
    fireEvent.change(screen.getByTestId("github-token-input"), {
      target: { value: "bad-token" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("github-token-submit-button"));
    });

    await waitFor(() =>
      expect(screen.getByTestId("github-token-auth-error")).toHaveTextContent(
        "token was rejected"
      )
    );
  });
});


// --- PRs panel: filters, freshness, banners, empty states, focus ---

const CONNECTED = {
  available: true,
  errorMessage: "",
  accounts: [{ username: "alice", host: "github.com", isEnvToken: true }],
};

function makePR(number: number, over: MessageInitShape<typeof UserPRSchema> = {}): UserPR {
  return create(UserPRSchema, {
    owner: "acme",
    repo: "api",
    number,
    title: `PR ${number}`,
    htmlUrl: `https://github.com/acme/api/pull/${number}`,
    host: "github.com",
    accountLogin: "alice",
    checkConclusion: "success",
    detailsLoaded: true,
    unresolvedThreadCount: 0,
    hasMergeConflict: false,
    ...over,
  });
}

type PanelProps = Partial<Omit<GitHubPRsSectionProps, "filters">>;

function Panel(props: PanelProps) {
  const filters = usePRListFilters();
  return (
    <GitHubPRsSection
      prs={[makePR(1), makePR(2, { checkConclusion: "failure" })]}
      authState={CONNECTED as never}
      accountStatuses={[]}
      lastUpdatedAt={undefined}
      error={undefined}
      refreshing={false}
      refresh={mockRefresh}
      attention={{ count: 0, degraded: false }}
      {...props}
      filters={filters}
    />
  );
}

describe("GitHubPRsSection PRs panel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("gitHubPRsSection_should_ExposeExactFilterAndSortOptionSets_When_Rendered", () => {
    render(<Panel />);
    const group = screen.getByRole("group", { name: "Filter PRs" });
    expect(within(group).getAllByRole("button").map((b) => b.textContent)).toEqual([
      "All",
      "CI failing",
      "Changes req",
      "Has session",
      "Draft",
      "Needs attention",
    ]);
    const select = screen.getByLabelText("Sort") as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.textContent)).toEqual([
      "Updated down",
      "Updated up",
      "Repo A-Z",
      "CI status",
      "Attention first",
    ]);
    expect(select.value).toBe("attention-first");
  });

  it("gitHubPRsSection_should_RenderFilterAsAriaPressedToggleGroupAndSortAsLabelledSelect_When_Rendered", () => {
    const before = window.location.href;
    render(<Panel />);
    const group = screen.getByRole("group", { name: "Filter PRs" });
    const pressed = () => within(group).getAllByRole("button").filter((b) => b.getAttribute("aria-pressed") === "true");
    expect(pressed().map((b) => b.textContent)).toEqual(["All"]);

    fireEvent.click(screen.getByRole("button", { name: "CI failing" }));
    expect(pressed().map((b) => b.textContent)).toEqual(["CI failing"]);
    expect(screen.getAllByTestId("github-pr-card")).toHaveLength(1);

    fireEvent.change(screen.getByLabelText("Sort"), { target: { value: "repo" } });
    expect(window.location.href).toBe(before); // filter/sort are not in the URL
    expect(screen.getByLabelText("Sort").tagName).toBe("SELECT");
  });

  it("gitHubPRsSection_should_ShowSignInExpiredBannerDistinctFromConnectBanner_When_AccountUnauthorized", () => {
    const statuses = [
      create(AccountPollStatusSchema, { host: "ghe.corp", accountLogin: "bob", state: AccountPollState.UNAUTHORIZED }),
      create(AccountPollStatusSchema, { host: "github.com", accountLogin: "alice", state: AccountPollState.OK }),
    ];
    const { unmount } = render(<Panel accountStatuses={statuses} />);
    const banner = screen.getByTestId("github-account-banner");
    expect(banner).toHaveAttribute("role", "status");
    expect(banner).toHaveTextContent("GitHub sign-in expired for bob on ghe.corp. Reconnect");
    expect(screen.getAllByTestId("github-account-banner")).toHaveLength(1); // alice OK: no banner
    expect(screen.getAllByTestId("github-pr-card")).toHaveLength(2); // other accounts' cards render
    expect(screen.queryByText("Connect GitHub to see your open PRs")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Reconnect" }));
    expect(screen.getByTestId("github-add-account-panel")).toBeInTheDocument();
    unmount();

    // no accounts at all is the distinct not-connected state
    render(<Panel prs={[]} authState={{ available: false, errorMessage: "", accounts: [] } as never} />);
    expect(screen.getByText("Connect GitHub to see your open PRs")).toBeInTheDocument();
    expect(screen.queryByTestId("github-account-banner")).toBeNull();
  });

  it("gitHubPRsSection_should_ShowRateLimitedAndGenericErrorAccountBanners_When_AccountsFail", () => {
    const statuses = [
      create(AccountPollStatusSchema, { host: "ghe.corp", accountLogin: "bob", state: AccountPollState.RATE_LIMITED }),
      create(AccountPollStatusSchema, { host: "github.com", accountLogin: "alice", state: AccountPollState.ERROR, detail: "timeout" }),
    ];
    render(<Panel accountStatuses={statuses} />);
    const banners = screen.getAllByTestId("github-account-banner").map((b) => b.textContent);
    expect(banners).toEqual([
      "GitHub rate limit reached for bob on ghe.corp.",
      "Could not refresh alice on github.com: timeout",
    ]);
  });

  it("gitHubPRsSection_should_ShowRefreshBusyWithoutDroppingFocus_When_RefreshInFlight", () => {
    const { rerender } = render(<Panel lastUpdatedAt={Date.now()} />);
    const button = screen.getByTestId("github-prs-refresh");
    expect(button).toHaveTextContent("Refresh");
    expect(button).not.toHaveAttribute("aria-busy", "true");
    act(() => button.focus());
    fireEvent.click(button);
    expect(mockRefresh).toHaveBeenCalledTimes(1);

    rerender(<Panel lastUpdatedAt={Date.now()} refreshing />);
    const busy = screen.getByTestId("github-prs-refresh");
    expect(busy).toBe(button);
    expect(busy).toHaveAttribute("aria-busy", "true");
    expect(busy).toHaveAttribute("aria-disabled", "true");
    expect(busy).not.toBeDisabled();
    expect(busy).toHaveTextContent("Refreshing...");
    expect(document.activeElement).toBe(busy);
    fireEvent.click(busy);
    expect(mockRefresh).toHaveBeenCalledTimes(1); // ignored while in flight
  });

  describe("with fake timers", () => {
    beforeEach(() => jest.useFakeTimers({ now: new Date("2026-10-07T12:00:00Z") }));
    afterEach(() => jest.useRealTimers());

    it("gitHubPRsSection_should_TickUpdatedLabelEvery30sAndSayJustNowOnlyUnder60s_When_FakeTimersAdvance", () => {
      const { unmount } = render(<Panel lastUpdatedAt={Date.now()} />);
      const label = () => screen.getByTestId("github-prs-updated").textContent;
      expect(label()).toBe("Updated just now");
      act(() => void jest.advanceTimersByTime(30_000));
      expect(label()).toBe("Updated just now");
      act(() => void jest.advanceTimersByTime(30_000));
      expect(label()).toBe("Updated 1 min ago");
      act(() => void jest.advanceTimersByTime(120_000));
      expect(label()).toBe("Updated 3 min ago");

      unmount();
      expect(jest.getTimerCount()).toBe(0); // ticker cleared on unmount
    });
  });

  it("gitHubPRsSection_should_KeepListWithBannerAndRetry_When_ErrorWithPriorData", () => {
    const { unmount } = render(<Panel lastUpdatedAt={Date.now()} error="boom" />);
    expect(screen.getAllByTestId("github-pr-card")).toHaveLength(2);
    const banner = screen.getByTestId("github-prs-error");
    expect(banner).toHaveAttribute("role", "status");
    expect(banner).toHaveTextContent("Showing data from just now. Could not refresh: boom");
    fireEvent.click(within(banner).getByRole("button", { name: "Retry" }));
    expect(mockRefresh).toHaveBeenCalledTimes(1);
    unmount();

    render(<Panel prs={[]} error="offline" />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Could not load PRs: offline");
    expect(within(alert).getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("gitHubPRsSection_should_DistinguishNoOpenPRsFromNotConnectedFromNoMatches_When_EachState", () => {
    const { unmount } = render(<Panel prs={[]} lastUpdatedAt={Date.now()} />);
    expect(screen.getByText("No open PRs")).toBeInTheDocument();
    expect(screen.getAllByText(/Updated/)).toHaveLength(1); // header only; the empty state does not repeat it
    expect(screen.queryByText("Connect GitHub to see your open PRs")).toBeNull();
    unmount();

    render(<Panel />);
    fireEvent.change(screen.getByTestId("github-prs-search"), { target: { value: "zzz-no-match" } });
    expect(screen.getByText("No PRs match your filters")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("clear-filters-empty"));
    expect(screen.getAllByTestId("github-pr-card")).toHaveLength(2);
    expect((screen.getByTestId("github-prs-search") as HTMLInputElement).value).toBe("");
    expect(screen.getByLabelText("Sort")).toHaveValue("attention-first");
    expect(screen.queryByTestId("clear-filters")).toBeNull(); // only offered while something is active
  });

  it("gitHubPRsSection_should_MoveFocusToListHeadingAfterRetryAndSearchBoxAfterClearFilters_When_Used", () => {
    const { rerender } = render(<Panel lastUpdatedAt={Date.now()} error="boom" />);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    rerender(<Panel lastUpdatedAt={Date.now()} error={undefined} />);
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Open pull requests" }));
    expect(screen.getByRole("heading", { name: "Open pull requests" })).toHaveAttribute("tabindex", "-1");

    fireEvent.change(screen.getByTestId("github-prs-search"), { target: { value: "zzz-no-match" } });
    fireEvent.click(screen.getByTestId("clear-filters-empty"));
    expect(document.activeElement).toBe(screen.getByTestId("github-prs-search"));
  });
});
