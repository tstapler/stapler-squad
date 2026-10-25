// The facade lint ban (Task 2.9a). Plain node:test: ESLint's plugin resolution does not work inside Jest's module sandbox.
import assert from "node:assert/strict";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { ESLint } from "eslint";

const webApp = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const eslint = new ESLint({ cwd: webApp });

async function messages(code, file) {
  const [result] = await eslint.lintText(code, { filePath: path.join(webApp, file) });
  return result.messages;
}

const banned = (msgs) => msgs.filter((m) => m.ruleId === "@typescript-eslint/no-restricted-imports");
const rawHistory = (msgs) => msgs.filter((m) => m.ruleId === "no-restricted-properties" && m.message.includes("Raw History"));
const LINK_IMPORT = 'import Link from "next/link";\nexport const x = Link;\n';

describe("navigation facade lint ban", () => {
  it("errors (severity 2) on next/link and points at the facade and the codemod", async () => {
    const found = banned(await messages(LINK_IMPORT, "src/components/Example.tsx"));
    assert.equal(found.length, 1);
    assert.equal(found[0].severity, 2);
    assert.match(found[0].message, /@\/lib\/nav/);
    assert.match(found[0].message, /next-nav-to-facade\.mjs/);
  });

  for (const module of ["next/navigation", "next/dynamic", "next/dist/client/web-vitals", "react-router-dom"]) {
    it(`errors on ${module}`, async () => {
      const found = banned(await messages(`import * as m from "${module}";\nexport const x = m;\n`, "src/lib/example.ts"));
      assert.equal(found.length, 1);
      assert.equal(found[0].severity, 2);
    });
  }

  it("allows type-only next imports (Metadata) and the facade itself", async () => {
    const source = 'import type { Metadata } from "next";\nimport { Link } from "@/lib/nav";\nexport const x: Metadata | typeof Link | null = null;\n';
    assert.deepEqual(banned(await messages(source, "src/app/example/page.tsx")), []);
  });

  for (const file of ["src/lib/nav/impl.next.tsx", "src/lib/lazy/impl.next.tsx", "src/lib/foo.test.ts", "src/lib/__tests__/foo.ts"]) {
    it(`does not apply inside ${file}`, async () => {
      assert.deepEqual(banned(await messages(LINK_IMPORT, file)), []);
    });
  }

  it("allows the router entry only from src/main.tsx and tests", async () => {
    const source = 'import { createAppRouter } from "@/lib/nav/entry";\nexport const x = createAppRouter;\n';
    assert.equal(banned(await messages(source, "src/app/routes.tsx")).length, 1);
    assert.equal(banned(await messages(source, "src/components/Example.tsx")).length, 1);
    assert.deepEqual(banned(await messages(source, "src/main.tsx")), []);
    assert.deepEqual(banned(await messages(source, "src/lib/nav/__tests__/entry.test.tsx")), []);
  });

  it("still bans react-router in src/main.tsx and src/app/routes.tsx", async () => {
    const source = 'import { Outlet } from "react-router";\nexport const x = Outlet;\n';
    assert.equal(banned(await messages(source, "src/main.tsx")).length, 1);
    assert.equal(banned(await messages(source, "src/app/routes.tsx")).length, 1);
  });

  it("src/main.tsx, entry.tsx and routes.tsx lint clean with no new exclusion", async () => {
    const results = await eslint.lintFiles(["src/main.tsx", "src/lib/nav/entry.tsx", "src/app/routes.tsx"]);
    const errors = results.flatMap((r) => r.messages.filter((m) => m.severity === 2).map((m) => `${r.filePath}: ${m.ruleId}: ${m.message}`));
    assert.deepEqual(errors, []);
  });

  it("flags raw history.pushState/replaceState outside lib/nav, but not inside it or in tests", async () => {
    const source = 'export const go = () => { window.history.replaceState(null, "", "?a=b"); window.history.pushState({}, ""); };\n';
    assert.equal(rawHistory(await messages(source, "src/lib/hooks/example.ts")).length, 2);
    assert.equal(rawHistory(await messages(source, "src/components/Example.tsx")).length, 2);
    assert.ok(rawHistory(await messages(source, "src/components/Example.tsx")).every((m) => m.severity === 2));
    assert.equal(rawHistory(await messages(source, "src/lib/nav/history.ts")).length, 0);
    assert.equal(rawHistory(await messages(source, "src/lib/hooks/__tests__/example.test.ts")).length, 0);
  });
});
