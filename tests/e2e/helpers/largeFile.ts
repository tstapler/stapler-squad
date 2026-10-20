// Deterministic large-file fixtures for the Files e2e specs (Task 1.6.7a). Line n is `L<7-digit n>` padded with `x` to
// `width - 1` bytes plus "\n", so the byte size is exactly lines * width and the content of any line is derivable.
import fs from "fs";
import path from "path";

export const pad = (n: number) => String(n).padStart(7, "0");

export interface LogSpec {
  name: string;
  lines: number;
  width: number;
  /** Line number to replace with the given text (padded the same way), e.g. a search needle. */
  needles?: Record<number, string>;
}

export function writeNumberedLog(dir: string, spec: LogSpec): string {
  const file = path.join(dir, spec.name);
  fs.rmSync(file, { force: true });
  for (let from = 1; from <= spec.lines; from += 100_000) {
    const count = Math.min(100_000, spec.lines - from + 1);
    const chunk = Array.from({ length: count }, (_, i) => {
      const n = from + i;
      return (spec.needles?.[n] ?? `L${pad(n)}`).padEnd(spec.width - 1, "x") + "\n";
    }).join("");
    fs.appendFileSync(file, chunk);
  }
  return file;
}

/** ~5 MB and ~50 MB are T1 (under the 64 MiB hash cap); ~200 MB is T2 (view-only). */
export const FIXTURES = {
  mb5: { name: "t1-5mb.log", lines: 500_000, width: 10 },
  mb50: { name: "t1-50mb.log", lines: 900_000, width: 56, needles: { 640_000: "NEEDLE_MID_640000" } },
  mb200: { name: "t2-200mb.log", lines: 3_000_000, width: 70 },
} satisfies Record<string, LogSpec>;
