/**
 * Tests for RepoPathInput component.
 *
 * Covers:
 *  1. Renders the generic hint when provided and value is not a GitHub ref
 *  2. detectGitHubUrl shows a "will clone" confirmation for a GitHub URL, replacing the generic hint
 *  3. detectGitHubUrl shows the confirmation for owner/repo shorthand too
 *  4. Without detectGitHubUrl, a GitHub-looking value does not trigger the confirmation
 *  5. detectGitHubUrl with a plain local path falls back to the generic hint
 *  6. Escape key handling: stops propagation when the dropdown is visible, does not
 *     when it is not (RepoPathInput — Escape key handling)
 *  7. Combobox ARIA triad present and reflects live dropdown state (RepoPathInput — combobox a11y)
 */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { RepoPathInput } from "./RepoPathInput";
import { useSessionRepoPaths } from "@/lib/hooks/useSessionRepoPaths";

// RepoPathInput uses useSessionRepoPaths (Redux) and usePathCompletions (RPC).
// Stub both so tests don't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: jest.fn(() => []),
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

const mockUseSessionRepoPaths = useSessionRepoPaths as jest.Mock;

beforeEach(() => {
  mockUseSessionRepoPaths.mockReturnValue([]);
});

describe("RepoPathInput — hint", () => {
  it("renders the generic hint when value is not a GitHub ref", () => {
    render(
      <RepoPathInput
        value="/home/user/project"
        onChange={jest.fn()}
        hint="Local path to your clone, or a GitHub URL — we'll clone it for you."
      />
    );

    expect(
      screen.getByText("Local path to your clone, or a GitHub URL — we'll clone it for you.")
    ).toBeInTheDocument();
    expect(screen.queryByTestId("repo-path-github-hint")).not.toBeInTheDocument();
  });
});

describe("RepoPathInput — GitHub URL detection", () => {
  it("shows a clone confirmation for a full GitHub URL and hides the generic hint", () => {
    render(
      <RepoPathInput
        value="https://github.com/facebook/react"
        onChange={jest.fn()}
        hint="Local path to your clone, or a GitHub URL — we'll clone it for you."
        detectGitHubUrl
      />
    );

    const githubHint = screen.getByTestId("repo-path-github-hint");
    expect(githubHint).toHaveTextContent("Will clone facebook/react");
    expect(githubHint).toHaveTextContent(
      "~/.stapler-squad/repos/github.com/facebook/react"
    );
    expect(
      screen.queryByText("Local path to your clone, or a GitHub URL — we'll clone it for you.")
    ).not.toBeInTheDocument();
  });

  it("shows a clone confirmation for owner/repo shorthand", () => {
    render(
      <RepoPathInput value="facebook/react" onChange={jest.fn()} detectGitHubUrl />
    );

    expect(screen.getByTestId("repo-path-github-hint")).toHaveTextContent(
      "Will clone facebook/react"
    );
  });

  it("does not show the confirmation when detectGitHubUrl is not set", () => {
    render(<RepoPathInput value="https://github.com/facebook/react" onChange={jest.fn()} />);

    expect(screen.queryByTestId("repo-path-github-hint")).not.toBeInTheDocument();
  });

  it("falls back to the generic hint for a plain local path even with detectGitHubUrl set", () => {
    render(
      <RepoPathInput
        value="/home/user/project"
        onChange={jest.fn()}
        hint="Local path to your clone."
        detectGitHubUrl
      />
    );

    expect(screen.getByText("Local path to your clone.")).toBeInTheDocument();
    expect(screen.queryByTestId("repo-path-github-hint")).not.toBeInTheDocument();
  });
});

describe("RepoPathInput — Escape key handling", () => {
  it("closes the dropdown when Escape is pressed while it is open (isolated)", () => {
    mockUseSessionRepoPaths.mockReturnValue(["/home/user/project-a"]);

    render(<RepoPathInput value="" onChange={jest.fn()} />);

    const input = screen.getByRole("combobox");
    fireEvent.focus(input);
    expect(screen.getByRole("listbox")).toBeInTheDocument();

    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("does not bubble to a parent's own keydown handler when the dropdown is open", () => {
    mockUseSessionRepoPaths.mockReturnValue(["/home/user/project-a"]);
    const parentKeyDown = jest.fn();

    render(
      <div onKeyDown={parentKeyDown}>
        <RepoPathInput value="" onChange={jest.fn()} />
      </div>
    );

    const input = screen.getByRole("combobox");
    fireEvent.focus(input);
    expect(screen.getByRole("listbox")).toBeInTheDocument();

    fireEvent.keyDown(input, { key: "Escape" });
    expect(parentKeyDown).not.toHaveBeenCalled();
  });

  it("bubbles normally to a parent's keydown handler when the dropdown was never opened", () => {
    mockUseSessionRepoPaths.mockReturnValue(["/home/user/project-a"]);
    const parentKeyDown = jest.fn();

    render(
      <div onKeyDown={parentKeyDown}>
        <RepoPathInput value="" onChange={jest.fn()} />
      </div>
    );

    const input = screen.getByRole("combobox");
    // No focus event fired, so `open` never becomes true.
    fireEvent.keyDown(input, { key: "Escape" });
    expect(parentKeyDown).toHaveBeenCalled();
  });

  it("bubbles normally when open is true but the dropdown renders nothing (empty history, no fs matches)", () => {
    mockUseSessionRepoPaths.mockReturnValue([]);
    const parentKeyDown = jest.fn();

    render(
      <div onKeyDown={parentKeyDown}>
        <RepoPathInput value="" onChange={jest.fn()} />
      </div>
    );

    const input = screen.getByRole("combobox");
    fireEvent.focus(input);
    // Dropdown never renders because there are no history entries and no fs matches.
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();

    fireEvent.keyDown(input, { key: "Escape" });
    expect(parentKeyDown).toHaveBeenCalled();
  });
});

describe("RepoPathInput — combobox a11y", () => {
  it("has the combobox ARIA triad with aria-expanded false when the dropdown is closed", () => {
    mockUseSessionRepoPaths.mockReturnValue([]);

    render(<RepoPathInput value="" onChange={jest.fn()} />);

    const input = screen.getByRole("combobox");
    expect(input).toHaveAttribute("aria-haspopup", "listbox");
    expect(input).toHaveAttribute("aria-expanded", "false");
  });

  it("reflects aria-expanded true once the dropdown opens", () => {
    mockUseSessionRepoPaths.mockReturnValue(["/home/user/x"]);

    render(<RepoPathInput value="" onChange={jest.fn()} />);

    const input = screen.getByRole("combobox");
    fireEvent.focus(input);
    expect(input).toHaveAttribute("aria-expanded", "true");
  });
});
