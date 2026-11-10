// tools/docscoverage parses DOC_FILES from docLoader.ts as a plain array of
// string literals (no Next or Astro build). This test fails in a web-app-only PR
// that turns it into anything else, mirroring ParseDocFiles in Go.
import fs from "fs";
import path from "path";
import { DOC_FILES } from "@/lib/docs/docLoader";

const SOURCE = fs.readFileSync(path.join(process.cwd(), "src/lib/docs/docLoader.ts"), "utf8");

function parseDocFilesLiteral(src: string): string[] {
  const m = /export\s+const\s+DOC_FILES\s*(?::[^=]+?)?=\s*\[([\s\S]*?)\]/.exec(src);
  if (!m) throw new Error("DOC_FILES is no longer an `export const DOC_FILES = [...]` literal");
  const body = m[1].replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");
  const literal = /"([^"\n]*)"|'([^'\n]*)'/g;
  const slugs = [...body.matchAll(literal)].map((x) => x[1] ?? x[2]);
  const leftover = body.replace(literal, "").replace(/[,\s]/g, "");
  if (leftover !== "") {
    throw new Error(`DOC_FILES is no longer an array of string literals (found "${leftover}"); fix tools/docscoverage's parser or keep it a literal`);
  }
  return slugs;
}

describe("DOC_FILES shape (read by tools/docscoverage)", () => {
  it("is an array of string literals equal to the runtime value", () => {
    expect(parseDocFilesLiteral(SOURCE)).toEqual(DOC_FILES);
  });

  it("rejects a spread or computed entry", () => {
    expect(() => parseDocFilesLiteral('export const DOC_FILES = [...OTHER, "a"];')).toThrow(/string literals/);
    expect(() => parseDocFilesLiteral("export const X = [];")).toThrow(/no longer/);
  });
});
