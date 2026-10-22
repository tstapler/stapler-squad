import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const M = require("node:module");
console.log(process.versions.bun ? "bun" : "node", "typeof _load:", typeof M._load, "_nodeModulePaths:", typeof M._nodeModulePaths, "_cache:", typeof M._cache);
try { const p = new M("/x/y.js"); const r = M._load("path", p); console.log("  _load('path') ->", r === null ? "null" : typeof r); } catch (e) { console.log("  _load threw:", e.message.slice(0, 80)); }
try { const p = new M("/x/y.js"); p.paths = M._nodeModulePaths("/x"); const r = M._load("@vanilla-extract/css/adapter", p); console.log("  _load(adapter) ->", r === null || r === undefined ? String(r) : typeof r); } catch (e) { console.log("  _load(adapter) threw:", e.message.slice(0, 80)); }
