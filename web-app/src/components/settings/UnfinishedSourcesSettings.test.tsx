import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { UnfinishedSourcesSettings } from "./UnfinishedSourcesSettings";

const mockUpdateConfig = jest.fn();

jest.mock("@/lib/hooks/useUnfinishedWorkConfig", () => ({
  useUnfinishedWorkConfig: () => ({
    config: { autoSpider: true, watchDirs: [], pinnedRepos: [] },
    loading: false,
    updateConfig: mockUpdateConfig,
  }),
}));
jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({ useGitHubEnterpriseHosts: () => ({ hosts: [] }) }));
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({ useSessionRepoPaths: () => [] }));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("./UnfinishedSourcesSettings.css", () => new Proxy({}, { get: (_t, p) => (typeof p === "string" ? p : "") }));

describe("UnfinishedSourcesSettings path fields", () => {
  beforeEach(() => mockUpdateConfig.mockClear());

  it("renders both watch-dir and pinned-repo fields as the rich path combobox", () => {
    render(<UnfinishedSourcesSettings />);
    const boxes = screen.getAllByRole("combobox");
    expect(boxes).toHaveLength(2);
    expect(screen.getByTestId("pinned-repo-input")).toHaveAttribute("aria-autocomplete", "list");
  });

  it("adds a pinned repo via the Add button", () => {
    render(<UnfinishedSourcesSettings />);
    fireEvent.change(screen.getByTestId("pinned-repo-input"), { target: { value: "/Users/me/proj" } });
    const addButtons = screen.getAllByRole("button", { name: "Add" });
    fireEvent.click(addButtons[addButtons.length - 1]);
    expect(mockUpdateConfig).toHaveBeenCalledWith(expect.objectContaining({ pinnedRepos: ["/Users/me/proj"] }));
  });

  it("gives both fields accessible names", () => {
    render(<UnfinishedSourcesSettings />);
    expect(screen.getByRole("combobox", { name: "New pinned repository path" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "New watch directory path" })).toBeInTheDocument();
  });

  it("adds a pinned repo on Enter (behavior preserved from the plain input)", () => {
    render(<UnfinishedSourcesSettings />);
    const field = screen.getByRole("combobox", { name: "New pinned repository path" });
    fireEvent.change(field, { target: { value: "/Users/me/proj" } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(mockUpdateConfig).toHaveBeenCalledWith(expect.objectContaining({ pinnedRepos: ["/Users/me/proj"] }));
  });
});
