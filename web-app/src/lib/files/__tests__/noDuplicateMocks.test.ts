import fs from "fs";
import path from "path";
import { findDuplicateMocks, HARNESS_MOCKED_MODULES } from "./fixtures";

const SRC = path.resolve(__dirname, "../../..");
const ROOTS = ["lib/files", "components/sessions/files"].map((p) => path.join(SRC, p));

function testFiles(dir: string): string[] {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) return e.name === "fixtures" ? [] : testFiles(full);
    return /\.test\.tsx?$/.test(e.name) && e.name !== "noDuplicateMocks.test.ts" ? [full] : [];
  });
}

describe("noDuplicateMocks", () => {
  it("noDuplicateMocks_should_FailTheBuild_When_NewFilesTestMocksAModuleTheHarnessAlreadyMocks", () => {
    for (const mod of HARNESS_MOCKED_MODULES) {
      expect(findDuplicateMocks(`jest.mock("${mod}", () => ({}));`)).toEqual([mod]);
      expect(findDuplicateMocks(`jest.doMock('${mod}')`)).toEqual([mod]);
    }
    expect(findDuplicateMocks(`jest.mock("@/lib/other", () => ({}));`)).toEqual([]);
    expect(
      findDuplicateMocks(`import { createFilesTestHarness } from "./fixtures";\nconst h = createFilesTestHarness();`),
    ).toEqual([]);
  });

  it("noDuplicateMocks_should_PassForEveryFilesTestOnDisk", () => {
    const files = ROOTS.flatMap(testFiles);
    const offenders = files.filter((f) => findDuplicateMocks(fs.readFileSync(f, "utf8")).length > 0);
    expect(offenders.map((f) => path.relative(SRC, f))).toEqual([]);
  });
});
