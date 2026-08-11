/**
 * Prints a diff of catalog features vs committed docs/api/features/*.md files.
 * Useful for CI PR comments to show what documentation would change.
 *
 * Usage:
 *   cd tools/docs-gen && npx ts-node --project tsconfig.json diff.ts
 *
 * Or via Makefile:
 *   make docs-features-diff
 */
import * as path from 'path';
import * as fs from 'fs';

interface CatalogFeature {
  id: string;
  title: string;
  description: string;
  rpcIds: readonly string[];
  componentPaths: readonly string[];
  testIds: readonly string[];
  status: string;
  since: string;
}

const catalogPath = path.resolve(__dirname, '../../web-app/src/lib/features/index');

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { FEATURE_CATALOG } = require(catalogPath) as {
  FEATURE_CATALOG: Record<string, CatalogFeature>;
};

const DOCS_DIR = path.resolve(__dirname, '../../docs/api/features');

const catalogIds = new Set(Object.keys(FEATURE_CATALOG));

// Find docs that no longer have a catalog entry
const existingDocs = fs.existsSync(DOCS_DIR)
  ? fs.readdirSync(DOCS_DIR).filter((f) => f.endsWith('.md'))
  : [];

const docIds = new Set(existingDocs.map((f) => f.replace(/\.md$/, '')));

const added = [...catalogIds].filter((id) => !docIds.has(id));
const removed = [...docIds].filter((id) => !catalogIds.has(id));
const present = [...catalogIds].filter((id) => docIds.has(id));

if (added.length === 0 && removed.length === 0 && present.length === catalogIds.size) {
  console.log('No documentation diff — catalog and docs are in sync.');
  process.exit(0);
}

if (added.length > 0) {
  console.log('## New features (docs will be generated):\n');
  for (const id of added) {
    const f = FEATURE_CATALOG[id];
    console.log(`+ ${f.title} (\`${id}\`)`);
  }
  console.log('');
}

if (removed.length > 0) {
  console.log('## Removed features (stale docs):\n');
  for (const id of removed) {
    console.log(`- ${id}`);
  }
  console.log('');
}

console.log(`Catalog: ${catalogIds.size} features | Docs: ${docIds.size} files`);
