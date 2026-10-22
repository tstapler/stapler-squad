// Probe: is the /backlog tour modal's pointer-events difference (Next none, Bun auto) deterministic or a timing artefact?
// Run from web-app/:  node ../project_plans/web-app-bun-build/spikes/s3/probe-tour.mjs <nextBase> <bunBase>
import { launch } from "./pw.mjs";

const [nextBase, bunBase] = process.argv.slice(2);
const browser = await launch();
const out = {};
for (const [name, base] of [["next", nextBase], ["bun", bunBase]]) {
  out[name] = [];
  for (let i = 0; i < 4; i++) {
    const page = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
    await page.goto(base + "/backlog/", { waitUntil: "load" });
    await page.waitForSelector("[data-testid=backlog-tour-modal]", { timeout: 10000 }).catch(() => {});
    const samples = [];
    for (const wait of [0, 300, 1500, 4000]) {
      await page.waitForTimeout(wait === 0 ? 0 : wait - (samples.length ? [0, 0, 300, 1500][samples.length] : 0));
      samples.push(await page.evaluate(() => {
        const el = document.querySelector("[data-testid=backlog-tour-modal]");
        return el ? { inline: el.getAttribute("style"), computed: getComputedStyle(el).pointerEvents } : null;
      }));
    }
    out[name].push(samples.map((s) => (s ? `${s.computed} (${s.inline})` : "absent")));
    await page.context().close();
  }
}
await browser.close();
console.log(JSON.stringify(out, null, 1));
