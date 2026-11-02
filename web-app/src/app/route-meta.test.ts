/** ROUTE_META is the only source of titles and guards: it must cover every page.tsx, and no Next-era file may return. */
import * as fs from "fs";
import * as path from "path";
import { NOT_FOUND_META, ROUTE_META } from "./route-meta";
import { ROUTE_PATHS } from "@/lib/nav/paths";

const APP_DIR = __dirname;
const ROOT_TITLE = "Stapler Squad Sessions";

function listPageRoutes(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) listPageRoutes(full, out);
    else if (entry.name === "page.tsx") {
      const rel = path.relative(APP_DIR, dir).split(path.sep).join("/");
      out.push("/" + rel);
    }
  }
  return out;
}

/** Every file under src/app whose name matches, so a Next-era file cannot sneak back in. */
function listFiles(dir: string, name: RegExp, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) listFiles(full, name, out);
    else if (name.test(entry.name)) out.push(full);
  }
  return out;
}

const pageRoutes = listPageRoutes(APP_DIR).map((r) => (r === "/" || r === "" ? "/" : r)).sort();

describe("ROUTE_META", () => {
  it("has a row for every page.tsx and no rows without a page", () => {
    expect(Object.keys(ROUTE_META).sort()).toEqual(pageRoutes);
  });

  it("RoutePath lists exactly the page.tsx routes", () => {
    expect([...ROUTE_PATHS].sort()).toEqual(pageRoutes);
  });

  it("pins well-known titles", () => {
    expect(ROUTE_META["/settings/jules"].title).toBe("Jules - Settings - Stapler Squad");
    expect(ROUTE_META["/changelog"].title).toBe(ROOT_TITLE);
  });

  it("exports the not-found title as data", () => {
    expect(NOT_FOUND_META.title).toBe("Page not found - Stapler Squad");
  });

  it("has no layout.tsx and no metadata exports left from the Next build", () => {
    expect(listFiles(APP_DIR, /^layout\.tsx$/)).toEqual([]);
    // Built from parts so the source itself holds none of the strings it forbids.
    const forbidden = new RegExp(["export const (meta", "data|view", "port)\\b|generate", "Metadata"].join(""));
    const offenders = listFiles(APP_DIR, /^page\.tsx$/).filter((f) => forbidden.test(fs.readFileSync(f, "utf8")));
    expect(offenders).toEqual([]);
  });
});
