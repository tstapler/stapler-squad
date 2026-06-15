/**
 * Generates per-feature Markdown documentation from the typed feature catalog.
 *
 * Usage:
 *   cd tools/docs-gen && npm install && npx ts-node --project tsconfig.json generate.ts
 *
 * Or via Makefile:
 *   make docs-features
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

// Load catalog using ts-node (CommonJS mode) with a relative path.
// No @/ alias here — docs-gen has its own tsconfig.
const catalogPath = path.resolve(__dirname, '../../web-app/src/lib/features/index');

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { FEATURE_CATALOG } = require(catalogPath) as {
  FEATURE_CATALOG: Record<string, CatalogFeature>;
};

const OUT_DIR = path.resolve(__dirname, '../../docs/api/features');
fs.mkdirSync(OUT_DIR, { recursive: true });

for (const [id, feature] of Object.entries(FEATURE_CATALOG)) {
  const rpcSection =
    feature.rpcIds.length > 0
      ? feature.rpcIds.map((r) => `- \`${r}\``).join('\n')
      : '_none_';

  const componentSection =
    feature.componentPaths.length > 0
      ? feature.componentPaths.map((c) => `- \`${c}\``).join('\n')
      : '_none_';

  const testSection =
    feature.testIds.length > 0
      ? feature.testIds.map((t) => `- ${t}`).join('\n')
      : '_none_';

  const md = [
    `# ${feature.title}`,
    '',
    `**ID**: \`${feature.id}\`  `,
    `**Status**: ${feature.status}  `,
    `**Since**: v${feature.since}`,
    '',
    feature.description,
    '',
    '## RPCs',
    '',
    rpcSection,
    '',
    '## Components',
    '',
    componentSection,
    '',
    '## Tests',
    '',
    testSection,
  ].join('\n');

  fs.writeFileSync(path.join(OUT_DIR, `${id}.md`), md + '\n');
  console.log(`  ✅ docs/api/features/${id}.md`);
}

console.log(`\nGenerated ${Object.keys(FEATURE_CATALOG).length} feature docs.`);
