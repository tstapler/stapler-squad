import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import sessionsReducer from "@/lib/store/sessionsSlice";
import { GitHubIssuePicker } from "./GitHubIssuePicker";
import type { GitHubIssue, GitHubRepo } from "@/lib/hooks/useBacklogService";

const searchGitHubRepos = jest.fn();
const listGitHubIssues = jest.fn();

// Returning the same jest.fn() references (not fresh arrow wrappers) matters:
// useGitHubIssuePicker's effect depends on listGitHubIssues by reference, and
// the real hook memoizes it — a fresh reference every render would make that
// effect re-fire every render, an infinite loop once anything gets cached.
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    searchGitHubRepos,
    listGitHubIssues,
  }),
  GitHubAuthError: class GitHubAuthError extends Error {},
}));

function makeStore() {
  return configureStore({ reducer: { sessions: sessionsReducer } });
}

beforeEach(() => {
  jest.clearAllMocks();
  // useGitHubIssuePicker caches issues/repos in real localStorage — clear
  // between tests so one test's fetch result doesn't leak into the next.
  window.localStorage.clear();
});

function renderPicker(onSelect = jest.fn(), onCancel = jest.fn()) {
  const store = makeStore();
  render(
    <Provider store={store}>
      <GitHubIssuePicker onSelect={onSelect} onCancel={onCancel} />
    </Provider>
  );
  return { onSelect, onCancel };
}

const REPO: GitHubRepo = { owner: "octocat", repo: "hello-world", description: "", isLocal: false, localPath: "" };

function makeIssue(overrides: Partial<GitHubIssue> = {}): GitHubIssue {
  return {
    number: 1,
    title: "Some issue",
    state: "open",
    url: "https://github.com/octocat/hello-world/issues/1",
    labels: [],
    isPR: false,
    ...overrides,
  };
}

async function goToIssuePhase(issues: GitHubIssue[]) {
  searchGitHubRepos.mockResolvedValue([REPO]);
  listGitHubIssues.mockResolvedValue(issues);
  const handlers = renderPicker();

  await waitFor(() => expect(screen.getByText("octocat/hello-world")).toBeInTheDocument());
  fireEvent.mouseDown(screen.getByText("octocat/hello-world"));

  await waitFor(() => expect(listGitHubIssues).toHaveBeenCalled());
  return handlers;
}

describe("GitHubIssuePicker — multi-select", () => {
  it("clicking anywhere on a row toggles its selection (not just the checkbox)", async () => {
    await goToIssuePhase([makeIssue({ number: 7, title: "Fix the thing" })]);

    await waitFor(() => expect(screen.getByText("Fix the thing")).toBeInTheDocument());
    expect(screen.getByText("Check issues to import, then Import.")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Fix the thing"));

    expect(screen.getByText("1 selected")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import (1)" })).not.toBeDisabled();
  });

  it("clicking the checkbox itself also toggles selection", async () => {
    await goToIssuePhase([makeIssue({ number: 7, title: "Fix the thing" })]);
    await waitFor(() => expect(screen.getByText("Fix the thing")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("checkbox", { name: /Select issue #7/ }));

    expect(screen.getByText("1 selected")).toBeInTheDocument();
  });

  it("calls onSelect with every checked issue when Import is clicked", async () => {
    const { onSelect } = await goToIssuePhase([
      makeIssue({ number: 1, title: "First" }),
      makeIssue({ number: 2, title: "Second" }),
    ]);
    await waitFor(() => expect(screen.getByText("First")).toBeInTheDocument());

    fireEvent.click(screen.getByText("First"));
    fireEvent.click(screen.getByText("Second"));
    expect(screen.getByText("2 selected")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Import (2)" }));

    expect(onSelect).toHaveBeenCalledWith(
      "octocat",
      "hello-world",
      expect.arrayContaining([
        expect.objectContaining({ number: 1 }),
        expect.objectContaining({ number: 2 }),
      ])
    );
  });

  // ImportGitHubIssue (server/services/backlog_service_sync.go) rejects PR
  // URLs outright — ParseGitHubRef categorizes /pull/N as RefTypePR, not
  // RefTypeIssue. The picker's issue list includes PRs (GitHub's /issues
  // endpoint returns both), so a PR row must never be selectable — otherwise
  // Import always fails for it with no indication why until the user tries.
  describe("pull requests can't be imported", () => {
    it("disables the checkbox for a PR row", async () => {
      await goToIssuePhase([makeIssue({ number: 9, title: "Some PR", isPR: true })]);
      await waitFor(() => expect(screen.getByText("Some PR")).toBeInTheDocument());

      expect(screen.getByRole("checkbox", { name: /can't be imported/ })).toBeDisabled();
    });

    it("does not select a PR row when clicked", async () => {
      await goToIssuePhase([makeIssue({ number: 9, title: "Some PR", isPR: true })]);
      await waitFor(() => expect(screen.getByText("Some PR")).toBeInTheDocument());

      fireEvent.click(screen.getByText("Some PR"));

      expect(screen.getByText("Check issues to import, then Import.")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Import" })).toBeDisabled();
    });

    it("still allows selecting a real issue alongside a disabled PR row", async () => {
      await goToIssuePhase([
        makeIssue({ number: 9, title: "Some PR", isPR: true }),
        makeIssue({ number: 10, title: "A real issue", isPR: false }),
      ]);
      await waitFor(() => expect(screen.getByText("A real issue")).toBeInTheDocument());

      fireEvent.click(screen.getByText("Some PR"));
      fireEvent.click(screen.getByText("A real issue"));

      expect(screen.getByText("1 selected")).toBeInTheDocument();
    });
  });
});
