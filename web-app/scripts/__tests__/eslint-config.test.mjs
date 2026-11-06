// Guards the rules that next/core-web-vitals used to supply (docs/reference/eslint-rules-from-next.md)
// and the custom boundaries rule, against eslint.config.mjs. Snippets are linted as text so no
// deliberately-bad fixture file lands under src/ where the repo lint would report it.
import assert from "node:assert/strict";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { ESLint } from "eslint";

const cwd = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const eslint = new ESLint({ cwd });
const WARN = 1;
const ERROR = 2;

async function lint(code, file = "src/components/ui/__fixture__.tsx") {
  const [result] = await eslint.lintText(code, { filePath: path.join(cwd, file) });
  return result.messages;
}

const retained = [
  ["react-hooks/rules-of-hooks", ERROR, "export function f() { if (Math.random()) { useThing(); } return null; }\nfunction useThing() { return 1; }\n"],
  [
    "react-hooks/exhaustive-deps",
    WARN,
    'import { useEffect } from "react";\nexport function C({ a }: { a: number }) { useEffect(() => { console.log(a); }, []); return null; }\n',
  ],
  ["jsx-a11y/alt-text", WARN, 'export const C = () => <img src="a.png" />;\n'],
  ["jsx-a11y/aria-props", WARN, 'export const C = () => <div aria-foo="x" />;\n'],
  ["jsx-a11y/aria-proptypes", WARN, 'export const C = () => <div aria-hidden="yes" />;\n'],
  ["jsx-a11y/aria-unsupported-elements", WARN, 'export const C = () => <meta charSet="UTF-8" aria-hidden="false" />;\n'],
  ["jsx-a11y/role-has-required-aria-props", WARN, 'export const C = () => <div role="checkbox" />;\n'],
  ["jsx-a11y/role-supports-aria-props", WARN, 'export const C = () => <div role="link" aria-checked="true" />;\n'],
  ["import/no-anonymous-default-export", WARN, "export default () => null;\n"],
  ["react/jsx-key", ERROR, "export const C = () => <ul>{[1, 2].map((n) => <li>{n}</li>)}</ul>;\n"],
];

describe("eslint.config.mjs retains the rules next/core-web-vitals supplied", () => {
  for (const [ruleId, severity, code] of retained) {
    it(`${ruleId} reports at severity ${severity}`, async () => {
      const hit = (await lint(code)).find((m) => m.ruleId === ruleId);
      assert.ok(hit, `${ruleId} did not fire`);
      assert.equal(hit.severity, severity);
    });
  }

  it("has no @next/next rule ids in the effective config", async () => {
    const config = await eslint.calculateConfigForFile(path.join(cwd, "src/app/page.tsx"));
    const nextRules = Object.keys(config.rules).filter((id) => id.startsWith("@next/next/"));
    assert.deepEqual(nextRules, []);
  });
});

describe("custom rules survive the conversion", () => {
  it("boundaries rejects src/lib importing from src/app", async () => {
    const messages = await lint('import page from "../app/page";\nexport const x = page;\n', "src/lib/__fixture__.ts");
    assert.ok(messages.some((m) => m.ruleId === "boundaries/dependencies" && m.severity === ERROR), JSON.stringify(messages));
  });

  it("no-restricted-syntax still bans bare 100vh", async () => {
    const messages = await lint('export const h = "100vh";\n', "src/components/ui/__fixture__.ts");
    assert.ok(messages.some((m) => m.ruleId === "no-restricted-syntax" && m.severity === ERROR));
  });

  it("analytics/require-page-analytics still loads from the file: plugin", async () => {
    const config = await eslint.calculateConfigForFile(path.join(cwd, "src/app/page.tsx"));
    assert.ok("analytics/require-page-analytics" in config.rules);
    assert.ok("rpc-lifecycle/require-abort-signal" in config.rules);
  });
});
