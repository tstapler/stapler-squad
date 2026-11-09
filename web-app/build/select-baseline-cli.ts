// CI entry: reads artifact records (one JSON object per line) on stdin, prints the id of the main-branch
// artifact built from <base sha>, or nothing when there is none. Usage: bun build/select-baseline-cli.ts <sha>
import { readFileSync } from "node:fs";
import { selectBaselineArtifactId, type ArtifactRecord } from "./compare-stats";

const baseSha = process.argv[2];
if (!baseSha) {
  console.error("usage: select-baseline-cli.ts <base sha> < artifacts.jsonl");
  process.exit(2);
}
const records = readFileSync(0, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l) as ArtifactRecord);
const id = selectBaselineArtifactId(records, baseSha);
if (id !== undefined) console.log(id);
