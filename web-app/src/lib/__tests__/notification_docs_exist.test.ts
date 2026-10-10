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
});
