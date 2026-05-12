/**
 * Omnibar path completion integration tests.
 *
 * Mocks usePathCompletions so we can control entries/pathExists/isLoading.
 * Uses fake timers to advance past the Omnibar's own 150ms detect debounce.
 */

import React from "react";
import { render, screen, within, fireEvent, act } from "@testing-library/react";
import { Omnibar } from "../Omnibar";
import type { PathEntry } from "@/gen/session/v1/session_pb";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockUsePathCompletions = jest.fn();
const mockUsePathHistory = jest.fn();

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: (...args: unknown[]) => mockUsePathCompletions(...args),
  clearCompletionCache: jest.fn(),
}));

jest.mock("@/lib/hooks/usePathHistory", () => ({
  usePathHistory: (...args: unknown[]) => mockUsePathHistory(...args),
  clearPathHistory: jest.fn(),
}));

jest.mock("@/lib/hooks/useSessionSearch", () => ({
  useSessionSearch: jest.fn(() => []),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: jest.fn(() => []),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: jest.fn(),
}));

jest.mock("@/components/sessions/OmnibarResultList", () => ({
  OmnibarResultList: () => null,
  getResultListItemCount: jest.fn(() => 1),
  getHighlightedItemId: jest.fn(() => undefined),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const defaultCompletions = {
  entries: [] as PathEntry[],
  baseDir: "/home/user",
  baseDirExists: false,
  pathExists: false,
  isLoading: false,
  error: null,
};

const defaultHistory = {
  getMatching: jest.fn((): PathHistoryEntry[] => []),
  getAll: jest.fn((): PathHistoryEntry[] => []),
  save: jest.fn(),
};

const dir = (name: string, base = "/home/user"): PathEntry => ({
  name,
  path: `${base}/${name}`,
  isDirectory: true,
} as unknown as PathEntry);

function renderOmnibar(
  props: { onClose?: jest.Mock; onCreateSession?: jest.Mock; onNavigateToSession?: jest.Mock } = {}
) {
  const onClose = props.onClose ?? jest.fn();
  const onCreateSession = props.onCreateSession ?? jest.fn().mockResolvedValue(undefined);
  const onNavigateToSession = props.onNavigateToSession ?? jest.fn();
  const utils = render(
    <Omnibar isOpen={true} onClose={onClose} onCreateSession={onCreateSession} onNavigateToSession={onNavigateToSession} />
  );
  const input = screen.getByRole("combobox", { name: /session source input/i });
  return { ...utils, input, onClose, onCreateSession, onNavigateToSession };
}

/** Type a value into the omnibar input and wait for the 150ms detect debounce. */
async function typeAndDetect(input: Element, value: string) {
  fireEvent.change(input, { target: { value } });
  await act(async () => {
    jest.advanceTimersByTime(150);
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Omnibar path completion", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUsePathHistory.mockReturnValue({ ...defaultHistory, getMatching: jest.fn(() => []), save: jest.fn() });
  });

  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  // -------------------------------------------------------------------------
  // Path existence indicator
  // -------------------------------------------------------------------------

  describe("path completion error", () => {
    it("shows 'Could not load completions' when hook returns an error", async () => {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        error: new Error("rpc fail"),
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/proj");
      expect(screen.getByText("Could not load completions")).toBeInTheDocument();
    });
  });

  describe("path existence indicator", () => {
    it("hidden when input is a GitHub URL (not a path input)", async () => {
      const { input } = renderOmnibar();
      await typeAndDetect(input, "https://github.com/owner/repo");
      // indicator span requires isPathInput=true; GitHub URL → false
      expect(screen.queryByText("✓")).not.toBeInTheDocument();
      expect(screen.queryByText("✗")).not.toBeInTheDocument();
      expect(screen.queryByText("⟳")).not.toBeInTheDocument();
    });

    it("shows ✓ when pathExists=true", async () => {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        pathExists: true,
        isLoading: false,
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/proj");
      expect(screen.getByText("✓")).toBeInTheDocument();
    });

    it("shows ✗ when pathExists=false", async () => {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        pathExists: false,
        isLoading: false,
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/nonexistent");
      expect(screen.getByText("✗")).toBeInTheDocument();
    });

    it("shows spinner when isLoading=true", async () => {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        isLoading: true,
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/proj");
      expect(screen.getByText("⟳")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // Dropdown visibility
  // -------------------------------------------------------------------------

  describe("dropdown visibility", () => {
    it("dropdown not rendered for GitHub URL input", async () => {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        entries: [dir("projects")],
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "https://github.com/owner/repo");
      // Even with entries, no dropdown for non-path input types.
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    it("dropdown renders when isPathInput and entries present", async () => {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        entries: [dir("projects"), dir("profile")],
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/p");
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // Keyboard navigation
  // -------------------------------------------------------------------------

  describe("keyboard navigation", () => {
    async function setupWithDropdown(entries: PathEntry[] = [dir("projects"), dir("profile")]) {
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        entries,
        baseDirExists: true,
      });
      const utils = renderOmnibar();
      await typeAndDetect(utils.input, "/home/user/p");
      return utils;
    }

    it("ArrowDown increments dropdownIndex (first entry becomes selected)", async () => {
      await setupWithDropdown();
      fireEvent.keyDown(screen.getByRole("combobox", { name: /session source input/i }), {
        key: "ArrowDown",
      });
      // After ArrowDown from -1 → 0, first option is selected.
      const options = screen.getAllByRole("option");
      expect(options[0].className).toContain("itemSelected");
    });

    it("ArrowUp from -1 stays at -1 (no selection)", async () => {
      await setupWithDropdown();
      fireEvent.keyDown(screen.getByRole("combobox", { name: /session source input/i }), {
        key: "ArrowUp",
      });
      const options = screen.getAllByRole("option");
      options.forEach((opt) => {
        expect(opt.className).not.toContain("itemSelected");
      });
    });

    it("Tab with single entry completes to that entry (appends /)", async () => {
      await setupWithDropdown([dir("projects")]);
      const input = screen.getByRole("combobox", { name: /session source input/i });
      fireEvent.keyDown(input, { key: "Tab" });
      expect((input as HTMLInputElement).value).toBe("/home/user/projects/");
    });

    it("Tab with multiple entries extends input to longest common prefix", async () => {
      // "projects" and "profile" share "pro"
      await setupWithDropdown([dir("projects"), dir("profile")]);
      const input = screen.getByRole("combobox", { name: /session source input/i });
      fireEvent.keyDown(input, { key: "Tab" });
      expect((input as HTMLInputElement).value).toBe("/home/user/pro");
    });

    it("Enter with dropdownIndex >= 0 accepts the selected entry", async () => {
      await setupWithDropdown([dir("projects")]);
      const input = screen.getByRole("combobox", { name: /session source input/i });
      // Select first entry with ArrowDown, then accept with Enter.
      fireEvent.keyDown(input, { key: "ArrowDown" });
      fireEvent.keyDown(input, { key: "Enter" });
      expect((input as HTMLInputElement).value).toBe("/home/user/projects/");
    });

    it("Escape dismisses dropdown, then returns to discovery mode, then closes modal", async () => {
      const { onClose } = await setupWithDropdown();
      const input = screen.getByRole("combobox", { name: /session source input/i });
      // First Escape: dismisses the path completion dropdown, stays in creation mode.
      fireEvent.keyDown(input, { key: "Escape" });
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
      expect(onClose).not.toHaveBeenCalled();
      // Second Escape: returns to discovery mode (creation → discovery transition).
      fireEvent.keyDown(input, { key: "Escape" });
      expect(onClose).not.toHaveBeenCalled();
      // Third Escape: closes modal (now in discovery mode with no selection).
      fireEvent.keyDown(input, { key: "Escape" });
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("aria-activedescendant set after ArrowDown and cleared after Escape", async () => {
      await setupWithDropdown([dir("projects"), dir("profile")]);
      const input = screen.getByRole("combobox", { name: /session source input/i });
      // Before any ArrowDown, attribute is absent.
      expect(input).not.toHaveAttribute("aria-activedescendant");
      // After ArrowDown, points to first option.
      fireEvent.keyDown(input, { key: "ArrowDown" });
      expect(input).toHaveAttribute(
        "aria-activedescendant",
        "path-completion-listbox-option-0"
      );
      // After Escape (dismiss dropdown), attribute is removed.
      fireEvent.keyDown(input, { key: "Escape" });
      expect(input).not.toHaveAttribute("aria-activedescendant");
    });

    it("input onChange resets dropdownIndex to -1", async () => {
      await setupWithDropdown();
      const input = screen.getByRole("combobox", { name: /session source input/i });
      // Move selection down.
      fireEvent.keyDown(input, { key: "ArrowDown" });
      const optsBefore = screen.getAllByRole("option");
      expect(optsBefore[0].className).toContain("itemSelected");
      // Changing input resets dropdownIndex.
      fireEvent.change(input, { target: { value: "/home/user/pr" } });
      const optsAfter = screen.getAllByRole("option");
      optsAfter.forEach((opt) => {
        expect(opt.className).not.toContain("itemSelected");
      });
    });

    it("dropdownDismissed prevents dropdown from showing after Escape", async () => {
      await setupWithDropdown();
      const input = screen.getByRole("combobox", { name: /session source input/i });
      // Dismiss the dropdown.
      fireEvent.keyDown(input, { key: "Escape" });
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
      // Typing more should not re-show the dropdown until a real navigation happens.
      fireEvent.change(input, { target: { value: "/home/user/pr" } });
      // dropdownDismissed is reset by onChange.
      // The dropdown should reappear since entries are still present and dismissed=false.
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // History entries
  // -------------------------------------------------------------------------

  describe("path history", () => {
    it("history entries appear above live entries in the dropdown", async () => {
      const historyPath = "/home/user/projects";
      mockUsePathHistory.mockReturnValue({
        getMatching: jest.fn(() => [
          { path: historyPath, count: 3, lastUsed: Date.now() },
        ]),
        getAll: jest.fn((): PathHistoryEntry[] => []),
        save: jest.fn(),
      });
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        entries: [dir("profile")],
        baseDirExists: true,
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/p");

      const options = screen.getAllByRole("option");
      // First option is the history entry (full path).
      expect(options[0].textContent).toContain(historyPath);
      // Second option is the live entry (name only).
      expect(options[1].textContent).toContain("profile");
    });

    it("live entry deduped when path matches a history entry", async () => {
      const sharedPath = "/home/user/projects";
      mockUsePathHistory.mockReturnValue({
        getMatching: jest.fn(() => [
          { path: sharedPath, count: 1, lastUsed: Date.now() },
        ]),
        getAll: jest.fn((): PathHistoryEntry[] => []),
        save: jest.fn(),
      });
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        entries: [
          // Same path returned by both history and live OS scan.
          { name: "projects", path: sharedPath, isDirectory: true },
          { name: "profile", path: "/home/user/profile", isDirectory: true },
        ],
        baseDirExists: true,
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/p");

      const listbox = screen.getByRole("listbox");
      const options = within(listbox).getAllByRole("option");
      // Only 2 total (history + profile), not 3 (no duplicate projects).
      expect(options).toHaveLength(2);
    });

    it("history entry with no live entries shows dropdown", async () => {
      mockUsePathHistory.mockReturnValue({
        getMatching: jest.fn(() => [
          { path: "/home/user/projects", count: 1, lastUsed: Date.now() },
        ]),
        getAll: jest.fn((): PathHistoryEntry[] => []),
        save: jest.fn(),
      });
      mockUsePathCompletions.mockReturnValue({
        ...defaultCompletions,
        entries: [],
        baseDirExists: true,
      });
      const { input } = renderOmnibar();
      await typeAndDetect(input, "/home/user/p");
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });
  });
});
