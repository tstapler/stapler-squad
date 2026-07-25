/**
 * Tests for RepoPathInput component.
 *
 * Covers:
 *  1. Renders the generic hint when provided and value is not a GitHub ref
 *  2. detectGitHubUrl shows a "will clone" confirmation for a GitHub URL, replacing the generic hint
 *  3. detectGitHubUrl shows the confirmation for owner/repo shorthand too
 *  4. Without detectGitHubUrl, a GitHub-looking value does not trigger the confirmation
 *  5. detectGitHubUrl with a plain local path falls back to the generic hint
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { RepoPathInput } from "./RepoPathInput";

// RepoPathInput uses useSessionRepoPaths (Redux) and usePathCompletions (RPC).
// Stub both so tests don't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

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
