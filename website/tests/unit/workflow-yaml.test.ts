// Structural assertions on .github/workflows/website-demo-publish.yml for
// plan.md Story 5.1.1 / validation.md rows 54-55. No YAML-parsing dependency
// is wired into website/ yet, so this inspects the raw text with the same
// "near-mechanical diff against demo-publish.yml" approach research/pitfalls.md
// §4 calls for -- sufficient for a flat, 2-space-indented GitHub Actions file,
// not a general-purpose YAML parser.
import { test } from 'vitest';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../../..');

const WEBSITE_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/website-demo-publish.yml');
const DEMO_PUBLISH_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/demo-publish.yml');
const UTF8 = 'utf8';

/** Raw body lines of a top-level (column-0) `key:` block, up to the next column-0 line. */
function extractTopLevelBlock(yamlText: string, key: string): string[] {
  const lines = yamlText.split('\n');
  const startIndex = lines.findIndex((line) => line.startsWith(`${key}:`));
  if (startIndex === -1) {
    throw new Error(`No top-level "${key}:" block found`);
  }
  const body: string[] = [];
  for (let i = startIndex + 1; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === '' || line.trim().startsWith('#')) continue;
    if (!/^\s/.test(line)) break; // next column-0 key ends the block
    body.push(line);
  }
  return body;
}

function extractOnTriggerKeys(yamlText: string): string[] {
  const body = extractTopLevelBlock(yamlText, 'on');
  return body
    .filter((line) => /^\s{2}\S/.test(line)) // first indent level only
    .map((line) => line.trim().replace(/:.*$/, ''));
}

function extractFeatureSpecs(yamlText: string): string[] {
  const envBody = extractTopLevelBlock(yamlText, 'env');
  const specsLineIndex = envBody.findIndex((line) => line.trim().startsWith('FEATURE_SPECS:'));
  if (specsLineIndex === -1) {
    throw new Error('No FEATURE_SPECS entry found in env: block');
  }
  const specLines: string[] = [];
  for (let i = specsLineIndex + 1; i < envBody.length; i++) {
    const line = envBody[i];
    if (!/^\s{4}\S/.test(line)) break; // FEATURE_SPECS's own nested indent level
    specLines.push(line.trim());
  }
  return specLines.filter(Boolean);
}

function specsMatch(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  const sortedA = [...a].sort();
  const sortedB = [...b].sort();
  return sortedA.every((spec, i) => spec === sortedB[i]);
}

test('websiteDemoWorkflow_should_HaveWorkflowDispatchOnlyTrigger_When_YamlParsed', () => {
  const yamlText = readFileSync(WEBSITE_WORKFLOW_PATH, UTF8);
  const triggers = extractOnTriggerKeys(yamlText);

  assert.deepEqual(triggers, ['workflow_dispatch']);
  for (const forbidden of ['push', 'pull_request', 'schedule', 'cron']) {
    assert.ok(!triggers.includes(forbidden), `on: block must not include "${forbidden}"`);
  }
});

test('websiteDemoWorkflow_should_RejectUnvettedSpec_When_FeatureSpecsListDiffersFromDemoPublishYml', () => {
  const websiteSpecs = extractFeatureSpecs(readFileSync(WEBSITE_WORKFLOW_PATH, UTF8));
  const demoPublishSpecs = extractFeatureSpecs(readFileSync(DEMO_PUBLISH_WORKFLOW_PATH, UTF8));

  // Happy-path baseline: today's two files carry the exact same five specs.
  assert.ok(
    specsMatch(websiteSpecs, demoPublishSpecs),
    'website-demo-publish.yml FEATURE_SPECS must exactly match demo-publish.yml',
  );

  // Error path: a 6th, unvetted spec must be caught by the same diff.
  const tampered = [...websiteSpecs, 'unvetted-new.spec.ts'];
  assert.ok(
    !specsMatch(tampered, demoPublishSpecs),
    'a 6th, unvetted spec added to FEATURE_SPECS must be detected as a mismatch',
  );
});
