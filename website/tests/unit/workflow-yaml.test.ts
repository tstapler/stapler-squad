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
import yaml from 'js-yaml';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../../..');

const WEBSITE_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/website-demo-publish.yml');
const DEMO_PUBLISH_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/demo-publish.yml');
const DEPLOY_WEBSITE_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/deploy-website.yml');
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

// Phase 6, Story 6.3.1: deploy-website.yml's build job must run its
// blocking checks in the exact dependency order plan.md specifies --
// sync-content verify (before anything reads the CuratedDocs it checks) ->
// build (before any check needs dist/) -> check-internal-links ->
// check-asset-health -> Lighthouse CI -> upload the artifact only once every
// earlier gate has passed. Uses js-yaml (unlike the raw-text block
// extraction above) because this job has many more steps in a deeper,
// list-of-maps structure than the simple top-level on:/env: blocks those
// older tests were written against -- real parsing is less brittle here
// than another ad hoc indentation-counting walk.
type WorkflowStep = { name?: string; uses?: string; run?: string };

/** Ordered list of the build job's step labels (name, falling back to uses/run). */
function extractBuildStepLabels(yamlText: string): string[] {
  const doc = yaml.load(yamlText) as { jobs: { build: { steps: WorkflowStep[] } } };
  return doc.jobs.build.steps.map((step) => step.name ?? step.uses ?? step.run ?? '(unnamed step)');
}

/**
 * Asserts every label in `requiredInOrder` appears in `stepLabels`, in that
 * relative order (other steps may appear between/around them). Returns the
 * missing label's text on failure, or null if the whole sequence is present
 * and ordered correctly.
 */
function findOrderViolation(stepLabels: string[], requiredInOrder: string[]): string | null {
  let searchFrom = 0;
  for (const required of requiredInOrder) {
    const foundAt = stepLabels.findIndex(
      (label, i) => i >= searchFrom && label.includes(required),
    );
    if (foundAt === -1) {
      return `missing step (or out of order): "${required}"`;
    }
    searchFrom = foundAt + 1;
  }
  return null;
}

const REQUIRED_BUILD_STEP_ORDER = [
  'Verify content sync',
  'Build site',
  'Check internal links',
  'Check demo-asset health',
  'Lighthouse CI (desktop)',
  'Lighthouse CI (mobile)',
  'Upload Pages artifact',
];

test('deployWorkflow_should_RunChecksInDependencyOrder_When_YamlParsed', () => {
  const stepLabels = extractBuildStepLabels(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8));

  assert.equal(
    findOrderViolation(stepLabels, REQUIRED_BUILD_STEP_ORDER),
    null,
    `deploy-website.yml step order was: ${JSON.stringify(stepLabels)}`,
  );
});

test('deployWorkflow_should_FlagMissingStep_When_AssetHealthCheckStepRemoved', () => {
  const stepLabels = extractBuildStepLabels(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8));
  const tampered = stepLabels.filter((label) => !label.includes('Check demo-asset health'));

  const violation = findOrderViolation(tampered, REQUIRED_BUILD_STEP_ORDER);
  assert.ok(
    violation && violation.includes('Check demo-asset health'),
    'removing the asset-health-check step must be caught by the same order check',
  );
});

// Story 1.1.2: deploy-website.yml must trigger only on push to main --
// repo's existing `github-pages` environment branch policy rejects anything
// else, so a stray pull_request trigger would just fail at deploy time
// instead of being caught here. Uses js-yaml like the two tests above
// (not the raw-text extractOnTriggerKeys() helper website-demo-publish.yml's
// tests use), since js-yaml parses this file's unquoted `on:` key as the
// string "on", not a YAML-1.1 boolean -- confirmed directly against this file.
test('deployWorkflow_should_TriggerOnlyOnPushToMain_When_YamlParsed', () => {
  const doc = yaml.load(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)) as {
    on: Record<string, unknown>;
  };

  assert.deepEqual(Object.keys(doc.on), ['push'], 'on: block must declare only a push trigger');
  assert.deepEqual((doc.on.push as { branches: string[] }).branches, ['main']);
});

// Story 1.1.2: the deploy job needs id-token: write to mint the OIDC token
// actions/deploy-pages requires. Named for the failure mode this test is
// designed to catch (a YAML missing that permission) -- it's written against
// the real file's current state, which already has it, so this passes today
// and would fail the moment the permission is removed.
test('deployWorkflow_should_RequestIdTokenWritePermission_When_YamlMissingPermissionsBlock', () => {
  const doc = yaml.load(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)) as {
    permissions: Record<string, string>;
  };

  assert.equal(doc.permissions['id-token'], 'write');
});
