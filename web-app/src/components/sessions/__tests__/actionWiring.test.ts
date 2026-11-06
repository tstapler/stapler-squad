/**
 * Wiring guards: a handler added to a prop surface but dropped at one call site
 * silently removes a menu item or card action (Pin/Unpin on the board, Hibernate
 * in the pane header). Source parsing avoids rendering these heavy components.
 */
import { readFileSync } from "fs";
import { resolve } from "path";

const read = (rel: string) => readFileSync(resolve(__dirname, "..", rel), "utf8");
const readPane = (rel: string) => readFileSync(resolve(__dirname, "../../pane", rel), "utf8");

/** Names of `onXxx` members declared inside `interface <name> { ... }`. */
function interfaceHandlers(source: string, name: string): string[] {
  const start = source.indexOf(`interface ${name}`);
  if (start < 0) throw new Error(`interface ${name} not found`);
  const open = source.indexOf("{", start);
  const body = source.slice(open, source.indexOf("\n}", open));
  return [...body.matchAll(/^\s{2}(on[A-Z]\w*)\??:/gm)].map((m) => m[1]);
}

/** Text of the first `<SessionActionsOverflow ... />` element (up to its self-closing `/>`). */
function overflowElement(source: string): string {
  const start = source.search(/<SessionActionsOverflow\s/);
  if (start < 0) throw new Error("no <SessionActionsOverflow> element");
  const rest = source.slice(start);
  const end = rest.search(/\n\s*\/>/);
  if (end < 0) throw new Error("could not find end of <SessionActionsOverflow> element");
  return rest.slice(0, end);
}

describe("SessionActionsOverflow call sites supply every callback", () => {
  const overflowProps = interfaceHandlers(read("SessionActionsOverflow.tsx"), "SessionActionsOverflowProps");

  // Props intentionally not supplied by a call site, with the reason. Empty: every site supplies all.
  const exempt: Record<string, Record<string, string>> = {
    "SessionCard.tsx": {},
    "SessionRow.tsx": {},
    "PaneHeader.tsx": {},
  };
  const sites: Array<[string, () => string]> = [
    ["SessionCard.tsx", () => read("SessionCard.tsx")],
    ["SessionRow.tsx", () => read("SessionRow.tsx")],
    ["PaneHeader.tsx", () => readPane("PaneHeader.tsx")],
  ];

  it("parses a plausible prop list", () => {
    expect(overflowProps.length).toBeGreaterThan(15);
  });

  it.each(sites)("%s", (file, load) => {
    const element = overflowElement(load());
    const missing = overflowProps.filter(
      (p) => !new RegExp(`\\b${p}=`).test(element) && !(p in exempt[file]),
    );
    expect(missing).toEqual([]);
  });
});

describe("SessionBoard getCardProps forwards every SessionList action handler", () => {
  const handlers = interfaceHandlers(read("SessionList.tsx"), "SessionListActionHandlers");
  // Handlers BoardCard has no per-card use for, with the reason.
  const exempt: Record<string, string> = {
    onDirectResumeSession: "bulk-resume only; board has no bulk resume",
    onNewSession: "list-header button; board has none",
  };

  it("parses a plausible handler list", () => {
    expect(handlers.length).toBeGreaterThan(15);
  });

  it("every handler is read in getCardProps", () => {
    const source = read("SessionBoard.tsx");
    const start = source.indexOf("const getCardProps");
    const block = source.slice(start, source.indexOf("handleToggleSession(session.id)", start));
    const missing = handlers.filter((h) => !(h in exempt) && !new RegExp(`\\b${h}\\b`).test(block));
    expect(missing).toEqual([]);
  });
});
