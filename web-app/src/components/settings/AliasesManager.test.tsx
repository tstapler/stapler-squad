/**
 * Tests for AliasesManager component.
 *
 * Covers:
 *  1. Renders alias list after load
 *  2. Shows empty state when no aliases
 *  3. Opens form on "New Alias" click
 *  4. Calls upsertAlias with correct payload in create mode
 *  5. Edit flow — pre-populates form, name is disabled
 *  6. Name validation — empty name shows inline error
 *  7. Name validation — invalid format shows regex error
 *  8. Name validation — collision in create mode shows error
 *  9. Name validation — case-insensitive collision detected
 * 10. Edit mode skips uniqueness check for own name
 * 11. Delete flow — click Delete shows "Confirm delete?" button
 * 12. Delete flow — auto-cancel after 3 seconds
 * 13. Live @name preview
 * 14. Env var add/remove
 * 15. Advanced section expand/collapse
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { AliasesManager } from "./AliasesManager";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

// Mock vanilla-extract CSS modules to return empty strings
jest.mock("./AliasesManager.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") }
  );
});

const mockListAliases = jest.fn();
const mockUpsertAlias = jest.fn();
const mockDeleteAlias = jest.fn();

(createClient as jest.Mock).mockReturnValue({
  listAliases: mockListAliases,
  upsertAlias: mockUpsertAlias,
  deleteAlias: mockDeleteAlias,
});

(createConnectTransport as jest.Mock).mockReturnValue({});

const sampleAlias = {
  name: "myproj",
  description: "My project",
  group: "work",
  path: "~/code/myproject",
  program: "claude",
  autoYes: false,
  tags: ["backend"],
  envVars: { FOO: "bar" },
  cliFlags: "",
};

beforeEach(() => {
  jest.clearAllMocks();
  mockListAliases.mockResolvedValue({ aliases: [sampleAlias] });
  mockUpsertAlias.mockResolvedValue({});
  mockDeleteAlias.mockResolvedValue({});
  (createClient as jest.Mock).mockReturnValue({
    listAliases: mockListAliases,
    upsertAlias: mockUpsertAlias,
    deleteAlias: mockDeleteAlias,
  });
});

describe("AliasesManager", () => {
  it("renders alias list after load", async () => {
    render(<AliasesManager />);
    await waitFor(() => {
      expect(screen.getByText("@myproj")).toBeInTheDocument();
    });
    expect(screen.getByText("My project")).toBeInTheDocument();
    expect(screen.getByText("Group: work")).toBeInTheDocument();
    expect(screen.getByText("Path: ~/code/myproject")).toBeInTheDocument();
    expect(screen.getByText("Program: claude")).toBeInTheDocument();
  });

  it("shows empty state when no aliases", async () => {
    mockListAliases.mockResolvedValueOnce({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => {
      expect(screen.getByText("No aliases configured.")).toBeInTheDocument();
    });
  });

  it("opens form on 'New Alias' click", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));
    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));
    expect(screen.getByRole("heading", { name: "New Alias" })).toBeInTheDocument();
    expect(screen.getByLabelText(/Name \*/)).toBeInTheDocument();
  });

  it("calls upsertAlias with correct payload in create mode", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));

    fireEvent.change(screen.getByLabelText(/Name \*/), { target: { value: "newproj" } });
    fireEvent.change(screen.getByLabelText(/Description/), { target: { value: "New project" } });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpsertAlias).toHaveBeenCalledWith(
        expect.objectContaining({
          alias: expect.objectContaining({
            name: "newproj",
            description: "New project",
          }),
        })
      );
    });
    // After save, form is hidden and list is reloaded
    expect(mockListAliases).toHaveBeenCalledTimes(2);
  });

  it("edit flow — pre-populates form and name is disabled", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    const nameInput = screen.getByLabelText(/Name \*/) as HTMLInputElement;
    expect(nameInput.value).toBe("myproj");
    expect(nameInput.disabled).toBe(true);

    const descInput = screen.getByLabelText(/Description/) as HTMLInputElement;
    expect(descInput.value).toBe("My project");
  });

  it("calls upsertAlias with updated description on edit save", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    const descInput = screen.getByLabelText(/Description/);
    fireEvent.change(descInput, { target: { value: "Updated description" } });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpsertAlias).toHaveBeenCalledWith(
        expect.objectContaining({
          alias: expect.objectContaining({
            name: "myproj",
            description: "Updated description",
          }),
        })
      );
    });
    // After save, form is hidden and list is reloaded
    expect(mockListAliases).toHaveBeenCalledTimes(2);
  });

  it("name validation — empty name shows inline error", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Name is required.")).toBeInTheDocument();
    expect(mockUpsertAlias).not.toHaveBeenCalled();
  });

  it("name validation — invalid format shows regex error", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));
    fireEvent.change(screen.getByLabelText(/Name \*/), { target: { value: "my project" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(
      screen.getByText("Name may only contain letters, digits, hyphens, and underscores.")
    ).toBeInTheDocument();
    expect(mockUpsertAlias).not.toHaveBeenCalled();
  });

  it("name validation — collision in create mode shows error", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));
    fireEvent.change(screen.getByLabelText(/Name \*/), { target: { value: "myproj" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(
      screen.getByText(/already exists/)
    ).toBeInTheDocument();
    expect(mockUpsertAlias).not.toHaveBeenCalled();
  });

  it("name validation — case-insensitive collision detected", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));
    fireEvent.change(screen.getByLabelText(/Name \*/), { target: { value: "MYPROJ" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(
      screen.getByText(/already exists/)
    ).toBeInTheDocument();
    expect(mockUpsertAlias).not.toHaveBeenCalled();
  });

  it("edit mode skips uniqueness check for own name", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpsertAlias).toHaveBeenCalled();
    });
    expect(screen.queryByText(/already exists/)).not.toBeInTheDocument();
  });

  it("delete flow — click Delete shows Confirm delete? button", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByTestId("alias-delete-myproj"));

    expect(screen.getByTestId("alias-confirm-delete-myproj")).toBeInTheDocument();
    expect(screen.getByText("Confirm delete?")).toBeInTheDocument();
  });

  it("delete flow — confirm delete calls deleteAlias", async () => {
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByTestId("alias-delete-myproj"));
    fireEvent.click(screen.getByTestId("alias-confirm-delete-myproj"));

    await waitFor(() => {
      expect(mockDeleteAlias).toHaveBeenCalledWith({ name: "myproj" });
    });
  });

  it("delete flow — auto-cancel after 3 seconds", async () => {
    render(<AliasesManager />);
    // Wait for the alias list to load with real timers before switching to fake
    await waitFor(() => expect(screen.getByTestId("alias-delete-myproj")).toBeInTheDocument());

    jest.useFakeTimers();
    fireEvent.click(screen.getByTestId("alias-delete-myproj"));
    expect(screen.getByTestId("alias-confirm-delete-myproj")).toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(3001);
    });

    expect(screen.queryByTestId("alias-confirm-delete-myproj")).not.toBeInTheDocument();
    jest.useRealTimers();
  });

  it("live @name preview — updates as user types", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));

    // default preview
    expect(screen.getByText("Preview: @name")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/Name \*/), { target: { value: "proj-x" } });
    expect(screen.getByText("Preview: @proj-x")).toBeInTheDocument();
  });

  it("env var add/remove — check Advanced, add variable, fill key, remove", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));

    // Check Advanced checkbox
    const advancedCheckbox = screen.getByRole("checkbox", { name: /Advanced/i });
    fireEvent.click(advancedCheckbox);

    // Click "Add variable"
    fireEvent.click(screen.getByRole("button", { name: "Add variable" }));

    // Fill in key
    const keyInputs = screen.getAllByPlaceholderText("KEY");
    fireEvent.change(keyInputs[0], { target: { value: "MY_VAR" } });

    // Remove it
    const removeBtn = screen.getByRole("button", { name: /Remove environment variable MY_VAR/ });
    fireEvent.click(removeBtn);

    expect(screen.queryByPlaceholderText("KEY")).not.toBeInTheDocument();
  });

  it("advanced section expand/collapse", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));

    // "Add variable" not visible before expanding
    expect(screen.queryByRole("button", { name: "Add variable" })).not.toBeInTheDocument();

    // Expand
    const advancedCheckbox = screen.getByRole("checkbox", { name: /Advanced/i });
    fireEvent.click(advancedCheckbox);
    expect(screen.getByRole("button", { name: "Add variable" })).toBeInTheDocument();

    // Collapse
    fireEvent.click(advancedCheckbox);
    expect(screen.queryByRole("button", { name: "Add variable" })).not.toBeInTheDocument();
  });

  it("shows error when upsertAlias RPC fails", async () => {
    mockListAliases.mockResolvedValue({ aliases: [] });
    mockUpsertAlias.mockRejectedValueOnce(new Error("network error"));
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("No aliases configured."));

    fireEvent.click(screen.getByRole("button", { name: "New Alias" }));
    fireEvent.change(screen.getByLabelText(/Name \*/), { target: { value: "newproj" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/Failed to save alias/);
  });

  it("shows error when deleteAlias RPC fails", async () => {
    mockDeleteAlias.mockRejectedValueOnce(new Error("network error"));
    render(<AliasesManager />);
    await waitFor(() => screen.getByText("@myproj"));

    fireEvent.click(screen.getByTestId("alias-delete-myproj"));
    fireEvent.click(screen.getByTestId("alias-confirm-delete-myproj"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/Failed to delete alias/);
  });
});
