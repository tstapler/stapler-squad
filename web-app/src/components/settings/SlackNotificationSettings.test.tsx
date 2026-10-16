/**
 * Tests for SlackNotificationSettings component.
 *
 * Covers the security/trust-relevant behaviors (Story 1.4.3 AC1/AC3) plus the
 * UX accessibility acceptance criteria from
 * project_plans/slack-review-notifications/implementation/validation.md's UX
 * table (UX-11 through UX-18).
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SlackNotificationSettings } from "./SlackNotificationSettings";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

jest.mock("./SlackNotificationSettings.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") },
  );
});

const mockGetSlackConfig = jest.fn();
const mockUpdateSlackConfig = jest.fn();
const mockTestSlackWebhook = jest.fn();

const configuredResponse = {
  config: {
    webhookConfigured: true,
    signingSecretConfigured: false,
    notifyOnQueueItem: true,
    queueDepthThreshold: 5,
    approvalEnabled: false,
    dashboardBaseUrl: "https://dashboard.example.com",
    lastDelivery: {
      attempted: true,
      success: true,
      error: "",
      attemptedAt: undefined,
    },
  },
};

const unconfiguredResponse = {
  config: {
    webhookConfigured: false,
    signingSecretConfigured: false,
    notifyOnQueueItem: false,
    queueDepthThreshold: 0,
    approvalEnabled: false,
    dashboardBaseUrl: "",
    lastDelivery: undefined,
  },
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetSlackConfig.mockResolvedValue(unconfiguredResponse);
  mockUpdateSlackConfig.mockResolvedValue(unconfiguredResponse);
  mockTestSlackWebhook.mockResolvedValue({ success: true, error: "" });
  (createClient as jest.Mock).mockReturnValue({
    getSlackConfig: mockGetSlackConfig,
    updateSlackConfig: mockUpdateSlackConfig,
    testSlackWebhook: mockTestSlackWebhook,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("SlackNotificationSettings", () => {
  it("SlackNotificationSettings_should_MaskWebhookField_When_AlreadyConfigured", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    render(<SlackNotificationSettings />);

    const input = await screen.findByLabelText("Webhook URL");
    // Masked: the input never shows a real value, only a "configured" placeholder.
    expect(input).toHaveValue("");
    expect(input).toHaveAttribute(
      "placeholder",
      expect.stringContaining("configured"),
    );
  });

  it("SlackNotificationSettings_should_DisableToggles_When_NoWebhookConfigured", async () => {
    mockGetSlackConfig.mockResolvedValue(unconfiguredResponse);
    render(<SlackNotificationSettings />);

    const checkbox = await screen.findByLabelText(
      "Notify on new review-queue item",
    );
    expect(checkbox).toBeDisabled();

    const approvalCheckbox = screen.getByLabelText(
      "Allow Approve/Deny from Slack",
    );
    expect(approvalCheckbox).toBeDisabled();
  });

  it("SlackNotificationSettings_should_ShowEdgeTriggeredDigestHint_NextToThresholdInput", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    render(<SlackNotificationSettings />);

    await screen.findByLabelText("Webhook URL");
    expect(screen.getByText(/one digest per burst/i)).toBeInTheDocument();
  });

  it("SlackNotificationSettings_should_HaveExplicitLabelForEveryInput", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    render(<SlackNotificationSettings />);
    await screen.findByLabelText("Webhook URL");

    expect(screen.getByLabelText("Webhook URL")).toBeInTheDocument();
    expect(
      screen.getByLabelText("Notify on new review-queue item"),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText(/Queue-depth digest threshold/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Dashboard URL")).toBeInTheDocument();
  });

  it("SlackNotificationSettings_should_SetAriaInvalidAndDescribedBy_When_WebhookUrlInvalid", async () => {
    mockGetSlackConfig.mockResolvedValue(unconfiguredResponse);
    render(<SlackNotificationSettings />);

    const input = await screen.findByLabelText("Webhook URL");
    fireEvent.change(input, { target: { value: "not-a-url" } });
    fireEvent.blur(input);

    await waitFor(() => {
      expect(input).toHaveAttribute("aria-invalid", "true");
    });
    const describedBy = input.getAttribute("aria-describedby") ?? "";
    expect(describedBy).toContain("slack-webhook-hint");
    expect(describedBy).toContain("slack-webhook-error");
  });

  it("SlackNotificationSettings_should_UseRoleAlertForBlockingErrors_And_RoleStatusForInfoState", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    mockTestSlackWebhook.mockResolvedValue({
      success: false,
      error: "slack returned 404: no_service",
    });
    render(<SlackNotificationSettings />);

    await screen.findByLabelText("Webhook URL");
    fireEvent.click(screen.getByRole("button", { name: "Send test message" }));

    const alertRegion = await screen.findByTestId("slack-test-webhook-result");
    expect(alertRegion).toHaveAttribute("role", "alert");
    expect(alertRegion.textContent).toContain("no_service");
  });

  it("renders success test result with role=status", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    mockTestSlackWebhook.mockResolvedValue({ success: true, error: "" });
    render(<SlackNotificationSettings />);

    await screen.findByLabelText("Webhook URL");
    fireEvent.click(screen.getByRole("button", { name: "Send test message" }));

    const statusRegion = await screen.findByTestId("slack-test-webhook-result");
    expect(statusRegion).toHaveAttribute("role", "status");
  });

  it("SlackNotificationSettings_should_UseNativeCheckboxInputsWithBoundLabels", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    render(<SlackNotificationSettings />);
    await screen.findByLabelText("Webhook URL");

    const notifyCheckbox = screen.getByLabelText(
      "Notify on new review-queue item",
    );
    expect(notifyCheckbox.tagName).toBe("INPUT");
    expect(notifyCheckbox).toHaveAttribute("type", "checkbox");
  });

  it("SlackNotificationSettings_should_UpdateResultRegionOnlyAfterRequestCompletes_NotOnClick", async () => {
    mockGetSlackConfig.mockResolvedValue(configuredResponse);
    let resolveTest: (v: {
      success: boolean;
      error: string;
    }) => void = () => {};
    mockTestSlackWebhook.mockReturnValue(
      new Promise((resolve) => {
        resolveTest = resolve;
      }),
    );
    render(<SlackNotificationSettings />);
    await screen.findByLabelText("Webhook URL");

    const button = screen.getByRole("button", { name: "Send test message" });
    fireEvent.click(button);

    // Immediately after click: button disabled/label changed, but no result yet.
    expect(button).toBeDisabled();
    expect(
      screen.queryByTestId("slack-test-webhook-result"),
    ).not.toBeInTheDocument();

    resolveTest({ success: true, error: "" });

    await waitFor(() => {
      expect(
        screen.getByTestId("slack-test-webhook-result"),
      ).toBeInTheDocument();
    });
  });
});
