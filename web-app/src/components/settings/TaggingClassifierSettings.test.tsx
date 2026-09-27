/**
 * Tests for TaggingClassifierSettings component.
 *
 * Covers the model-hierarchy form: load renders stored values, fallbacks can
 * be added/removed (capped), save posts the hierarchy and reloads, and an
 * active env override surfaces the warning banner. Mirrors
 * JulesSettings.test.tsx's mocking pattern.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TaggingClassifierSettings } from "./TaggingClassifierSettings";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

jest.mock("./TaggingClassifierSettings.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") },
  );
});

const mockGetConfig = jest.fn();
const mockUpdateConfig = jest.fn();

const baseConfig = {
  model: "haiku",
  fallbackModels: [] as string[],
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetConfig.mockResolvedValue({ config: { ...baseConfig }, envOverrideActive: false });
  mockUpdateConfig.mockResolvedValue({ config: { ...baseConfig } });
  (createClient as jest.Mock).mockReturnValue({
    getTaggingClassifierConfig: mockGetConfig,
    updateTaggingClassifierConfig: mockUpdateConfig,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("TaggingClassifierSettings", () => {
  it("renders the stored model and fallback list on load", async () => {
    mockGetConfig.mockResolvedValue({
      config: { model: "sonnet", fallbackModels: ["proxy-free"] },
      envOverrideActive: false,
    });
    render(<TaggingClassifierSettings />);

    expect(await screen.findByDisplayValue("sonnet")).toBeInTheDocument();
    expect(await screen.findByText("proxy-free")).toBeInTheDocument();
  });

  it("shows the env-override warning when env wins over the form", async () => {
    mockGetConfig.mockResolvedValue({
      config: { model: "proxy-free", fallbackModels: [] },
      envOverrideActive: true,
    });
    render(<TaggingClassifierSettings />);

    expect(
      await screen.findByText(/Environment overrides are active/),
    ).toBeInTheDocument();
  });

  it("adds and removes fallback models, then saves the hierarchy", async () => {
    render(<TaggingClassifierSettings />);
    await screen.findByDisplayValue("haiku");

    fireEvent.change(screen.getByLabelText("Add a fallback model"), {
      target: { value: "proxy-free" },
    });
    fireEvent.click(screen.getByText("Add"));
    expect(await screen.findByText("proxy-free")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "Remove fallback model proxy-free" }),
    );
    await waitFor(() =>
      expect(screen.queryByText("proxy-free")).not.toBeInTheDocument(),
    );

    fireEvent.click(screen.getByText("Add"));
    // Empty input adds nothing.
    expect(screen.queryByRole("listitem")).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Add a fallback model"), {
      target: { value: "proxy-free" },
    });
    fireEvent.click(screen.getByText("Add"));
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() =>
      expect(mockUpdateConfig).toHaveBeenCalledWith({
        config: { model: "haiku", fallbackModels: ["proxy-free"] },
      }),
    );
    expect(await screen.findByText(/no restart needed/)).toBeInTheDocument();
  });

  it("surfaces save errors without losing the form", async () => {
    mockUpdateConfig.mockRejectedValueOnce(new Error("boom"));
    render(<TaggingClassifierSettings />);
    await screen.findByDisplayValue("haiku");

    fireEvent.click(screen.getByText("Save"));
    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
  });
});
