/**
 * Tests for BacklogItemForm — focused on the repo-path UX fix.
 *
 * Covers:
 *  1. Repository Path field renders its generic hint text
 *  2. Skip planning / skip review gate checkboxes render plain-language help text
 *  3. Typing a GitHub URL into Repository Path shows the "Will clone" confirmation
 *  4. Submitting with a GitHub URL in Repository Path shows a "Cloning repository…" busy label
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogItemForm } from "./BacklogItemForm";

// RepoPathInput uses useSessionRepoPaths (Redux) and usePathCompletions (RPC).
// Stub both so tests don't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

describe("BacklogItemForm — Repository Path help", () => {
  it("renders the generic repo-path hint", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(
      screen.getByText("Local path to your clone, or a GitHub URL — we'll clone it for you.")
    ).toBeInTheDocument();
  });

  it("shows a clone confirmation when a GitHub URL is entered", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "https://github.com/facebook/react" },
    });

    expect(screen.getByTestId("repo-path-github-hint")).toHaveTextContent(
      "Will clone facebook/react"
    );
  });
});

describe("BacklogItemForm — checkbox help text", () => {
  it("explains skip planning phase in plain language", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(
      screen.getByText("Go straight to triage without a separate planning pass.")
    ).toBeInTheDocument();
  });

  it("explains skip review gate in plain language", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(
      screen.getByText("Mark work done without an automated review pass first.")
    ).toBeInTheDocument();
  });
});

describe("BacklogItemForm — cloning busy state", () => {
  it("shows 'Cloning repository…' instead of 'Saving…' while submitting a GitHub URL", async () => {
    let resolveSubmit: () => void = () => {};
    const onSubmit = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSubmit = resolve;
        })
    );

    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Clone and triage this repo" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "https://github.com/facebook/react" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("backlog-form-submit")).toHaveTextContent("Cloning repository…")
    );

    resolveSubmit();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
  });
});
