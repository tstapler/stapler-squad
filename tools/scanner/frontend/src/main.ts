#!/usr/bin/env ts-node
import * as fs from 'fs';
import * as path from 'path';
import { scanComponents, FrontendFeature } from './component-scanner';
import { generateGapReport } from './gap-reporter';

interface FrontendRegistry {
  version: string;
  generatedAt: string;
  features: FrontendFeature[];
}

function main(): void {
  const args = process.argv.slice(2);
  const srcDir = args[0] || 'web-app/src';
  const outputFile = args[1] || 'docs/registry/frontend-features.json';
  const backendRegistryFile = args[2] || 'docs/registry/backend-features.json';
  const gapReportFile = args[3] || 'docs/registry/coverage-gaps.json';

  const absoluteSrcDir = path.resolve(process.cwd(), srcDir);

  if (!fs.existsSync(absoluteSrcDir)) {
    console.error(`Source directory not found: ${absoluteSrcDir}`);
    process.exit(1);
  }

  // Use cwd as the root for relative paths so registry entries are relative to the project root.
  const projectRoot = process.cwd();
  console.log(`Scanning frontend features in: ${absoluteSrcDir}`);
  const features = scanComponents(absoluteSrcDir, projectRoot);
  console.log(`Found ${features.length} feature-marked components`);

  const registry: FrontendRegistry = {
    version: '1',
    generatedAt: new Date().toISOString(),
    features,
  };

  // Write frontend registry
  const outputDir = path.dirname(path.resolve(process.cwd(), outputFile));
  fs.mkdirSync(outputDir, { recursive: true });
  fs.writeFileSync(path.resolve(process.cwd(), outputFile), JSON.stringify(registry, null, 2));
  console.log(`Frontend registry written to: ${outputFile}`);

  // Generate coverage gap report if backend registry exists
  const absBackendRegistry = path.resolve(process.cwd(), backendRegistryFile);
  if (fs.existsSync(absBackendRegistry)) {
    try {
      const absOutputFile = path.resolve(process.cwd(), outputFile);
      const gaps = generateGapReport(absBackendRegistry, absOutputFile);
      fs.writeFileSync(
        path.resolve(process.cwd(), gapReportFile),
        JSON.stringify(gaps, null, 2),
      );
      console.log(
        `Coverage gap report written to: ${gapReportFile} (${gaps.unmatchedBackend.length} unmatched backend, ${gaps.unmatchedFrontend.length} unmatched frontend)`,
      );
    } catch (err) {
      console.warn(`Warning: Could not generate gap report: ${err}`);
    }
  } else {
    console.log(`Backend registry not found at ${backendRegistryFile}, skipping gap report`);
  }
}

main();
