// Functional canary: a real .css.ts compiled and processed the way VePlugin will (Story 5.1).
import { compile, processVanillaFile } from "@vanilla-extract/integration";
import path from "node:path";

const filePath = path.resolve(__dirname, "../fixtures/canary/canary.css.ts");

try {
  const identOption = "debug";
  const compiled = await compile({ filePath, cwd: process.cwd(), identOption });
  const source = await processVanillaFile({ source: compiled.source, filePath, identOption });
  const encoded = /source=([A-Za-z0-9+/=]+)/.exec(source)?.[1] ?? "";
  console.log(JSON.stringify({ ok: true, source, css: Buffer.from(encoded, "base64").toString() }));
} catch (e) {
  console.log(JSON.stringify({ ok: false, error: String(e) }));
  process.exit(1);
}
