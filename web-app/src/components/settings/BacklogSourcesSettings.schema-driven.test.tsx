/**
 * Proves BacklogSourcesSettings' add-source form is genuinely schema-driven, not
 * hardcoded to owner/repo fields that happen to match both real plugins today.
 * Mocks ./backlogSourceSchemas with a synthetic schema pair whose fields differ
 * from each other and from owner/repo — a component that ignores the schema and
 * renders hardcoded fields would fail these assertions; the real PLUGIN_SCHEMAS
 * entries (github_issues/github_prs) are intentionally NOT used here since they
 * currently have identical field lists and couldn't distinguish this.
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

jest.mock("./backlogSourceSchemas", () => ({
  PLUGIN_SCHEMAS: [
    {
      id: "plugin_a",
      label: "Plugin A",
      fields: [{ key: "project_key", label: "Project Key", placeholder: "Project Key (e.g. ABC)" }],
      requiresToken: true,
      tokenLabel: "Plugin A token",
    },
    {
      id: "plugin_b",
      label: "Plugin B",
      fields: [{ key: "workspace_slug", label: "Workspace Slug", placeholder: "Workspace Slug (e.g. my-team)" }],
      requiresToken: false,
      tokenLabel: "",
    },
  ],
}));

const mockListItemSources = jest.fn();
const mockCreateItemSource = jest.fn();

beforeEach(() => {
  jest.clearAllMocks();
  mockListItemSources.mockResolvedValue({ sources: [] });
  mockCreateItemSource.mockResolvedValue({ source: null });
  (createClient as jest.Mock).mockReturnValue({
    listItemSources: mockListItemSources,
    createItemSource: mockCreateItemSource,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("BacklogSourcesSettings (schema-driven form, mocked schemas)", () => {
  it("renders the first mocked plugin's own field, not a hardcoded owner/repo pair", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("No sources configured.")).toBeInTheDocument());

    expect(screen.getByPlaceholderText("Project Key (e.g. ABC)")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/^Owner/)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/^Repo /)).not.toBeInTheDocument();
  });

  it("switches to the second mocked plugin's distinct field set and token requirement", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("No sources configured.")).toBeInTheDocument());

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "plugin_b" } });

    expect(screen.getByPlaceholderText("Workspace Slug (e.g. my-team)")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Project Key (e.g. ABC)")).not.toBeInTheDocument();
    // plugin_b sets requiresToken: false — no token input should render.
    expect(screen.queryByPlaceholderText(/token/i)).not.toBeInTheDocument();
  });

  it("does not send a leftover token when submitting a plugin that does not require one", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("No sources configured.")).toBeInTheDocument());

    // Type a token while on plugin_a (requiresToken: true)...
    fireEvent.change(screen.getByPlaceholderText("Plugin A token"), { target: { value: "leaked-secret" } });

    // ...then switch to plugin_b (requiresToken: false, no token field rendered).
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "plugin_b" } });
    expect(screen.queryByPlaceholderText("Plugin A token")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "My Workspace" },
    });
    fireEvent.change(screen.getByPlaceholderText("Workspace Slug (e.g. my-team)"), { target: { value: "my-team" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Source" }));

    await waitFor(() => expect(mockCreateItemSource).toHaveBeenCalled());
    expect(mockCreateItemSource).toHaveBeenCalledWith(
      expect.objectContaining({ pluginId: "plugin_b", token: "" })
    );
  });
});
