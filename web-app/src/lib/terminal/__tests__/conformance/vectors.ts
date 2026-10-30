/** Loads the shared VT conformance vectors from tests/terminal-conformance/ (see docs/how-to/vt-conformance-runbook.md). */
import { readFileSync } from "fs";
import { join } from "path";

const VECTOR_DIR = join(__dirname, "../../../../../../tests/terminal-conformance");

export interface Vector {
  name: string;
  description: string;
  cols: number;
  rows: number;
  input: string;
  expectedScreen: string;
  /** Terminal-to-host bytes the vector must provoke; undefined when the vector only checks the screen. */
  expectedResponse?: string;
}

export interface Outcome {
  screen: string;
  response: string;
}

export interface Baseline {
  /** Vectors that fail on this renderer today; the suite fails if one starts passing, so the table cannot rot. */
  knownFailing: string[];
}

const read = (file: string): string => readFileSync(join(VECTOR_DIR, file), "utf8");

export function loadVectors(): Vector[] {
  const manifest = JSON.parse(read("manifest.json")) as {
    vectors: { name: string; description: string; cols: number; rows: number; checksResponse: boolean }[];
  };
  return manifest.vectors.map((v) => ({
    name: v.name,
    description: v.description,
    cols: v.cols,
    rows: v.rows,
    input: read(`${v.name}.ans`),
    expectedScreen: read(`${v.name}.expected.txt`),
    expectedResponse: v.checksResponse ? read(`${v.name}.response.txt`) : undefined,
  }));
}

export function loadBaseline(renderer: "xterm" | "ghostty"): Baseline {
  return (JSON.parse(read("baseline.json")) as Record<string, Baseline>)[renderer];
}

/** Rows right-trimmed, trailing blank rows dropped, newline-terminated — the `.expected.txt` format. */
export function normalizeScreen(rows: string[]): string {
  const trimmed = rows.map((r) => r.replace(/\s+$/, ""));
  while (trimmed.length > 0 && trimmed[trimmed.length - 1] === "") trimmed.pop();
  return trimmed.map((r) => `${r}\n`).join("");
}

export function vectorPasses(v: Vector, got: Outcome): boolean {
  if (got.screen !== v.expectedScreen) return false;
  return v.expectedResponse === undefined || got.response === v.expectedResponse;
}
