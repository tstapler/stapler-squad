// Gate S0 helper: compile three vanilla-extract files under Bun. Run from web-app/:  bun ../project_plans/web-app-bun-build/spikes/ve-compile3.ts
import { createRequire } from "node:module";
import { join } from "node:path";

const cwd = process.cwd();
const { compile, processVanillaFile } = createRequire(join(cwd, "package.json"))("@vanilla-extract/integration");
const files = ["src/styles/theme.css.ts", "src/styles/theme-contract.css.ts", "src/styles/layout.css.ts"];
let failed = 0;
for (const f of files) {
  try {
    const filePath = join(cwd, f);
    const { source } = await compile({ filePath, cwd, identOption: "short" });
    const out = await processVanillaFile({ source, filePath, identOption: "short" });
    console.log(`ok   ${f} -> ${out.length} bytes`);
  } catch (e) {
    failed++;
    console.log(`FAIL ${f}: ${String(e).slice(0, 160)}`);
  }
}
process.exit(failed ? 1 : 0);
