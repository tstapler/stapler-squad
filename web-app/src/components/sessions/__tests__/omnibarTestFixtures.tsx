/**
 * Shared jest.mock factory bodies, fixtures, and render/interaction helpers for
 * Omnibar's dependency-mock-heavy specs (Omnibar.spawnshell, Omnibar.submitReset,
 * Omnibar.remote, Omnibar.alias). They mock the same ~14 Omnibar dependencies and
 * share a renderOmnibar/typeAndDetect harness almost verbatim.
 *
 * babel-jest hoists `jest.mock(...)` calls above imports, so each spec file keeps
 * its own thin `jest.mock("module", () => require("./omnibarTestFixtures").mockXyz())`
 * line — only the factory bodies live here, deduplicated. `mockUsePathCompletions`,
 * `mockUsePathHistory`, and `mockUseAliases` are exported as live jest.fn() instances
 * so spec files can both wire them into a mock factory (via require(), for hoisting)
 * and call `.mockReturnValue(...)` on them directly in beforeEach — Jest gives each
 * spec file its own isolated module registry, so this module (and its jest.fn()
 * instances) is re-instantiated fresh per test file, not actually shared state.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import type { AliasEntry } from "@/lib/hooks/useAliases";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import { SessionType } from "@/gen/session/v1/types_pb";
import { Omnibar } from "../Omnibar";

// ---------------------------------------------------------------------------
// Mock function instances referenced both by the module factories below and by
// spec files' own beforeEach/afterEach setup.
// ---------------------------------------------------------------------------

export const mockUsePathCompletions = jest.fn();
export const mockUsePathHistory = jest.fn();
export const mockUseAliases = jest.fn();

// ---------------------------------------------------------------------------
// jest.mock factory bodies
// ---------------------------------------------------------------------------

export function mockNextNavigationModule() {
  return {
    usePathname: jest.fn(),
    useRouter: jest.fn(() => ({ push: jest.fn(), replace: jest.fn() })),
  };
}

export function mockThemeContextModule() {
  return { useTheme: jest.fn(() => ({ setTheme: jest.fn(), theme: "dark" })) };
}

export function mockConfigModule() {
  return { getApiBaseUrl: jest.fn(() => "http://localhost:8543") };
}

export function mockUsePathCompletionsModule() {
  return {
    usePathCompletions: (...args: unknown[]) => mockUsePathCompletions(...args),
    clearCompletionCache: jest.fn(),
  };
}

export function mockUsePathHistoryModule() {
  return {
    usePathHistory: (...args: unknown[]) => mockUsePathHistory(...args),
    clearPathHistory: jest.fn(),
  };
}

export function mockUseSessionSearchModule() {
  return { useSessionSearch: jest.fn(() => []) };
}

export function mockUseWorktreeSuggestionsModule() {
  return { useWorktreeSuggestions: jest.fn(() => ({ worktrees: [], isLoading: false })) };
}

export function mockUseAliasesModule() {
  return { useAliases: (...args: unknown[]) => mockUseAliases(...args) };
}

/** Plain variant: `complete` ignores its argument. */
export function mockUseAliasSuggestionsModule() {
  return {
    useAliasSuggestions: jest.fn(() => ({
      isAliasBrowse: false,
      isAliasCompletion: false,
      filteredAliases: [],
      complete: jest.fn(),
    })),
  };
}

/** Variant used by specs that assert the completed `@alias ` text. */
export function mockUseAliasSuggestionsWithLabelModule() {
  return {
    useAliasSuggestions: jest.fn(() => ({
      isAliasBrowse: false,
      isAliasCompletion: false,
      filteredAliases: [],
      complete: jest.fn((a: AliasEntry) => `@${a.name} `),
    })),
  };
}

export function mockUseAtCommandSuggestionsModule() {
  return {
    useAtCommandSuggestions: jest.fn(() => ({
      isAtCommand: false,
      suggestions: [],
      complete: jest.fn(),
    })),
  };
}

export function mockUseAvailableProgramsModule() {
  return { useAvailablePrograms: jest.fn(() => []) };
}

export function mockUseSlashCommandsModule() {
  return { useSlashCommands: jest.fn(() => ({ commands: [] })) };
}

export function mockUseSlashCommandSuggestionsModule() {
  return {
    useSlashCommandSuggestions: jest.fn(() => ({
      isActive: false,
      suggestions: [],
      complete: jest.fn(),
    })),
  };
}

export function mockStoreModule() {
  return { useAppSelector: jest.fn(() => []) };
}

export function mockSessionsSliceModule() {
  return {
    selectAllSessions: jest.fn(),
    selectActiveSessionsSortedByUpdatedAt: jest.fn(),
  };
}

export function mockOmnibarResultListModule() {
  return {
    OmnibarResultList: () => null,
    getResultListItemCount: jest.fn(() => 0),
    getHighlightedItemId: jest.fn(() => undefined),
  };
}

export function mockApiTransportModule() {
  return { getConnectTransport: jest.fn(() => ({})) };
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

export const defaultCompletions = {
  entries: [],
  baseDir: "/home/user",
  baseDirExists: false,
  pathExists: false,
  isLoading: false,
  error: null,
};

export const existingPathCompletions = {
  entries: [],
  baseDir: "/home/user",
  baseDirExists: true,
  pathExists: true,
  isLoading: false,
  error: null,
};

export function makeHistoryFixture() {
  return {
    getMatching: jest.fn((): PathHistoryEntry[] => []),
    getAll: jest.fn((): PathHistoryEntry[] => []),
    save: jest.fn(),
  };
}

export const SSQ_ALIAS: AliasEntry = {
  name: "ssq",
  group: "",
  path: "/home/user/projects/stapler-squad",
  description: "Stapler Squad project alias",
  profile: "",
  program: "claude",
  autoYes: false,
  tags: [],
  sessionType: SessionType.NEW_WORKTREE,
  namePrefix: "ssq-",
};

// ---------------------------------------------------------------------------
// Render / interaction helpers
// ---------------------------------------------------------------------------

export function renderOmnibar(
  props: {
    onClose?: jest.Mock;
    onCreateSession?: jest.Mock;
    onNavigateToSession?: jest.Mock;
    remotes?: { name: string }[];
  } = {}
) {
  const onClose = props.onClose ?? jest.fn();
  const onCreateSession = props.onCreateSession ?? jest.fn().mockResolvedValue(undefined);
  const onNavigateToSession = props.onNavigateToSession ?? jest.fn();
  const utils = render(
    <Omnibar
      isOpen={true}
      onClose={onClose}
      onCreateSession={onCreateSession}
      onNavigateToSession={onNavigateToSession}
      remotes={props.remotes}
    />
  );
  const input = screen.getByRole("combobox", { name: /session source input/i });
  return { ...utils, input, onClose, onCreateSession, onNavigateToSession };
}

/** Type a value into the omnibar input and wait for the 150ms detect debounce plus React state flush. */
export async function typeAndDetect(input: Element, value: string) {
  fireEvent.change(input, { target: { value } });
  await act(async () => {
    jest.advanceTimersByTime(200);
  });
}
