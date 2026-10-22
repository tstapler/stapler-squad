import { readFileSync } from "fs";
import { resolve } from "path";

const LIMIT = 350;
const budgeted = ["FilesTab.tsx", "CodeEditor.tsx"];

describe("Files viewer line budget", () => {
  it.each(budgeted)("filesTab_should_StayUnder350Lines_When_AllStoriesLanded: %s", (file) => {
    const lines = readFileSync(resolve(__dirname, "..", file), "utf8").split("\n").length;
    expect({ file, lines, ok: lines <= LIMIT }).toEqual({ file, lines, ok: true });
  });
});
