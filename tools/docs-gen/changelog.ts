/**
 * Prints features introduced in a specific version.
 *
 * Usage:
 *   cd tools/docs-gen && npx ts-node --project tsconfig.json changelog.ts 1.0.0
 *
 * Or via Makefile:
 *   make changelog-since VERSION=1.0.0
 */
import * as path from 'path';

interface CatalogFeature {
  id: string;
  title: string;
  description: string;
  since: string;
  status: string;
}

const catalogPath = path.resolve(__dirname, '../../web-app/src/lib/features/index');

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { FEATURE_CATALOG } = require(catalogPath) as {
  FEATURE_CATALOG: Record<string, CatalogFeature>;
};

const version = process.argv[2];
if (!version) {
  console.error('Usage: ts-node changelog.ts <version>');
  console.error('Example: ts-node changelog.ts 1.0.0');
  process.exit(1);
}

const features = Object.values(FEATURE_CATALOG).filter((f) => f.since === version);

if (features.length === 0) {
  console.log(`No features introduced in v${version}.`);
} else {
  console.log(`## New in v${version}\n`);
  for (const f of features) {
    console.log(`### ${f.title} (\`${f.id}\`)\n${f.description}\n`);
  }
}
