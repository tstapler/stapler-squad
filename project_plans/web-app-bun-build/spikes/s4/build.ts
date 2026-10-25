// Bundles the shared prototype entry (../s2/prototype-entry.tsx) with this directory's react-router 7.18.4.
import { rmSync, mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const here = import.meta.dir;
rmSync(`${here}/dist`, { recursive: true, force: true });
mkdirSync(`${here}/dist`);
const result = await Bun.build({
  entrypoints: [resolve(here, "../s2/prototype-entry.tsx")],
  outdir: `${here}/dist`,
  naming: "main.js",
  splitting: false,
  minify: false,
  define: { "process.env.NODE_ENV": '"production"' },
  plugins: [
    {
      name: "resolve-from-s4",
      setup(b) {
        b.onResolve({ filter: /^(react|react-dom|react-router)(\/.*)?$/ }, (args) => ({
          path: Bun.resolveSync(args.path, here),
        }));
      },
    },
  ],
});
if (!result.success) {
  console.error(result.logs);
  process.exit(1);
}
writeFileSync(
  `${here}/dist/index.html`,
  `<!doctype html><html><head><meta charset="utf-8"><title>boot</title></head><body><div id="root"></div><script type="module" src="/main.js"></script></body></html>`,
);
console.log("built", result.outputs.map((o) => `${o.path} ${o.size}`));
