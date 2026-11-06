import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "../../../../..");
const SCANNED = ["components/sessions", "lib/files"].map((p) => path.join(SRC, p));
const ALLOWED = ["components/sessions/files/engine", "components/sessions/files/extensions"].map((p) => path.join(SRC, p));

// Static, type-only, dynamic and require forms of a forbidden specifier.
const FORBIDDEN = /(?:from\s+|import\s*\(\s*|import\s+|require\s*\(\s*)["'](@codemirror\/[^"']+|@replit\/codemirror-vim|@lezer\/[^"']+|codemirror)["']/g;

function forbiddenImports(source: string): string[] {
  return [...source.matchAll(FORBIDDEN)].map((m) => m[1]);
}

function sourceFiles(dir: string): string[] {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) return e.name === "__tests__" ? [] : sourceFiles(full);
    const isSource = /\.tsx?$/.test(e.name) && !/\.(test|spike)\.tsx?$/.test(e.name) && !e.name.endsWith(".css.ts");
    return isSource ? [full] : [];
  });
}

describe("importBoundary", () => {
  it("detector_should_FlagStaticTypeDynamicAndRequireForms", () => {
    expect(forbiddenImports(`import { a } from "@codemirror/view";`)).toEqual(["@codemirror/view"]);
    expect(forbiddenImports(`const m = await import("@replit/codemirror-vim");`)).toEqual(["@replit/codemirror-vim"]);
    expect(forbiddenImports(`import { basicSetup } from 'codemirror';`)).toEqual(["codemirror"]);
    expect(forbiddenImports(`let v: import("@codemirror/view").EditorView;`)).toEqual(["@codemirror/view"]);
    expect(forbiddenImports(`import x from "@/components/sessions/CodeEditor";`)).toEqual([]);
  });

  it("noCodemirrorOrVimOrLezerImportOutsideEngineAndExtensionsDirectories", () => {
    const offenders = SCANNED.flatMap(sourceFiles)
      .filter((f) => !ALLOWED.some((a) => f.startsWith(a + path.sep)))
      .filter((f) => forbiddenImports(fs.readFileSync(f, "utf8")).length > 0)
      .map((f) => path.relative(SRC, f));
    expect(offenders).toEqual([]);
  });

  it("scan_should_SeeTheEngineDirectory_SoAVacuousPassIsImpossible", () => {
    const engineFiles = ALLOWED.flatMap(sourceFiles);
    expect(engineFiles.some((f) => forbiddenImports(fs.readFileSync(f, "utf8")).length > 0)).toBe(true);
  });
});
