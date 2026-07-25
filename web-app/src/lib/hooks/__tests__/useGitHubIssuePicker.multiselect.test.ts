/**
 * Tests for useGitHubIssuePicker's multi-issue selection contract
 * (selectIssues) — introduced to let the picker import several GitHub
 * issues in one go instead of one at a time.
 */

import { renderHook, act } from "@testing-library/react";
import React from "react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import sessionsReducer from "@/lib/store/sessionsSlice";
import { useGitHubIssuePicker } from "../useGitHubIssuePicker";
import type { GitHubIssue, GitHubRepo } from "../useBacklogService";

function makeStore() {
  return configureStore({
    reducer: { sessions: sessionsReducer },
  });
}

function makeWrapper(store: ReturnType<typeof makeStore>) {
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(Provider, { store } as any, children);
  }
  return Wrapper;
}

function makeRepo(overrides: Partial<GitHubRepo> = {}): GitHubRepo {
  return { owner: "octocat", repo: "hello-world", description: "", isLocal: false, localPath: "", ...overrides };
}

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

describe("useGitHubIssuePicker — selectIssues", () => {
  it("calls onSelect once with the owner/repo and every selected issue", async () => {
    const onSelect = jest.fn();
    const searchGitHubRepos = jest.fn().mockResolvedValue([]);
    const listGitHubIssues = jest.fn().mockResolvedValue([]);
    const store = makeStore();

    const { result } = renderHook(
      () => useGitHubIssuePicker({ searchGitHubRepos, listGitHubIssues, onSelect }),
      { wrapper: makeWrapper(store) }
    );
    // Let the mount-time searchGitHubRepos("", 100) fetch settle before driving selections.
    await act(async () => {});

    act(() => {
      result.current.selectRepo(makeRepo());
    });

    const issues = [makeIssue({ number: 1 }), makeIssue({ number: 2 }), makeIssue({ number: 3 })];
    act(() => {
      result.current.selectIssues(issues);
    });

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith("octocat", "hello-world", issues);
  });

  it("is a no-op when called with an empty selection", async () => {
    const onSelect = jest.fn();
    const searchGitHubRepos = jest.fn().mockResolvedValue([]);
    const listGitHubIssues = jest.fn().mockResolvedValue([]);
    const store = makeStore();

    const { result } = renderHook(
      () => useGitHubIssuePicker({ searchGitHubRepos, listGitHubIssues, onSelect }),
      { wrapper: makeWrapper(store) }
    );
    // Let the mount-time searchGitHubRepos("", 100) fetch settle before driving selections.
    await act(async () => {});

    act(() => {
      result.current.selectRepo(makeRepo());
    });
    act(() => {
      result.current.selectIssues([]);
    });

    expect(onSelect).not.toHaveBeenCalled();
  });

  it("is a no-op before a repo has been selected", async () => {
    const onSelect = jest.fn();
    const searchGitHubRepos = jest.fn().mockResolvedValue([]);
    const listGitHubIssues = jest.fn().mockResolvedValue([]);
    const store = makeStore();

    const { result } = renderHook(
      () => useGitHubIssuePicker({ searchGitHubRepos, listGitHubIssues, onSelect }),
      { wrapper: makeWrapper(store) }
    );
    await act(async () => {});

    act(() => {
      result.current.selectIssues([makeIssue()]);
    });

    expect(onSelect).not.toHaveBeenCalled();
  });
});
