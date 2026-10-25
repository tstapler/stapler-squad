// Bun-side checks for VeCompiler. Usage: vanilla-extract.bun.ts <scenario>; prints one JSON object.
import { getSourceFromVirtualCssFile } from "@vanilla-extract/integration";
import { mkdirSync, rmSync, utimesSync, writeFileSync } from "node:fs";
import path from "node:path";
import { VeCompiler } from "../../vanilla-extract";

const webApp = process.cwd();
const scenario = process.argv[2];
const work = path.join(webApp, ".build", `ve-${process.pid}`);

const VIRTUAL_CSS = /["'][^"']*\.vanilla\.css\?source=[^"']+["']/g;
async function decodeCss(js: string): Promise<string> {
  const parts: string[] = [];
  for (const m of js.matchAll(VIRTUAL_CSS)) parts.push((await getSourceFromVirtualCssFile(m[0].slice(1, -1))).source);
  return parts.join("\n");
}

function tokens(color: string): string {
  return `import { createTheme } from "@vanilla-extract/css";\nexport const [themeClass, vars] = createTheme({ c: { fg: "${color}" } });\n`;
}
const FOO = `import { style } from "@vanilla-extract/css";\nimport { vars } from "./tokens.css";\nexport const foo = style({ color: vars.c.fg });\n`;

async function run(): Promise<unknown> {
  mkdirSync(work, { recursive: true });
  if (scenario === "output") {
    const file = path.join(webApp, "build/__tests__/fixtures/app/src/styles/theme.css.ts");
    const out: Record<string, unknown> = {};
    for (const identOption of ["short", "debug"] as const) {
      const ve = new VeCompiler({ cwd: webApp, identOption });
      const js = await ve.load(file);
      out[identOption] = { exports: await ve.classExports(file), css: await decodeCss(js) };
    }
    return out;
  }
  if (scenario === "canary") {
    const ve = new VeCompiler({ cwd: webApp, identOption: "debug" });
    const file = path.join(webApp, "build/__tests__/fixtures/canary/canary.css.ts");
    return { exports: await ve.classExports(file), css: await decodeCss(await ve.load(file)) };
  }
  if (scenario === "invalidate") {
    const tok = path.join(work, "tokens.css.ts");
    const foo = path.join(work, "Foo.css.ts");
    writeFileSync(tok, tokens("#111111"));
    writeFileSync(foo, FOO);
    const ve = new VeCompiler({ cwd: webApp });
    const steps: { name: string; compiles: number; css: string }[] = [];
    const step = async (name: string) => {
      const css = await decodeCss(await ve.load(foo));
      steps.push({ name, compiles: ve.compileCount, css });
    };
    await step("first");
    await step("unchanged");
    const later = new Date(Date.now() + 60_000);
    utimesSync(tok, later, later);
    await step("touched");
    writeFileSync(tok, tokens("#222222"));
    await step("edited-dependency");
    return { steps };
  }
  if (scenario === "lru") {
    const ve = new VeCompiler({ cwd: webApp, maxEntries: 2 });
    const files = ["a", "b", "c"].map((n, i) => {
      const f = path.join(work, `${n}.css.ts`);
      writeFileSync(f, tokens(`#00000${i}`));
      return f;
    });
    for (const f of files) await ve.load(f);
    const before = ve.compileCount;
    await ve.load(files[2]);
    const hitLatest = ve.compileCount === before;
    await ve.load(files[0]);
    return { cacheSize: ve.cacheSize, hitLatest, evictedOldestRecompiled: ve.compileCount === before + 1 };
  }
  if (scenario === "throws") {
    const bad = path.join(work, "bad.css.ts");
    writeFileSync(bad, 'throw new Error("kaboom");\n');
    const ve = new VeCompiler({ cwd: webApp });
    try {
      await ve.load(bad);
      return { err: null, cacheSize: ve.cacheSize };
    } catch (e) {
      return { err: String(e), cacheSize: ve.cacheSize, bad };
    }
  }
  throw new Error(`unknown scenario ${scenario}`);
}

try {
  console.log(JSON.stringify(await run()));
} finally {
  rmSync(work, { recursive: true, force: true });
}
