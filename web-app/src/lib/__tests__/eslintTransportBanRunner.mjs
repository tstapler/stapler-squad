// Child process for eslintTransportBan.test.ts: Jest cannot import the ESM flat config in-process.
// Reads [{ id, code, filePath }] from stdin; prints { [id]: ["ruleId: message", ...] } for error-severity findings.
import { ESLint } from "eslint";
import { join } from "node:path";

const root = process.argv[2];
const cases = JSON.parse(await new Response(process.stdin).text());
const eslint = new ESLint({ cwd: root, overrideConfigFile: join(root, "eslint.config.mjs") });
const out = {};
for (const { id, code, filePath } of cases) {
  const [result] = await eslint.lintText(code, { filePath: join(root, filePath) });
  out[id] = result.messages.filter((m) => m.severity === 2).map((m) => `${m.ruleId}: ${m.message}`);
}
process.stdout.write(JSON.stringify(out));
