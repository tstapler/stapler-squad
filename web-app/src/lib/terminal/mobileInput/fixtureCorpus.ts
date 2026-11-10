/** Loads every committed replay fixture (test-only; reads the filesystem). */
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import type { ReplayFixture } from "./fixtureSchema";

export const FIXTURE_DIR = join(__dirname, "__fixtures__");

export interface CorpusFile {
  name: string;
  text: string;
}

/** Every file under the corpus dir, not an allow-list, so a stray or malformed file is linted too. */
export function readCorpusFiles(): CorpusFile[] {
  return readdirSync(FIXTURE_DIR)
    .sort()
    .map((name) => ({ name, text: readFileSync(join(FIXTURE_DIR, name), "utf8") }));
}

export function loadFixtures(): ReplayFixture[] {
  return readCorpusFiles()
    .filter((f) => f.name.endsWith(".json"))
    .map((f) => JSON.parse(f.text) as ReplayFixture);
}
