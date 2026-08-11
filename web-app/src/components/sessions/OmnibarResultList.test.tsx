/**
 * Tests for OmnibarResultList component.
 *
 * Covers:
 *  - T-UNIT-TS-017: Session section hidden when sessionResults is empty
 *  - T-UNIT-TS-018: Repo section hidden when repoEntries is empty
 *  - T-UNIT-TS-019: "+ New Session" item always renders
 *  - T-UNIT-TS-021: ARIA activedescendant tracking via getHighlightedItemId helper
 *  - T-UNIT-TS-022: Sessions DOM order precedes repos (pitfall guard T-PITFALL-009)
 *  - onSessionSelect called with correct session on item click
 *  - onRepoSelect called with correct path on item click
 *  - getResultListItemCount helper returns correct totals
 *  - getHighlightedItemId helper returns correct ids
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import {
  OmnibarResultList,
  getResultListItemCount,
  getHighlightedItemId,
} from "./OmnibarResultList";
import type { SessionSearchResult } from "@/lib/hooks/useSessionSearch";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mock row components with simple <li> stubs so tests stay fast and isolated.
// The row component tests live in separate files.
// ---------------------------------------------------------------------------

jest.mock("./OmnibarSessionResult", () => ({
  OmnibarSessionResult: ({
    result,
    isHighlighted,
    id,
    onClick,
  }: {
    result: { session: { id: string; title: string } };
    isHighlighted: boolean;
    id: string;
    onClick: (session: { id: string; title: string }) => void;
  }) => (
    <li
      role="option"
      id={id}
      aria-selected={isHighlighted}
      data-testid={`session-result-${result.session.id}`}
      onMouseDown={(e) => {
        e.preventDefault();
        onClick(result.session);
      }}
    >
      {result.session.title}
    </li>
  ),
}));

jest.mock("./OmnibarRepoResult", () => ({
  OmnibarRepoResult: ({
    entry,
    isHighlighted,
    id,
    onClick,
  }: {
    entry: { path: string };
    isHighlighted: boolean;
    id: string;
    onClick: (path: string) => void;
  }) => (
    <li
      role="option"
      id={id}
      aria-selected={isHighlighted}
      data-testid={`repo-result-${entry.path}`}
      onMouseDown={(e) => {
        e.preventDefault();
        onClick(entry.path);
      }}
    >
      {entry.path}
    </li>
  ),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(overrides: Partial<Record<string, unknown>> = {}): Session {
  return {
    id: "session-1",
    title: "My Session",
    status: 1, // RUNNING
    branch: "main",
    path: "/home/user/project",
    tags: [] as string[],
    ...overrides,
  } as unknown as Session;
}

function makeSessionResult(
  session: Session,
  score = 0
): SessionSearchResult {
  return { session, score, matchedFields: [] };
}

function makeRepoEntry(path: string, lastUsed = Date.now()): PathHistoryEntry {
  return { path, count: 1, lastUsed };
}

const DEFAULT_PROPS = {
  onSessionSelect: jest.fn(),
  onRepoSelect: jest.fn(),
  onCreateNew: jest.fn(),
  highlightedIndex: -1,
  id: "omnibar-result-listbox",
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("OmnibarResultList", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // T-UNIT-TS-019
  describe("create-new item", () => {
    it("always renders even when both sections are empty", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[]}
        />
      );

      expect(screen.getByText(/New Session/i)).toBeInTheDocument();
    });

    it("renders when only session results are present", () => {
      const session = makeSession({ id: "s1", title: "Auth" });
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[]}
        />
      );

      expect(screen.getByText(/New Session/i)).toBeInTheDocument();
    });

    it("renders when only repo entries are present", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[makeRepoEntry("/home/user/myrepo")]}
        />
      );

      expect(screen.getByText(/New Session/i)).toBeInTheDocument();
    });

    it("has role=option and correct id", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[]}
          id="test-listbox"
        />
      );

      const createNew = screen.getByRole("option", { name: /New Session/i });
      expect(createNew).toBeInTheDocument();
      expect(createNew.id).toBe("test-listbox-create-new");
    });

    it("calls onCreateNew when clicked via mouseDown", () => {
      const onCreateNew = jest.fn();
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          onCreateNew={onCreateNew}
          sessionResults={[]}
          repoEntries={[]}
        />
      );

      fireEvent.mouseDown(screen.getByRole("option", { name: /New Session/i }));
      expect(onCreateNew).toHaveBeenCalledTimes(1);
    });

    it("is highlighted when highlightedIndex equals sessionCount + repoCount", () => {
      const session = makeSession({ id: "s1", title: "Auth" });
      const repo = makeRepoEntry("/home/user/repo");
      // With 1 session + 1 repo, create-new index = 2
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[repo]}
          highlightedIndex={2}
        />
      );

      const createNew = screen.getByRole("option", { name: /New Session/i });
      expect(createNew).toHaveAttribute("aria-selected", "true");
    });

    it("is not highlighted when highlightedIndex does not point to it", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[]}
          highlightedIndex={-1}
        />
      );

      const createNew = screen.getByRole("option", { name: /New Session/i });
      expect(createNew).toHaveAttribute("aria-selected", "false");
    });
  });

  // T-UNIT-TS-017
  describe("session section visibility", () => {
    it("hides SESSIONS header when sessionResults is empty", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[makeRepoEntry("/home/user/repo")]}
        />
      );

      expect(screen.queryByText("SESSIONS")).not.toBeInTheDocument();
    });

    it("shows SESSIONS header when sessionResults is non-empty", () => {
      const session = makeSession({ id: "s1", title: "Auth" });
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[]}
        />
      );

      expect(screen.getByText("SESSIONS")).toBeInTheDocument();
    });

    it("renders one row per session result", () => {
      const sessions = [
        makeSession({ id: "s1", title: "Session One" }),
        makeSession({ id: "s2", title: "Session Two" }),
      ];
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={sessions.map((s) => makeSessionResult(s))}
          repoEntries={[]}
        />
      );

      expect(screen.getByTestId("session-result-s1")).toBeInTheDocument();
      expect(screen.getByTestId("session-result-s2")).toBeInTheDocument();
    });
  });

  // T-UNIT-TS-018
  describe("repo section visibility", () => {
    it("hides REPOS header when repoEntries is empty", () => {
      const session = makeSession({ id: "s1", title: "Auth" });
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[]}
        />
      );

      expect(screen.queryByText("REPOS")).not.toBeInTheDocument();
    });

    it("shows REPOS header when repoEntries is non-empty", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[makeRepoEntry("/home/user/repo")]}
        />
      );

      expect(screen.getByText("REPOS")).toBeInTheDocument();
    });

    it("renders one row per repo entry", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[
            makeRepoEntry("/home/user/repo-a"),
            makeRepoEntry("/home/user/repo-b"),
          ]}
        />
      );

      expect(
        screen.getByTestId("repo-result-/home/user/repo-a")
      ).toBeInTheDocument();
      expect(
        screen.getByTestId("repo-result-/home/user/repo-b")
      ).toBeInTheDocument();
    });
  });

  // T-PITFALL-009: sessions DOM order precedes repos
  describe("DOM order: sessions before repos", () => {
    it("all session option elements appear before any repo option elements", () => {
      const session = makeSession({ id: "s1", title: "Auth" });
      const repo = makeRepoEntry("/home/user/myrepo");
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[repo]}
          id="test-lb"
        />
      );

      const options = screen.getAllByRole("option");
      // Filter out create-new (last item)
      const sessionOptions = options.filter((el) =>
        el.id.startsWith("test-lb-session")
      );
      const repoOptions = options.filter((el) =>
        el.id.startsWith("test-lb-repo")
      );

      expect(sessionOptions.length).toBeGreaterThan(0);
      expect(repoOptions.length).toBeGreaterThan(0);

      const lastSessionIndex = Math.max(
        ...sessionOptions.map((el) => options.indexOf(el))
      );
      const firstRepoIndex = Math.min(
        ...repoOptions.map((el) => options.indexOf(el))
      );
      expect(lastSessionIndex).toBeLessThan(firstRepoIndex);
    });
  });

  // onSessionSelect callback
  describe("onSessionSelect", () => {
    it("is called with the correct session when a session row is clicked", () => {
      const onSessionSelect = jest.fn();
      const session = makeSession({ id: "s1", title: "Auth" });
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          onSessionSelect={onSessionSelect}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[]}
        />
      );

      fireEvent.mouseDown(screen.getByTestId("session-result-s1"));
      expect(onSessionSelect).toHaveBeenCalledTimes(1);
      expect(onSessionSelect.mock.calls[0][0].id).toBe("s1");
    });

    it("is not called when a repo row is clicked", () => {
      const onSessionSelect = jest.fn();
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          onSessionSelect={onSessionSelect}
          sessionResults={[]}
          repoEntries={[makeRepoEntry("/home/user/myrepo")]}
        />
      );

      fireEvent.mouseDown(screen.getByTestId("repo-result-/home/user/myrepo"));
      expect(onSessionSelect).not.toHaveBeenCalled();
    });
  });

  // onRepoSelect callback
  describe("onRepoSelect", () => {
    it("is called with the correct path when a repo row is clicked", () => {
      const onRepoSelect = jest.fn();
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          onRepoSelect={onRepoSelect}
          sessionResults={[]}
          repoEntries={[makeRepoEntry("/home/user/myrepo")]}
        />
      );

      fireEvent.mouseDown(screen.getByTestId("repo-result-/home/user/myrepo"));
      expect(onRepoSelect).toHaveBeenCalledTimes(1);
      expect(onRepoSelect.mock.calls[0][0]).toBe("/home/user/myrepo");
    });

    it("is not called when a session row is clicked", () => {
      const onRepoSelect = jest.fn();
      const session = makeSession({ id: "s1", title: "Auth" });
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          onRepoSelect={onRepoSelect}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[]}
        />
      );

      fireEvent.mouseDown(screen.getByTestId("session-result-s1"));
      expect(onRepoSelect).not.toHaveBeenCalled();
    });
  });

  // ARIA structure
  describe("ARIA structure", () => {
    it("renders a listbox with the given id", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[]}
          id="my-listbox"
        />
      );

      const listbox = screen.getByRole("listbox");
      expect(listbox).toHaveAttribute("id", "my-listbox");
    });

    it("session result ids follow omnibar-result-session pattern", () => {
      const session = makeSession({ id: "s42", title: "Auth" });
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[]}
          id="lb"
        />
      );

      expect(screen.getByTestId("session-result-s42").id).toBe("lb-session-s42");
    });

    it("repo result ids follow omnibar-result-repo pattern with encoded path", () => {
      const path = "/home/user/my repo";
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[makeRepoEntry(path)]}
          id="lb"
        />
      );

      const expectedId = `lb-repo-${encodeURIComponent(path)}`;
      expect(screen.getByTestId(`repo-result-${path}`).id).toBe(expectedId);
    });

    it("section headers have role=presentation and aria-hidden", () => {
      const session = makeSession({ id: "s1", title: "Auth" });
      const { container } = render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={[makeRepoEntry("/a")]}
        />
      );

      // aria-hidden elements are excluded from getByRole queries by default;
      // query the DOM directly to verify the attribute combination.
      const headers = container.querySelectorAll(
        '[role="presentation"][aria-hidden="true"]'
      );
      expect(headers.length).toBeGreaterThanOrEqual(2);
    });
  });

  // Index management / highlightedIndex
  describe("highlighted index forwarding", () => {
    it("passes isHighlighted=true to the session at the given index", () => {
      const sessions = [
        makeSession({ id: "s1", title: "First" }),
        makeSession({ id: "s2", title: "Second" }),
      ];
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={sessions.map((s) => makeSessionResult(s))}
          repoEntries={[]}
          highlightedIndex={1}
        />
      );

      expect(screen.getByTestId("session-result-s1")).toHaveAttribute(
        "aria-selected",
        "false"
      );
      expect(screen.getByTestId("session-result-s2")).toHaveAttribute(
        "aria-selected",
        "true"
      );
    });

    it("passes isHighlighted=true to the repo at the correct offset", () => {
      const session = makeSession({ id: "s1", title: "Session" });
      const repos = [
        makeRepoEntry("/home/user/repo-a"),
        makeRepoEntry("/home/user/repo-b"),
      ];
      // 1 session + highlight index 2 → second repo (index 1)
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[makeSessionResult(session)]}
          repoEntries={repos}
          highlightedIndex={2}
        />
      );

      expect(
        screen.getByTestId("repo-result-/home/user/repo-a")
      ).toHaveAttribute("aria-selected", "false");
      expect(
        screen.getByTestId("repo-result-/home/user/repo-b")
      ).toHaveAttribute("aria-selected", "true");
    });
  });

  // sessionCounts optional prop
  describe("sessionCounts prop", () => {
    it("renders without error when sessionCounts is undefined", () => {
      render(
        <OmnibarResultList
          {...DEFAULT_PROPS}
          sessionResults={[]}
          repoEntries={[makeRepoEntry("/home/user/repo")]}
          sessionCounts={undefined}
        />
      );

      expect(screen.getByTestId("repo-result-/home/user/repo")).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// getResultListItemCount helper
// ---------------------------------------------------------------------------

describe("getResultListItemCount", () => {
  it("returns sessionCount + repoCount + 1", () => {
    expect(getResultListItemCount(3, 2)).toBe(6);
  });

  it("returns 1 when both counts are 0 (create-new item)", () => {
    expect(getResultListItemCount(0, 0)).toBe(1);
  });

  it("returns correct count with only sessions", () => {
    expect(getResultListItemCount(5, 0)).toBe(6);
  });

  it("returns correct count with only repos", () => {
    expect(getResultListItemCount(0, 3)).toBe(4);
  });
});

// ---------------------------------------------------------------------------
// getHighlightedItemId helper (T-UNIT-TS-021)
// ---------------------------------------------------------------------------

describe("getHighlightedItemId", () => {
  const sessions = [
    makeSession({ id: "s1", title: "First" }),
    makeSession({ id: "s2", title: "Second" }),
  ];
  const sessionResults = sessions.map((s) => makeSessionResult(s));
  const repos = [
    makeRepoEntry("/home/user/repo-a"),
    makeRepoEntry("/home/user/repo-b"),
  ];

  it("returns undefined when highlightedIndex is -1", () => {
    expect(
      getHighlightedItemId("lb", sessionResults, repos, -1)
    ).toBeUndefined();
  });

  it("returns session id for index within session range", () => {
    expect(getHighlightedItemId("lb", sessionResults, repos, 0)).toBe(
      "lb-session-s1"
    );
    expect(getHighlightedItemId("lb", sessionResults, repos, 1)).toBe(
      "lb-session-s2"
    );
  });

  it("returns repo id for index in repo range", () => {
    // 2 sessions → repos start at index 2
    expect(getHighlightedItemId("lb", sessionResults, repos, 2)).toBe(
      `lb-repo-${encodeURIComponent("/home/user/repo-a")}`
    );
    expect(getHighlightedItemId("lb", sessionResults, repos, 3)).toBe(
      `lb-repo-${encodeURIComponent("/home/user/repo-b")}`
    );
  });

  it("returns create-new id for last index", () => {
    // 2 sessions + 2 repos → create-new at index 4
    expect(getHighlightedItemId("lb", sessionResults, repos, 4)).toBe(
      "lb-create-new"
    );
  });

  it("returns create-new id when sessions and repos are empty", () => {
    expect(getHighlightedItemId("lb", [], [], 0)).toBe("lb-create-new");
  });
});
