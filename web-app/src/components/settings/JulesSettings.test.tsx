/**
 * Tests for JulesSettings component.
 *
 * Covers Story 3.1.1's three acceptance criteria per
 * project_plans/google-jules-integration/implementation/validation.md's
 * Epic 3.1 test table (masked key field, actionable Test connection
 * message, revocable acknowledged repos). Mirrors
 * SlackNotificationSettings.test.tsx's mocking pattern.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { JulesSettings } from "./JulesSettings";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

jest.mock("./JulesSettings.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") },
  );
});

const mockGetJulesConfig = jest.fn();
const mockUpdateJulesConfig = jest.fn();
const mockTestJulesConnection = jest.fn();
const mockConfirmEgressConsent = jest.fn();

const REPO_PATH = "/home/tstapler/code/github.com/tstapler/stapler-squad";

const baseConfig = {
  enabled: false,
  hasApiKey: false,
  egressAcknowledgedRepos: [] as string[],
  maxConcurrentJulesSessions: 2,
  maxJulesSessionsPerDay: 15,
  authReconnectRequired: false,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetJulesConfig.mockResolvedValue({ config: { ...baseConfig } });
  mockUpdateJulesConfig.mockResolvedValue({ config: { ...baseConfig } });
  mockTestJulesConnection.mockResolvedValue({ ok: true, message: "" });
  mockConfirmEgressConsent.mockResolvedValue({ egressAcknowledgedRepos: [] });
  (createClient as jest.Mock).mockReturnValue({
    getJulesConfig: mockGetJulesConfig,
    updateJulesConfig: mockUpdateJulesConfig,
    testJulesConnection: mockTestJulesConnection,
    confirmEgressConsent: mockConfirmEgressConsent,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("JulesSettings", () => {
  it("renders the key field empty with the stored-key placeholder and type=password, never the real key", async () => {
    mockGetJulesConfig.mockResolvedValue({
      config: { ...baseConfig, hasApiKey: true },
    });
    render(<JulesSettings />);

    const input = await screen.findByLabelText("API key");
    expect(input).toHaveAttribute("type", "password");
    expect(input).toHaveValue("");
    expect(input).toHaveAttribute(
      "placeholder",
      "Key stored — enter a new key to replace it",
    );
    // The DOM must contain no key characters anywhere.
    expect(document.body.innerHTML).not.toMatch(/AIza|sk-|api[_-]?key.{0,20}[A-Za-z0-9]{16,}/i);
  });

  it("shows the not-connected message naming the repo when Test connection targets an unregistered source", async () => {
    const notConnectedMessage =
      "tstapler/stapler-squad is not connected to Jules. Connect it at jules.google.com, then test again.";
    mockTestJulesConnection.mockResolvedValue({
      ok: false,
      message: notConnectedMessage,
    });
    render(<JulesSettings />);

    const repoInput = await screen.findByLabelText(/Test connection/);
    fireEvent.change(repoInput, { target: { value: REPO_PATH } });
    fireEvent.click(screen.getByRole("button", { name: "Test connection" }));

    const statusRegion = await screen.findByTestId(
      "jules-test-connection-result",
    );
    expect(statusRegion).toHaveAttribute("role", "status");
    expect(statusRegion.textContent).toContain("tstapler/stapler-squad");
    expect(statusRegion.textContent).toContain("jules.google.com");
    expect(mockTestJulesConnection).toHaveBeenCalledWith({
      repoPath: REPO_PATH,
    });
  });

  it("calls UpdateJulesConfig without the repo when Revoke is clicked", async () => {
    mockGetJulesConfig.mockResolvedValue({
      config: { ...baseConfig, egressAcknowledgedRepos: [REPO_PATH] },
    });
    render(<JulesSettings />);

    const revokeButton = await screen.findByRole("button", {
      name: "Revoke cloud-egress consent for tstapler/stapler-squad",
    });
    expect(
      screen.getByText("tstapler/stapler-squad"),
    ).toBeInTheDocument();

    fireEvent.click(revokeButton);

    await waitFor(() => {
      expect(mockUpdateJulesConfig).toHaveBeenCalled();
    });
    expect(
      screen.queryByRole("button", {
        name: "Revoke cloud-egress consent for tstapler/stapler-squad",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("tstapler/stapler-squad"),
    ).not.toBeInTheDocument();
  });
});
