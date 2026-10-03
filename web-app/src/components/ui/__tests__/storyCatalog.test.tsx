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

// DebugMenu fetches the server log level on open; jsdom has no fetch.
global.fetch = jest.fn(() => Promise.resolve({ json: () => Promise.resolve({ level: "INFO" }) })) as jest.Mock;

jest.mock("@/lib/hooks/useSessionRepoPaths",() => ({ useSessionRepoPaths: () => [] }));
jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({
  useGitHubEnterpriseHosts: () => ({ hosts: [] }),
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

// jsdom lacks scrollIntoView; SlashCommandDropdown calls it on the selected row.
Element.prototype.scrollIntoView = Element.prototype.scrollIntoView ?? (() => {});

const SRC = path.resolve(__dirname, "../../..");
const DIRS = ["components/ui", "components/common"];

// Follow-up: stories still to write (tracked in project_plans/audit-ux-component-library/audit.md).
const UNCATALOGED: Record<string, string> = {
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

  it("UNCATALOGED is empty: new components need a story (an exemption needs a reason and a raised cap)", () => {
    expect(Object.keys(UNCATALOGED).length).toBeLessThanOrEqual(0);
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
