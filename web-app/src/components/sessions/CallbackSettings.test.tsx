/**
 * Tests for CallbackSettings (webhook-triggers Epic 5.1/7.3).
 *
 * Covers: masked-configured-state rendering (never shows the real URL), edit +
 * save wiring, and clear-to-disable.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { CallbackSettings } from "./CallbackSettings";
import { CallbackConfigProto } from "@/gen/session/v1/session_pb";

let mockConfig: Partial<CallbackConfigProto> | null = null;
let mockLoading = false;
const mockUpdateConfig = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/hooks/useCallbackConfig", () => ({
  useCallbackConfig: () => ({
    config: mockConfig,
    loading: mockLoading,
    error: null,
    updateConfig: mockUpdateConfig,
    refresh: jest.fn(),
  }),
}));

describe("CallbackSettings", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockConfig = {
      onSessionCompleteConfigured: true,
      onSessionStaleConfigured: false,
      onQueueItemCreatedConfigured: false,
    };
    mockLoading = false;
  });

  it("CallbackSettings_should_showConfiguredBadge_When_urlIsSet", () => {
    render(<CallbackSettings />);
    const row = screen.getByTestId("callback-on-session-complete").closest("div")!.parentElement!;
    expect(row).toHaveTextContent("Configured");
  });

  it("CallbackSettings_should_neverDisplayTheActualUrl", () => {
    render(<CallbackSettings />);
    // The real URL is never fetched/returned by GetCallbackConfig — only booleans.
    // Assert no value in any URL input reveals a URL-shaped string.
    for (const testId of [
      "callback-on-session-complete",
      "callback-on-session-stale",
      "callback-on-queue-item-created",
    ]) {
      expect(screen.getByTestId(testId)).toHaveValue("");
    }
  });

  it("CallbackSettings_should_disableSave_When_noFieldsEdited", () => {
    render(<CallbackSettings />);
    expect(screen.getByTestId("callback-settings-save")).toBeDisabled();
  });

  it("CallbackSettings_should_callUpdateConfigWithOnlyEditedField_When_saved", async () => {
    render(<CallbackSettings />);
    fireEvent.change(screen.getByTestId("callback-on-session-stale"), {
      target: { value: "https://example.com/stale" },
    });
    expect(screen.getByTestId("callback-settings-save")).not.toBeDisabled();

    fireEvent.click(screen.getByTestId("callback-settings-save"));

    await waitFor(() => expect(mockUpdateConfig).toHaveBeenCalledTimes(1));
    expect(mockUpdateConfig).toHaveBeenCalledWith({ onSessionStaleUrl: "https://example.com/stale" });
  });

  it("CallbackSettings_should_sendEmptyString_When_clearButtonClicked", async () => {
    render(<CallbackSettings />);
    fireEvent.click(screen.getByTestId("callback-on-session-complete-clear"));
    fireEvent.click(screen.getByTestId("callback-settings-save"));

    await waitFor(() => expect(mockUpdateConfig).toHaveBeenCalledTimes(1));
    expect(mockUpdateConfig).toHaveBeenCalledWith({ onSessionCompleteUrl: "" });
  });

  // ─── aria-live save announcement (Fix 3) ──────────────────────────────────

  it("CallbackSettings_should_announceSaveSuccess_When_saved", async () => {
    render(<CallbackSettings />);
    fireEvent.change(screen.getByTestId("callback-on-session-stale"), {
      target: { value: "https://example.com/stale" },
    });
    fireEvent.click(screen.getByTestId("callback-settings-save"));

    await waitFor(() => {
      const liveRegion = document.querySelector('[aria-live="polite"]');
      expect(liveRegion).toHaveTextContent("Callback settings saved.");
    });
  });

  it("CallbackSettings_should_announceSaveFailure_When_saveRejects", async () => {
    mockUpdateConfig.mockRejectedValueOnce(new Error("network error"));
    render(<CallbackSettings />);
    fireEvent.change(screen.getByTestId("callback-on-session-stale"), {
      target: { value: "https://example.com/stale" },
    });
    fireEvent.click(screen.getByTestId("callback-settings-save"));

    await waitFor(() => {
      const liveRegion = document.querySelector('[aria-live="polite"]');
      expect(liveRegion).toHaveTextContent(/Failed to save callback settings: network error/);
    });
  });
});
