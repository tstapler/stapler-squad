/** @jest-environment node */
import { spawnSync } from "child_process";
import { join } from "path";

const ROOT = join(__dirname, "..", "..", "..");
type Case = { code: string; filePath: string };

// One child process lints every case: loading the plugin-heavy flat config is the slow part.
function lintAll(cases: Record<string, Case>): Record<string, string[]> {
  const input = Object.entries(cases).map(([id, c]) => ({ id, ...c }));
  const run = spawnSync(process.execPath, [join(__dirname, "eslintTransportBanRunner.mjs"), ROOT], {
    cwd: ROOT,
    input: JSON.stringify(input),
    encoding: "utf8",
    maxBuffer: 16 * 1024 * 1024,
  });
  if (run.status !== 0) throw new Error(`eslint runner failed: ${run.stderr}`);
  return JSON.parse(run.stdout);
}

const STATIC = `import { createConnectTransport } from "@connectrpc/connect-web";\nexport const t = createConnectTransport;\n`;
const DYNAMIC = `export const load = () => import("@connectrpc/connect-web");\n`;
const transportBan = (errs: string[]) => errs.filter((e) => /unary transports only in lib\/api\/transport\.ts/.test(e));

const CODEMIRROR = `import { EditorView } from "@codemirror/view";\nexport const v = EditorView;\n`;
const NAV = `import { useRouter } from "next/navigation";\nexport const r = useRouter;\n`;
const FILES_UI = "src/components/sessions/files/Foo.tsx";
const HOOK = "src/lib/hooks/useFoo.ts";

jest.setTimeout(120_000);

const results = (() => {
  const cases: Record<string, Case> = {
    staticHook: { code: STATIC, filePath: HOOK },
    staticFilesUi: { code: STATIC, filePath: FILES_UI },
    dynamicHook: { code: DYNAMIC, filePath: HOOK },
    staticTransport: { code: STATIC, filePath: "src/lib/api/transport.ts" },
    dynamicTransport: { code: DYNAMIC, filePath: "src/lib/api/transport.ts" },
    staticTransportDir: { code: STATIC, filePath: "src/lib/transport/foo.ts" },
    staticTestFile: { code: STATIC, filePath: "src/lib/hooks/useFoo.test.ts" },
    staticTestsDir: { code: STATIC, filePath: "src/lib/hooks/__tests__/useFoo.ts" },
    codemirror: { code: CODEMIRROR, filePath: FILES_UI },
    mixed: { code: STATIC + CODEMIRROR, filePath: FILES_UI },
    nav: { code: NAV, filePath: HOOK },
  };
  return lintAll(cases);
})();

describe("eslint transport ban", () => {
  it("noRestrictedImports_should_Error_When_CreateConnectTransportImportedOutsideSharedModule", () => {
    expect(transportBan(results.staticHook)).toHaveLength(1);
    expect(transportBan(results.staticFilesUi)).toHaveLength(1);
    expect(transportBan(results.dynamicHook)).toHaveLength(1);
  });

  it("noRestrictedImports_should_NotError_When_ImporterIsTestFileOrNextNavigationBanPreserved", () => {
    for (const id of ["staticTransport", "dynamicTransport", "staticTransportDir", "staticTestFile", "staticTestsDir"]) {
      expect(transportBan(results[id])).toEqual([]);
    }
    // The pre-existing rules still fire next to the new one.
    expect(results.codemirror.some((e) => /ADR-036/.test(e))).toBe(true);
    expect(transportBan(results.mixed)).toHaveLength(1);
    expect(results.mixed.some((e) => /ADR-036/.test(e))).toBe(true);
    expect(results.nav.some((e) => /nav/.test(e))).toBe(true);
  });
});
