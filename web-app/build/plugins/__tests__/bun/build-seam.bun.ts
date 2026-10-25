// Bundles the seam fixture entries with the build-seam plugin. Usage: build-seam.bun.ts <entry-file-name>.
import path from "node:path";
import { buildSeamPlugin } from "../../build-seam";

const src = path.join(process.cwd(), "build/plugins/__tests__/fixtures/seam/src");
const entry = process.argv[2];

try {
  const res = await Bun.build({ entrypoints: [path.join(src, entry)], target: "browser", throw: true, plugins: [buildSeamPlugin(src)] });
  const text = await res.outputs[0].text();
  console.log(JSON.stringify({ ok: true, text }));
} catch (e) {
  const first = (e as { errors?: { message?: string }[] }).errors?.[0];
  console.log(JSON.stringify({ ok: false, message: first?.message ?? String(e) }));
}
