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
 *  8. Worktree grouping: primary repo checkout ranks at/above its worktrees and is
 *     synthesized when absent from history (RepoPathInput — worktree grouping)
 */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { RepoPathInput } from "./RepoPathInput";
import { useSessionRepoPaths } from "@/lib/hooks/useSessionRepoPaths";
import { useRepoPathSuggestions, type RepoPathWorktreeInfo } from "@/lib/hooks/useRepoPathSuggestions";

// RepoPathInput uses useSessionRepoPaths (Redux), usePathCompletions (RPC), and
// useRepoPathSuggestions (RPC, worktree resolution). Stub all three so tests
// don't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: jest.fn(() => []),
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

jest.mock("@/lib/hooks/useRepoPathSuggestions", () => ({
  ...jest.requireActual("@/lib/hooks/useRepoPathSuggestions"),
  useRepoPathSuggestions: jest.fn(() => ({ resolutions: new Map(), version: 0 })),
}));

const mockUseSessionRepoPaths = useSessionRepoPaths as jest.Mock;
const mockUseRepoPathSuggestions = useRepoPathSuggestions as jest.Mock;

beforeEach(() => {
  mockUseSessionRepoPaths.mockReturnValue([]);
  mockUseRepoPathSuggestions.mockReturnValue({ resolutions: new Map(), version: 0 });
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

describe("RepoPathInput — worktree grouping", () => {
  function resolutions(
    entries: Record<string, RepoPathWorktreeInfo>
  ): Map<string, RepoPathWorktreeInfo> {
    return new Map(Object.entries(entries));
  }

  // Options render with a tilde-abbreviated display name, so assertions here
  // key off the `title` attribute (always the raw path) rather than the
  // rendered accessible name.
  function optionOrder(): string[] {
    return screen
      .getAllByRole("option")
      .map((o) => o.getAttribute("title"))
      .filter((t): t is string => t !== null);
  }

  it("AC0: ranks the primary repo checkout at or above its own worktrees, even though it's the oldest session", () => {
    const root = "/repo/stapler-squad";
    const worktrees = [
      "/repo/worktrees/stapler-squad-a",
      "/repo/worktrees/stapler-squad-b",
      "/repo/worktrees/stapler-squad-c",
      "/repo/worktrees/stapler-squad-d",
    ];
    // Recency order: 4 churny worktrees first, the older root session last.
    mockUseSessionRepoPaths.mockReturnValue([...worktrees, root]);
    mockUseRepoPathSuggestions.mockReturnValue({
      resolutions: resolutions({
        [root]: { rootPath: root, isMain: true, branch: "main" },
        ...Object.fromEntries(
          worktrees.map((w, i) => [w, { rootPath: root, isMain: false, branch: `feature-${i}` }])
        ),
      }),
      version: 1,
    });

    render(<RepoPathInput value="stapler-squad" onChange={jest.fn()} />);
    fireEvent.focus(screen.getByRole("combobox"));

    const order = optionOrder();
    const rootIndex = order.indexOf(root);
    expect(rootIndex).toBeGreaterThanOrEqual(0);
    for (const w of worktrees) {
      expect(rootIndex).toBeLessThan(order.indexOf(w));
    }
    // Worktree entries carry a visible "worktree of ..." label.
    expect(screen.getAllByText(/worktree of/)).toHaveLength(worktrees.length);
  });

  it("AC2: synthesizes the primary repo checkout even when no session is rooted there", () => {
    const root = "/repo/stapler-squad";
    const worktree = "/repo/worktrees/stapler-squad-a";
    mockUseSessionRepoPaths.mockReturnValue([worktree]);
    mockUseRepoPathSuggestions.mockReturnValue({
      resolutions: resolutions({
        [root]: { rootPath: root, isMain: true, branch: "main" },
        [worktree]: { rootPath: root, isMain: false, branch: "feature-a" },
      }),
      version: 1,
    });

    render(<RepoPathInput value="stapler-squad" onChange={jest.fn()} />);
    fireEvent.focus(screen.getByRole("combobox"));

    expect(optionOrder()).toContain(root);
  });

  it("AC3: falls back to plain, unlabeled history when resolution is unavailable (RPC failure/timeout)", () => {
    mockUseSessionRepoPaths.mockReturnValue(["/repo/plain-project"]);
    mockUseRepoPathSuggestions.mockReturnValue({ resolutions: new Map(), version: 0 });

    render(<RepoPathInput value="" onChange={jest.fn()} />);
    fireEvent.focus(screen.getByRole("combobox"));

    expect(optionOrder()).toEqual(["/repo/plain-project"]);
    expect(screen.queryByText(/worktree of/)).not.toBeInTheDocument();
  });

  it("AC4: matches a candidate with a trailing slash to its normalized resolution", () => {
    const root = "/repo/stapler-squad";
    const worktree = "/repo/worktrees/stapler-squad-a";
    mockUseSessionRepoPaths.mockReturnValue([`${worktree}/`]);
    mockUseRepoPathSuggestions.mockReturnValue({
      resolutions: resolutions({
        [root]: { rootPath: root, isMain: true, branch: "main" },
        [worktree]: { rootPath: root, isMain: false, branch: "feature-a" },
      }),
      version: 1,
    });

    render(<RepoPathInput value="stapler-squad" onChange={jest.fn()} />);
    fireEvent.focus(screen.getByRole("combobox"));

    const order = optionOrder();
    expect(order.indexOf(root)).toBeGreaterThanOrEqual(0);
    expect(order.indexOf(root)).toBeLessThan(order.indexOf(`${worktree}/`));
  });
});
