/**
 * Tests for GlobalDefaultsForm component.
 *
 * Covers:
 *  1. Renders the Max Concurrent Backlog Work Items input with the loaded value
 *  2. Submitting includes maxConcurrentBacklogWorkItems in the update payload
 *  3. Editing the field updates the submitted payload
 *  4. Stale session threshold/notify fields load, save, and survive a failed save
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { GlobalDefaultsForm } from "./GlobalDefaultsForm";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

jest.mock("./GlobalDefaultsForm.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") }
  );
});

const mockGetSessionDefaults = jest.fn();
const mockUpdateGlobalDefaults = jest.fn();

const sampleDefaults = {
  program: "claude",
  oneOffBaseDir: "~/oneoff",
  newProjectBaseDir: "~/Projects",
  tags: [],
  envVars: {},
  cliFlags: "",
  maxAutoReworkIterations: 3,
  maxConcurrentBacklogWorkItems: 2,
  staleSessionThresholdMinutes: 30,
  staleSessionNotifyEnabled: true,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetSessionDefaults.mockResolvedValue({ defaults: sampleDefaults });
  mockUpdateGlobalDefaults.mockResolvedValue({ defaults: sampleDefaults });
  (createClient as jest.Mock).mockReturnValue({
    getSessionDefaults: mockGetSessionDefaults,
    updateGlobalDefaults: mockUpdateGlobalDefaults,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("GlobalDefaultsForm", () => {
  it("renders the Max Concurrent Backlog Work Items input with the loaded value", async () => {
    render(<GlobalDefaultsForm />);
    const input = await screen.findByLabelText("Max Concurrent Backlog Work Items");
    expect(input).toHaveValue(2);
  });

  it("submits the loaded maxConcurrentBacklogWorkItems value unchanged", async () => {
    render(<GlobalDefaultsForm />);
    await screen.findByLabelText("Max Concurrent Backlog Work Items");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateGlobalDefaults).toHaveBeenCalledWith(
        expect.objectContaining({ maxConcurrentBacklogWorkItems: 2 })
      );
    });
  });

  it("submits an edited maxConcurrentBacklogWorkItems value", async () => {
    render(<GlobalDefaultsForm />);
    const input = await screen.findByLabelText("Max Concurrent Backlog Work Items");

    fireEvent.change(input, { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateGlobalDefaults).toHaveBeenCalledWith(
        expect.objectContaining({ maxConcurrentBacklogWorkItems: 5 })
      );
    });
  });

  it("pre-fills the stale session threshold and notify controls from loaded defaults", async () => {
    render(<GlobalDefaultsForm />);

    const thresholdInput = await screen.findByTestId("stale-session-threshold-input");
    expect(thresholdInput).toHaveValue(30);

    const notifyCheckbox = screen.getByTestId("stale-session-notify-checkbox");
    expect(notifyCheckbox).toBeChecked();
  });

  it("submits edited stale session threshold and notify values on save", async () => {
    render(<GlobalDefaultsForm />);

    const thresholdInput = await screen.findByTestId("stale-session-threshold-input");
    const notifyCheckbox = screen.getByTestId("stale-session-notify-checkbox");

    fireEvent.change(thresholdInput, { target: { value: "45" } });
    fireEvent.click(notifyCheckbox);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateGlobalDefaults).toHaveBeenCalledWith(
        expect.objectContaining({
          staleSessionThresholdMinutes: 45,
          staleSessionNotifyEnabled: false,
        })
      );
    });
  });

  it("preserves typed stale session values and keeps Save clickable when the save fails", async () => {
    mockUpdateGlobalDefaults.mockRejectedValue(new Error("network error"));

    render(<GlobalDefaultsForm />);

    const thresholdInput = await screen.findByTestId("stale-session-threshold-input");
    const notifyCheckbox = screen.getByTestId("stale-session-notify-checkbox");
    const saveButton = screen.getByRole("button", { name: "Save" });

    fireEvent.change(thresholdInput, { target: { value: "60" } });
    fireEvent.click(notifyCheckbox);
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockUpdateGlobalDefaults).toHaveBeenCalled();
    });

    expect(thresholdInput).toHaveValue(60);
    expect(notifyCheckbox).not.toBeChecked();
    expect(saveButton).not.toBeDisabled();
  });
});
