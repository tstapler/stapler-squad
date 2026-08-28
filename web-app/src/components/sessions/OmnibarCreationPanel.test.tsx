import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SESSION_TYPES, OmnibarCreationPanel } from "./OmnibarCreationPanel";
import type { OmnibarCreationPanelProps, RemoteOption } from "./OmnibarCreationPanel";
import type { OmnibarFormState } from "./Omnibar";
import type { WorktreeEntry } from "@/gen/session/v1/session_pb";

const DEFAULT_FORM_STATE: OmnibarFormState = {
  sessionName: "test-session",
  branch: "",
  program: "claude",
  category: "",
  autoYes: false,
  useTitleAsBranch: true,
  sessionType: "new_worktree",
  existingWorktree: "",
  workingDir: "",
  parentDir: "",
  projectName: "",
  newProjectSessionType: "new_worktree",
  firstPrompt: "",
  createIfMissing: false,
  autonomousMode: false,
  autoApprove: false,
  extraArgs: [],
};

function buildProps(overrides: Partial<OmnibarCreationPanelProps> = {}): OmnibarCreationPanelProps {
  return {
    formState: DEFAULT_FORM_STATE,
    setFormField: jest.fn(),
    onSubmit: jest.fn(),
    onCancel: jest.fn(),
    worktrees: [],
    isSubmitting: false,
    canSubmit: true,
    error: null,
    showAdvanced: false,
    onToggleAdvanced: jest.fn(),
    uploadBaseUrl: "/api",
    onAttachedImagesChange: jest.fn(),
    ...overrides,
  };
}

describe("omnibar-create-session-button testid", () => {
  it("resolves to exactly one element", () => {
    render(<OmnibarCreationPanel {...buildProps()} />);
    expect(screen.getByTestId("omnibar-create-session-button")).toBeInTheDocument();
  });
});

describe("remote selector (Epic 4.3 Story 4.3.2 — ADR-001: remote-as-orthogonal-flag)", () => {
  const REMOTES: RemoteOption[] = [{ name: "prod-box" }, { name: "staging-box" }];

  it("is absent from the DOM (not merely disabled) when zero remotes are configured", () => {
    render(<OmnibarCreationPanel {...buildProps({ remotes: [] })} />);
    expect(screen.queryByTestId("remote-selector")).not.toBeInTheDocument();
  });

  it("is also absent when the remotes prop is omitted entirely (defaults to empty)", () => {
    render(<OmnibarCreationPanel {...buildProps()} />);
    expect(screen.queryByTestId("remote-selector")).not.toBeInTheDocument();
  });

  it("renders with a 'This machine' default option plus every configured remote when >=1 is configured", () => {
    render(<OmnibarCreationPanel {...buildProps({ remotes: REMOTES })} />);
    const select = screen.getByTestId("remote-selector");
    expect(select).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "This machine" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "prod-box" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "staging-box" })).toBeInTheDocument();
  });

  it("stays visible and composable alongside existing_worktree-specific fields when a remote is selected", () => {
    const setFormField = jest.fn();
    // A non-empty worktrees list routes existing_worktree to the plain <select> branch
    // (rather than RepoPathInput, which needs a Redux Provider this test doesn't set up) —
    // irrelevant to what's under test here (remote-selector composability).
    const worktrees: WorktreeEntry[] = [
      { path: "/repo/wt1", branch: "feat", isMain: false } as WorktreeEntry,
    ];
    render(
      <OmnibarCreationPanel
        {...buildProps({
          remotes: REMOTES,
          setFormField,
          worktrees,
          formState: { ...DEFAULT_FORM_STATE, sessionType: "existing_worktree" },
        })}
      />
    );

    // The remote selector composes with the session type — it's a sibling control, not a
    // replacement for existing_worktree's own fields (ADR-001).
    const select = screen.getByTestId("remote-selector");
    expect(select).toBeInTheDocument();
    expect(screen.getByLabelText(/existing worktree path/i)).toBeInTheDocument();

    fireEvent.change(select, { target: { value: "prod-box" } });
    expect(setFormField).toHaveBeenCalledWith("remoteName", "prod-box");

    // existing_worktree's own field remains visible and functional after the remote pick.
    expect(screen.getByLabelText(/existing worktree path/i)).toBeInTheDocument();
  });
});

describe("SESSION_TYPES hint copy", () => {
  it("gives every session type a non-empty scenario-based description", () => {
    for (const type of SESSION_TYPES) {
      expect(type.description).toMatch(/^Use this when/);
    }
  });

  it("gives every session type a distinct description", () => {
    const descriptions = SESSION_TYPES.map((t) => t.description);
    expect(new Set(descriptions).size).toBe(descriptions.length);
  });
});
