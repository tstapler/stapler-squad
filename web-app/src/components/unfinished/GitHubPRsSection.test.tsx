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
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { GitHubPRsSection } from "./GitHubPRsSection";
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
});

(createConnectTransport as jest.Mock).mockReturnValue({});

let mockAuthState: { available: boolean; errorMessage?: string; accounts: unknown[] } | undefined;
const mockRefresh = jest.fn();

jest.mock("@/lib/hooks/useGitHubPRs", () => ({
  useGitHubPRs: () => ({
    prs: [],
    authState: mockAuthState,
    refresh: mockRefresh,
  }),
}));

describe("GitHubPRsSection add-account UX", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAuthState = { available: false, errorMessage: "", accounts: [] };
  });

  it("renders both the device-flow and personal-access-token tabs when auth is unavailable", () => {
    render(<GitHubPRsSection />);

    expect(screen.getByTestId("github-auth-tab-device")).toBeInTheDocument();
    expect(screen.getByTestId("github-auth-tab-token")).toBeInTheDocument();
  });

  it("switches to the token form when the token tab is clicked", () => {
    render(<GitHubPRsSection />);

    fireEvent.click(screen.getByTestId("github-auth-tab-token"));

    expect(screen.getByTestId("github-token-auth-form")).toBeInTheDocument();
  });

  it("submits a token and completes auth on success", async () => {
    mockAddGitHubAccountWithToken.mockResolvedValueOnce({});
    render(<GitHubPRsSection />);

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
    render(<GitHubPRsSection />);

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
