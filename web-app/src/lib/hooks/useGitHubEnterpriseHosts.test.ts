import { renderHook, waitFor, act } from "@testing-library/react";
import { useGitHubEnterpriseHosts } from "./useGitHubEnterpriseHosts";
import { createClient } from "@connectrpc/connect";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: () => ({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:0",
  createAuthInterceptor: () => (next: unknown) => next,
}));

const mockListGitHubAccounts = jest.fn();

(createClient as jest.Mock).mockReturnValue({ listGitHubAccounts: mockListGitHubAccounts });

beforeEach(() => {
  jest.clearAllMocks();
  mockListGitHubAccounts.mockResolvedValue({
    accounts: [],
    enterpriseHosts: ["github.example-corp.com"],
  });
});

describe("useGitHubEnterpriseHosts", () => {
  it("useGitHubEnterpriseHosts_should_PopulateHosts_When_FetchSucceeds", async () => {
    const { result } = renderHook(() => useGitHubEnterpriseHosts());
    await waitFor(() => expect(result.current.hosts).toEqual(["github.example-corp.com"]));
  });

  it("useGitHubEnterpriseHosts_should_ReturnEmptyHosts_When_RPCThrows", async () => {
    mockListGitHubAccounts.mockRejectedValue(new Error("network failure"));

    const { result } = renderHook(() => useGitHubEnterpriseHosts());
    await waitFor(() => expect(mockListGitHubAccounts).toHaveBeenCalledTimes(1));

    expect(result.current.hosts).toEqual([]);
  });

  describe("refetch", () => {
    it("useGitHubEnterpriseHosts_should_CallListGitHubAccountsTwice_When_RefetchCalled", async () => {
      const { result } = renderHook(() => useGitHubEnterpriseHosts());
      await waitFor(() => expect(mockListGitHubAccounts).toHaveBeenCalledTimes(1));

      act(() => {
        result.current.refetch();
      });

      await waitFor(() => expect(mockListGitHubAccounts).toHaveBeenCalledTimes(2));
    });

    it("useGitHubEnterpriseHosts_should_PickUpNewlyAddedHost_When_RefetchCalledAfterAccountConnected", async () => {
      // Regression test: an account connected mid-session (e.g. via
      // AddGitHubAccountFromCLI) must be reflected after a refetch — this hook
      // previously fetched only once at mount, so a host added after page load
      // silently never reached GitHubEnterpriseURLDetector for the rest of the
      // session (see OmnibarContext, which now refetches on omnibar open).
      const { result } = renderHook(() => useGitHubEnterpriseHosts());
      await waitFor(() => expect(result.current.hosts).toEqual(["github.example-corp.com"]));

      mockListGitHubAccounts.mockResolvedValueOnce({
        accounts: [],
        enterpriseHosts: ["github.example-corp.com", "github.other-example.com"],
      });

      act(() => {
        result.current.refetch();
      });

      await waitFor(() =>
        expect(result.current.hosts).toEqual(["github.example-corp.com", "github.other-example.com"])
      );
    });

    it("useGitHubEnterpriseHosts_should_IgnoreStaleFetch_When_RefetchFiresBeforePriorFetchResolves", async () => {
      let resolveFirst!: (v: unknown) => void;
      const firstResponse = new Promise((resolve) => {
        resolveFirst = resolve;
      });
      mockListGitHubAccounts.mockReturnValueOnce(firstResponse);
      mockListGitHubAccounts.mockResolvedValueOnce({
        accounts: [],
        enterpriseHosts: ["github.newer-example.com"],
      });

      const { result } = renderHook(() => useGitHubEnterpriseHosts());

      act(() => {
        result.current.refetch();
      });
      await waitFor(() => expect(result.current.hosts).toEqual(["github.newer-example.com"]));

      // The stale first fetch resolves after the newer one already landed.
      await act(async () => {
        resolveFirst({ accounts: [], enterpriseHosts: ["github.stale-example.com"] });
      });

      expect(result.current.hosts).toEqual(["github.newer-example.com"]);
    });
  });
});
