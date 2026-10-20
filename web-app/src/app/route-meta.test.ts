/**
 * Drift test: ROUTE_META must mirror the `metadata` exports and FeatureGuard layouts under src/app.
 * Titles are parsed with the TypeScript compiler API, not by importing Next modules.
 */
import * as fs from "fs";
import * as path from "path";
import * as ts from "typescript";
import { NOT_FOUND_META, ROUTE_META } from "./route-meta";

const APP_DIR = __dirname;
const ROOT_TITLE = "Stapler Squad Sessions";
const FIX_HINT = 'add or fix the row in web-app/src/app/route-meta.ts';

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

function parse(file: string): ts.SourceFile {
  return ts.createSourceFile(file, fs.readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true);
}

/** `metadata.title` string literal, or null. Throws on `generateMetadata`, which this test cannot evaluate. */
function metadataTitle(file: string): string | null {
  if (!fs.existsSync(file)) return null;
  let title: string | null = null;
  parse(file).forEachChild((node) => {
    if (ts.isFunctionDeclaration(node) && node.name?.text === "generateMetadata") {
      throw new Error(`${file} exports generateMetadata; extend route-meta.test.ts to evaluate it, then ${FIX_HINT}`);
    }
    if (!ts.isVariableStatement(node)) return;
    for (const decl of node.declarationList.declarations) {
      if (decl.name.getText() !== "metadata" || !decl.initializer || !ts.isObjectLiteralExpression(decl.initializer)) continue;
      for (const prop of decl.initializer.properties) {
        if (ts.isPropertyAssignment(prop) && prop.name.getText() === "title" && ts.isStringLiteral(prop.initializer)) {
          title = prop.initializer.text;
        }
      }
    }
  });
  return title;
}

/** Flag of a `<FeatureGuard flag="...">` in a layout file, or null. */
function layoutGuard(file: string): string | null {
  if (!fs.existsSync(file)) return null;
  let flag: string | null = null;
  const visit = (node: ts.Node) => {
    if ((ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) && node.tagName.getText() === "FeatureGuard") {
      for (const attr of node.attributes.properties) {
        if (ts.isJsxAttribute(attr) && attr.name.getText() === "flag" && attr.initializer && ts.isStringLiteral(attr.initializer)) {
          flag = attr.initializer.text;
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(parse(file));
  return flag;
}

/** Directories from the route's own directory up to src/app, nearest first. */
function ancestry(route: string): string[] {
  const dirs: string[] = [];
  let dir = path.resolve(APP_DIR, route.slice(1));
  for (;;) {
    dirs.push(dir);
    if (dir === APP_DIR) return dirs;
    dir = path.dirname(dir);
  }
}

function effectiveTitle(route: string): string {
  const own = metadataTitle(path.join(APP_DIR, route, "page.tsx"));
  if (own) return own;
  for (const dir of ancestry(route)) {
    const t = metadataTitle(path.join(dir, "layout.tsx"));
    if (t) return t;
  }
  return ROOT_TITLE;
}

function effectiveGuard(route: string): string | undefined {
  for (const dir of ancestry(route)) {
    const g = layoutGuard(path.join(dir, "layout.tsx"));
    if (g) return g;
  }
  return undefined;
}

const pageRoutes = listPageRoutes(APP_DIR).map((r) => (r === "/" || r === "" ? "/" : r)).sort();

describe("ROUTE_META", () => {
  it("has a row for every page.tsx and no rows without a page", () => {
    expect(Object.keys(ROUTE_META).sort()).toEqual(pageRoutes);
  });

  it.each(pageRoutes)("title for %s equals the effective Next title", (route) => {
    const meta = ROUTE_META[route];
    if (!meta) throw new Error(`No RouteMeta row for ${route}: ${FIX_HINT}`);
    expect({ route, title: meta.title }).toEqual({ route, title: effectiveTitle(route) });
  });

  it.each(pageRoutes)("guard for %s equals the FeatureGuard in its layout chain", (route) => {
    expect({ route, guard: ROUTE_META[route]?.guard }).toEqual({ route, guard: effectiveGuard(route) });
  });

  it("pins well-known titles", () => {
    expect(ROUTE_META["/settings/jules"].title).toBe("Jules - Settings - Stapler Squad");
    expect(ROUTE_META["/changelog"].title).toBe(ROOT_TITLE);
  });

  it("exports the not-found title as data", () => {
    expect(NOT_FOUND_META.title).toBe("Page not found - Stapler Squad");
  });

  it("no layout keeps a bespoke router.replace(\"/\") guard", () => {
    for (const route of pageRoutes) {
      const file = path.join(APP_DIR, route, "layout.tsx");
      if (fs.existsSync(file)) expect(fs.readFileSync(file, "utf8")).not.toContain('router.replace("/")');
    }
  });
});
