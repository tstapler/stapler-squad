#!/usr/bin/env npx ts-node
import * as fs from 'fs';
import * as path from 'path';

interface BackendFeature {
  id: string;
  type: string;
  tested: boolean;
  testIds: string[];
}

interface Registry {
  version: string;
  features: BackendFeature[];
}

function findSpecFiles(dir: string): string[] {
  const results: string[] = [];
  if (!fs.existsSync(dir)) return results;
  function walk(d: string) {
    for (const entry of fs.readdirSync(d, { withFileTypes: true })) {
      const full = path.join(d, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.name.endsWith('.spec.ts')) results.push(full);
    }
  }
  walk(dir);
  return results;
}

function extractFeatureIds(testFile: string): string[] {
  const content = fs.readFileSync(testFile, 'utf-8');
  const matches = content.match(/@feature\s+([\w:,-]+)/g) || [];
  return matches.flatMap(m => m.replace('@feature', '').trim().split(/[,\s]+/).filter(Boolean));
}

function generateCoverageReport(registryPath: string, testDir: string): void {
  const registry: Registry = JSON.parse(fs.readFileSync(registryPath, 'utf-8'));
  const testFiles = findSpecFiles(testDir);

  const coveredIds = new Set<string>();
  for (const file of testFiles) {
    for (const id of extractFeatureIds(file)) {
      coveredIds.add(id);
    }
  }

  const totalFeatures = registry.features.length;
  const coveredCount = registry.features.filter(f => coveredIds.has(f.id)).length;
  const coveragePercent = totalFeatures > 0 ? Math.round((coveredCount / totalFeatures) * 100) : 0;

  const report = {
    totalFeatures,
    coveredCount,
    coveragePercent,
    coveredIds: Array.from(coveredIds),
    uncoveredIds: registry.features.filter(f => !coveredIds.has(f.id)).map(f => f.id),
  };

  console.log(`## Feature E2E Coverage Report`);
  console.log(`Feature E2E coverage: ${coveredCount}/${totalFeatures} tested (${coveragePercent}%)`);
  console.log(JSON.stringify(report, null, 2));

  const reportDir = path.dirname(registryPath);
  fs.writeFileSync(path.join(reportDir, 'coverage-report.json'), JSON.stringify(report, null, 2));
}

const registryPath = process.argv[2] || 'docs/registry/backend-features.json';
// Accept either a glob pattern like '../../tests/e2e/**/*.spec.ts' or a directory
const testArg = process.argv[3] || 'tests/e2e';
// Derive the spec directory from a glob pattern (strip /**/*.spec.ts suffix)
const testDir = testArg.replace(/\/\*\*\/\*\.spec\.ts$/, '');

if (fs.existsSync(registryPath)) {
  generateCoverageReport(registryPath, testDir);
} else {
  console.log('Registry not found, skipping coverage report');
  process.exit(0);
}
