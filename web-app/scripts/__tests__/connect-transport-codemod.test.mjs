// Liveness checks for the createConnectTransport -> getConnectTransport codemod. Plain `node --test` (test:codemods).
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, describe, it } from "node:test";
import { run, translateCall } from "../codemods/connect-transport-to-shared.mjs";

const scratch = mkdtempSync(path.join(tmpdir(), "connect-transport-"));
after(() => rmSync(scratch, { recursive: true, force: true }));

const write = (name, text) => {
  const file = path.join(scratch, name);
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, text);
  return file;
};

describe("connect-transport-to-shared codemod", () => {
  it("translateCall maps default options and flags the rest", () => {
    assert.equal(translateCall("{ baseUrl: getApiBaseUrl() }"), "getConnectTransport()");
    assert.equal(translateCall("{ baseUrl: getApiBaseUrl(), interceptors: [createAuthInterceptor()] }"), "getConnectTransport()");
    assert.equal(translateCall("{ baseUrl }"), "getConnectTransport({ baseUrl })");
    assert.equal(translateCall("{ baseUrl: props.url }"), "getConnectTransport({ baseUrl: props.url })");
    assert.equal(translateCall("{ baseUrl, useBinaryFormat: true }"), null);
    assert.equal(translateCall("{ baseUrl, fetch: f }"), null);
    assert.equal(translateCall("{ baseUrl, interceptors: [a(), b()] }"), null);
  });

  it("codemodDryRun_should_ReportCallers_And_FlagNonDefaultOptions", () => {
    const clean = write(
      "clean.tsx",
      `import { createConnectTransport } from "@connectrpc/connect-web";\nimport { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";\nconst t = createConnectTransport({ baseUrl: getApiBaseUrl(), interceptors: [createAuthInterceptor()] });\n`,
    );
    write(
      "custom.ts",
      `import { createConnectTransport } from "@connectrpc/connect-web";\nconst t = createConnectTransport({ baseUrl: "x", useBinaryFormat: true });\n`,
    );
    write("skipped.test.ts", `createConnectTransport({ baseUrl: "x" });\n`);
    const before = readFileSync(clean, "utf8");
    const r = run([scratch], { check: true });
    assert.equal(r.callerFiles.length, 2);
    assert.equal(r.converted, 1);
    assert.equal(r.nonDefault.length, 1);
    assert.match(r.nonDefault[0], /custom\.ts:2/);
    assert.equal(readFileSync(clean, "utf8"), before, "dry run must not write");
  });

  it("rewrites calls, prunes dead imports, and is idempotent", () => {
    const file = write(
      "app/page.tsx",
      `import { createClient } from "@connectrpc/connect";\nimport { createConnectTransport } from "@connectrpc/connect-web";\nimport { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";\nexport const c = createClient(S, createConnectTransport({ baseUrl: getApiBaseUrl(), interceptors: [createAuthInterceptor()] }));\n`,
    );
    run([path.join(scratch, "app")]);
    const out = readFileSync(file, "utf8");
    assert.match(out, /createClient\(S, getConnectTransport\(\)\)/);
    assert.match(out, /import \{ getConnectTransport \} from "@\/lib\/api\/transport";/);
    assert.doesNotMatch(out, /connect-web|@\/lib\/config/);
    assert.equal(run([path.join(scratch, "app")]).changed.length, 0);
  });

  it("keeps config imports still used elsewhere in the file", () => {
    const file = write(
      "keep/a.tsx",
      `import { createConnectTransport } from "@connectrpc/connect-web";\nimport { getApiBaseUrl } from "@/lib/config";\nconst url = getApiBaseUrl();\nconst t = createConnectTransport({ baseUrl: getApiBaseUrl() });\n`,
    );
    run([path.join(scratch, "keep")]);
    assert.match(readFileSync(file, "utf8"), /import \{ getApiBaseUrl \} from "@\/lib\/config";/);
  });
});
