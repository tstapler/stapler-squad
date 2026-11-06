import { readdirSync, readFileSync, statSync } from "fs";
import { join, relative } from "path";

const SRC = join(__dirname, "..", "..", "..");
const isTestPath = (p: string) => /(^|\/)__tests__\//.test(p) || /\.(test|spec)\.[tj]sx?$/.test(p);

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) return name === "gen" ? [] : sourceFiles(full);
    return /\.[tj]sx?$/.test(name) ? [full] : [];
  });
}

describe("createConnectTransport callers", () => {
  it("createConnectTransportCallers_should_EqualPinnedFourFiles_When_NonTestSourcesScanned", () => {
    const callers = sourceFiles(SRC)
      .map((f) => relative(SRC, f))
      .filter((rel) => !isTestPath(rel))
      .filter((rel) => readFileSync(join(SRC, rel), "utf8").includes("createConnectTransport"))
      .sort();
    expect(callers).toEqual([
      "components/backlog/backlogItemDetailTestFixtures.ts",
      "lib/api/transport.ts",
      "lib/transport/watch-ws-transport.ts",
      "lib/utils/backoff.ts",
    ]);
  });
});
