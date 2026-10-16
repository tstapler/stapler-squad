// T-E2-31: the notification how-to and delivery-gate reference exist under docs/ (Diataxis
// placement, never .claude/docs) and every relative link in them resolves.
import fs from "fs";
import path from "path";

const REPO = path.join(process.cwd(), "..");
const DOCS = [
  "docs/how-to/manage-notifications.md",
  "docs/reference/notification-delivery-gate.md",
];

function relativeLinks(markdown: string): string[] {
  const links: string[] = [];
  for (const m of markdown.matchAll(/\[[^\]]*\]\(([^)\s]+)\)/g)) {
    const target = m[1].split("#")[0];
    if (target && !/^[a-z][a-z0-9+.-]*:/i.test(target) && !target.startsWith("/")) links.push(target);
  }
  return links;
}

describe("notification docs", () => {
  it.each(DOCS)("%s exists and is not under .claude/docs", (rel) => {
    expect(rel).not.toMatch(/\.claude\/docs/);
    expect(fs.existsSync(path.join(REPO, rel))).toBe(true);
  });

  it.each(DOCS)("%s has only resolvable relative links", (rel) => {
    const file = path.join(REPO, rel);
    const dead = relativeLinks(fs.readFileSync(file, "utf8")).filter(
      (link) => !fs.existsSync(path.resolve(path.dirname(file), link)),
    );
    expect(dead).toEqual([]);
  });

  it("the project does not keep notification docs under .claude/docs", () => {
    expect(fs.existsSync(path.join(REPO, ".claude", "docs"))).toBe(false);
  });

  // T-RP-82: the Reply release-note line stays in the how-to, so the headline dead end is
  // not presented as fixed for questions or sessions it cannot answer.
  it("the how-to carries the Reply release-note line", () => {
    const howTo = fs.readFileSync(path.join(REPO, "docs/how-to/manage-notifications.md"), "utf8");
    expect(howTo).toContain(
      "Reply answers a single-select question from a background session with one tap; other questions, and sessions whose hook has not yet been refreshed (a service restart or session resume refreshes it, no agent restart needed), show 'Answer in the terminal'. Free text is not supported.",
    );
  });
});
