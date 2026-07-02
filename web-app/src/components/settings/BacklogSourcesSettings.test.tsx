/**
 * Tests for BacklogSourcesSettings component.
 *
 * Covers:
 *  1. Renders source list after load
 *  2. Shows empty state when no sources
 *  3. Add source form calls createItemSource with correct payload
 *  4. Add button disabled until required fields are filled
 *  5. Toggle enabled calls updateItemSource
 *  6. Delete calls deleteItemSource
 *  7. Sync now calls triggerSync
 *  8. View history calls getSyncHistory and renders events
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogSourcesSettings } from "./BacklogSourcesSettings";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost", createAuthInterceptor: () => (x: unknown) => x }));

jest.mock("./BacklogSourcesSettings.css", () => {
  return new Proxy({}, { get: (_target, prop) => (typeof prop === "string" ? prop : "") });
});

const mockListItemSources = jest.fn();
const mockCreateItemSource = jest.fn();
const mockUpdateItemSource = jest.fn();
const mockDeleteItemSource = jest.fn();
const mockTriggerSync = jest.fn();
const mockGetSyncHistory = jest.fn();

const sampleSource = {
  id: "src-1",
  pluginId: "github_issues",
  displayName: "Acme Issues",
  enabled: true,
  tokenConfigured: true,
  lastSyncedAt: undefined,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockListItemSources.mockResolvedValue({ sources: [sampleSource] });
  mockCreateItemSource.mockResolvedValue({ source: sampleSource });
  mockUpdateItemSource.mockResolvedValue({ source: { ...sampleSource, enabled: false } });
  mockDeleteItemSource.mockResolvedValue({});
  mockTriggerSync.mockResolvedValue({});
  mockGetSyncHistory.mockResolvedValue({
    events: [
      {
        id: "ev-1",
        startedAt: undefined,
        finishedAt: undefined,
        itemsCreated: 2,
        itemsUpdated: 1,
        itemsSkipped: 0,
        itemsErrored: 0,
        errorMessage: "",
      },
    ],
  });

  (createClient as jest.Mock).mockReturnValue({
    listItemSources: mockListItemSources,
    createItemSource: mockCreateItemSource,
    updateItemSource: mockUpdateItemSource,
    deleteItemSource: mockDeleteItemSource,
    triggerSync: mockTriggerSync,
    getSyncHistory: mockGetSyncHistory,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("BacklogSourcesSettings", () => {
  it("renders source list after load", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => {
      expect(screen.getByText("Acme Issues")).toBeInTheDocument();
    });
    expect(screen.getByText("github_issues")).toBeInTheDocument();
  });

  it("shows empty state when no sources", async () => {
    mockListItemSources.mockResolvedValue({ sources: [] });
    render(<BacklogSourcesSettings />);
    await waitFor(() => {
      expect(screen.getByText("No sources configured.")).toBeInTheDocument();
    });
  });

  it("add button is disabled until required fields are filled", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const addButton = screen.getByRole("button", { name: "Add Source" });
    expect(addButton).toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });
    fireEvent.change(screen.getByPlaceholderText("GitHub personal access token"), { target: { value: "tok123" } });

    expect(addButton).not.toBeDisabled();
  });

  it("calls createItemSource with the correct payload on submit", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });
    fireEvent.change(screen.getByPlaceholderText("GitHub personal access token"), { target: { value: "tok123" } });

    fireEvent.click(screen.getByRole("button", { name: "Add Source" }));

    await waitFor(() => {
      expect(mockCreateItemSource).toHaveBeenCalledWith({
        pluginId: "github_issues",
        displayName: "Widgets Issues",
        configJson: JSON.stringify({ owner: "acme", repo: "widgets" }),
        token: "tok123",
      });
    });
  });

  it("shows an error message when createItemSource fails", async () => {
    mockCreateItemSource.mockRejectedValue(new Error("token invalid"));
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });
    fireEvent.change(screen.getByPlaceholderText("GitHub personal access token"), { target: { value: "tok123" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Source" }));

    expect(await screen.findByText("token invalid")).toBeInTheDocument();
  });

  it("toggles enabled state via updateItemSource", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: "Disable Acme Issues" }));

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith({
        sourceId: "src-1",
        displayName: "Acme Issues",
        enabled: false,
        token: "",
      });
    });
  });

  it("calls deleteItemSource when remove is clicked", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Remove Acme Issues" }));

    await waitFor(() => {
      expect(mockDeleteItemSource).toHaveBeenCalledWith({ sourceId: "src-1" });
    });
  });

  it("calls triggerSync when Sync now is clicked", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Sync now" }));

    await waitFor(() => {
      expect(mockTriggerSync).toHaveBeenCalledWith({ sourceId: "src-1" });
    });
  });

  it("fetches and renders sync history on View history click", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "View history" }));

    await waitFor(() => {
      expect(mockGetSyncHistory).toHaveBeenCalledWith({ sourceId: "src-1" });
    });
    expect(await screen.findByText(/created 2, updated 1, skipped 0/)).toBeInTheDocument();
  });
});
