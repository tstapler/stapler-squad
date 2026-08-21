import React from "react";
import { render, screen } from "@testing-library/react";
import { SESSION_TYPES, OmnibarCreationPanel } from "./OmnibarCreationPanel";
import type { OmnibarCreationPanelProps } from "./OmnibarCreationPanel";
import type { OmnibarFormState } from "./Omnibar";

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
