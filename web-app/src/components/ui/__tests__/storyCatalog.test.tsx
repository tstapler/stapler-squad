/**
 * Component-library gate: every story renders, and every shared component in
 * components/ui + components/common is either cataloged in a story or listed
 * in UNCATALOGED with a reason (so the gap is visible, not silent).
 */
import fs from "fs";
import path from "path";
import React from "react";
import { render } from "@testing-library/react";
import { composeStories } from "@storybook/react";

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({ useSessionRepoPaths: () => [] }));
jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({
  useGitHubEnterpriseHosts: () => ({ hosts: [] }),
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

const SRC = path.resolve(__dirname, "../../..");
const DIRS = ["components/ui", "components/common"];

// Follow-up: stories still to write (tracked in project_plans/audit-ux-component-library/audit.md).
const UNCATALOGED: Record<string, string> = {
  ActionBar: "follow-up: needs session fixtures",
  AliasPalette: "follow-up: needs alias fixtures",
  AppLink: "re-export of next/link, nothing to catalog",
  AtCommandDropdown: "follow-up: omnibar-internal",
  AutocompleteInput: "follow-up",
  AvailableFlags: "follow-up: flag-picker internal",
  Collapsible: "follow-up",
  DebugMenu: "dev-only",
  ErrorBoundary: "behavioral wrapper, no visual states",
  EstimatedValue: "follow-up",
  FlagCombobox: "follow-up",
  FlagInfoButton: "follow-up",
  KeyboardHint: "follow-up",
  LiveRegion: "a11y utility, no visual states",
  NavBadge: "follow-up: needs store",
  Navigation: "follow-up: needs router",
  NotificationItem: "follow-up: needs notification fixtures",
  NotificationPanel: "follow-up: needs store",
  NotificationToast: "follow-up: needs notification fixtures",
  NotificationsNavBadge: "follow-up: needs store",
  PathCompletionDropdown: "internal to RepoPathInput (covered by its stories)",
  ProbeStatusBadge: "follow-up: needs probe fixtures",
  SlashCommandDropdown: "follow-up: omnibar-internal",
  SystemBanner: "follow-up: needs store",
  UnknownFlagsWarning: "follow-up: flag-picker internal",
};

function componentFiles(dir: string): string[] {
  return fs
    .readdirSync(path.join(SRC, dir))
    .filter((f) => /^[A-Z][A-Za-z0-9]*\.tsx$/.test(f) && !/\.(test|stories)\.tsx$/.test(f))
    .map((f) => f.replace(/\.tsx$/, ""));
}

describe("component library catalog", () => {
  const storyFiles = DIRS.flatMap((d) =>
    fs.readdirSync(path.join(SRC, d)).filter((f) => f.endsWith(".stories.tsx")).map((f) => `${d}/${f}`)
  );

  it("has stories for the core shared components, including the rich path field", () => {
    const names = storyFiles.map((f) => path.basename(f, ".stories.tsx"));
    expect(names).toEqual(
      expect.arrayContaining(["Button", "Badge", "Card", "Input", "Modal", "RepoPathInput", "InlineNotice"])
    );
  });

  it("every shared component is cataloged or has a documented exemption", () => {
    const cataloged = new Set(storyFiles.map((f) => path.basename(f, ".stories.tsx")));
    const missing = DIRS.flatMap(componentFiles).filter((c) => !cataloged.has(c) && !(c in UNCATALOGED));
    expect(missing).toEqual([]);
  });

  it("UNCATALOGED can only shrink (ratchet: lower this number when stories are added)", () => {
    expect(Object.keys(UNCATALOGED).length).toBeLessThanOrEqual(25);
  });

  it("UNCATALOGED has no stale entries", () => {
    const cataloged = new Set(storyFiles.map((f) => path.basename(f, ".stories.tsx")));
    const all = new Set(DIRS.flatMap(componentFiles));
    const stale = Object.keys(UNCATALOGED).filter((c) => cataloged.has(c) || !all.has(c));
    expect(stale).toEqual([]);
  });

  describe.each(storyFiles)("%s", (file) => {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const mod = require(path.join(SRC, file));
    const stories = composeStories(mod) as Record<string, React.ComponentType>;
    it.each(Object.keys(stories))("story %s renders real content", (name) => {
      const Story = stories[name];
      // baseElement: Modal portals out of the container
      const { baseElement } = render(<Story />);
      // RTL's own container div is always present; a story must add at least one element beyond it.
      expect(baseElement.querySelectorAll("body *").length).toBeGreaterThan(1);
    });
  });
});
