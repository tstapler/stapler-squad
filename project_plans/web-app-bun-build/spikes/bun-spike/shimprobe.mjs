import { createRequire } from "node:module";
import vm from "node:vm";
const f = process.cwd() + "/src/x.css.ts";
const req = createRequire(f);
console.log("createRequire adapter:", typeof req("@vanilla-extract/css/adapter")?.setAdapter);
const sandbox = { require: req, out: null };
try { new vm.Script("out = typeof require('@vanilla-extract/css/adapter').setAdapter", {filename: f}).runInNewContext(sandbox); console.log("in vm context:", sandbox.out); } catch (e) { console.log("vm threw:", e.message.slice(0,100)); }
import { createRequire as c2 } from "node:module";
console.log("which eval resolves:", import.meta.resolveSync ? import.meta.resolveSync("eval") : "n/a");
