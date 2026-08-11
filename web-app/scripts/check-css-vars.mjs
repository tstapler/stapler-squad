#!/usr/bin/env node
/**
 * Validates that all var(--xxx) references in CSS Modules files are defined in globals.css.
 * stylelint's no-unknown-custom-properties rule operates per-file only and produces false
 * positives for CSS Modules consuming tokens from a separate globals.css; this script does
 * the cross-file check that stylelint cannot.
 *
 * Run via: npm run lint:css-vars
 * See: docs/adr/009-vanilla-extract-type-safe-css.md
 */

import { readFileSync, readdirSync, statSync } from "fs";
import { join, relative, extname } from "path";
import { fileURLToPath } from "url";

const __dirname = fileURLToPath(new URL(".", import.meta.url));
const WEB_APP_ROOT = join(__dirname, "..");
const GLOBALS_CSS = join(WEB_APP_ROOT, "src/app/globals.css");
const SRC_DIR = join(WEB_APP_ROOT, "src");

// ── Collect all custom properties defined in globals.css ──────────────────────

function extractDefinedProps(css) {
  const defined = new Set();
  for (const m of css.matchAll(/\B(--[\w-]+)\s*:/g)) {
    defined.add(m[1]);
  }
  return defined;
}

const globalsContent = readFileSync(GLOBALS_CSS, "utf8");
const defined = extractDefinedProps(globalsContent);

// ── Walk src/ for .module.css files ───────────────────────────────────────────

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      yield* walk(full);
    } else {
      yield full;
    }
  }
}

// ── Check each .module.css file for undefined var() references ────────────────

let errors = 0;

for (const file of walk(SRC_DIR)) {
  if (extname(file) !== ".css") continue;
  if (!file.endsWith(".module.css")) continue;

  const content = readFileSync(file, "utf8");
  const refs = [...content.matchAll(/var\((--[\w-]+)/g)];

  for (const [, prop] of refs) {
    if (!defined.has(prop)) {
      const rel = relative(WEB_APP_ROOT, file);
      console.error(`\x1b[31merror\x1b[0m  ${rel}: undefined CSS variable \`${prop}\` — add it to src/app/globals.css first`);
      errors++;
    }
  }
}

if (errors > 0) {
  console.error(`\n${errors} undefined CSS variable reference${errors === 1 ? "" : "s"} found.`);
  process.exit(1);
} else {
  console.log(`\x1b[32m✓\x1b[0m All CSS variable references are defined in globals.css`);
}
