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

// Clips are recorded inside the deploy and ship in the Pages artifact, so the
// build must fetch them (tolerating a failed recording) and regenerate
// features.json before building; nothing may be committed back by CI.
test('deployWorkflow_should_RecordAndShipClipsWithoutCommitting_When_YamlParsed', () => {
  const text = readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8);
  const doc = yaml.load(text) as CiWorkflow & { jobs: Record<string, { needs?: unknown; steps: WorkflowStep[] }> };

  assert.ok(doc.jobs['record-clips'], 'deploy must have a record-clips job');
  assert.ok(doc.jobs['record-clips'].steps.some((step) => step.run?.includes('scripts/record-website-demos.sh')));
  assert.equal(doc.jobs.build.needs, 'record-clips');

  const steps = doc.jobs.build.steps;
  const download = steps.findIndex((step) => step.name === 'Download demo clips');
  const regenerate = steps.findIndex((step) => step.name === 'Regenerate features.json with the clips');
  const build = steps.findIndex((step) => step.name === 'Build site');
  assert.ok(download !== -1 && download < regenerate && regenerate < build, 'download -> regenerate -> build order');
  assert.equal((steps[download] as { 'continue-on-error'?: boolean })['continue-on-error'], true, 'a failed recording must not block the deploy');

  assert.ok(!/git (commit|push)/.test(text), 'deploy must not commit anything back');
  assert.ok(!/gh release (upload|create)/.test(text), 'clips ship in the Pages artifact, not a release');
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

// Story 1.1.2: deploy-website.yml must never trigger on pull_request -- the
// repo's `github-pages` environment branch policy rejects anything but main,
// so a stray PR trigger would only fail at deploy time. push-to-main is the
// content trigger; schedule/workflow_dispatch redeploy without a content
// change (website-ci.yml gates PRs instead). Uses js-yaml (not the raw-text
// extractOnTriggerKeys() helper) since it parses this file's unquoted `on:`
// key as the string "on", not a YAML-1.1 boolean.
test('deployWorkflow_should_NeverTriggerOnPullRequest_When_YamlParsed', () => {
  const doc = yaml.load(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)) as {
    on: Record<string, unknown>;
  };

  assert.deepEqual(Object.keys(doc.on).sort(), ['push', 'schedule', 'workflow_dispatch']);
  assert.deepEqual((doc.on.push as { branches: string[] }).branches, ['main']);
});

type CiWorkflow = {
  on: Record<string, unknown>;
  jobs: Record<string, { steps: (WorkflowStep & { with?: unknown; 'working-directory'?: string })[] }>;
};

const WEBSITE_CI_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/website-ci.yml');

// The PR gate must run every check the deploy runs, byte-for-byte, or a PR can
// go green and still break the Pages deploy after merge (which is how the site
// went 25 deploys without publishing). Only the Pages publishing steps are deploy-only.
test('websiteCiWorkflow_should_RunTheSameChecksAsDeploy_When_YamlParsed', () => {
  const deploy = yaml.load(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)) as CiWorkflow;
  const ci = yaml.load(readFileSync(WEBSITE_CI_WORKFLOW_PATH, UTF8)) as CiWorkflow;

  assert.deepEqual(Object.keys(ci.on), ['pull_request'], 'website-ci.yml must trigger only on pull_request');

  // Every deploy step before the Pages publishing steps must also run in the PR
  // gate, except the ones that consume the deploy's own recorded clips.
  const DEPLOY_ONLY = ['Download demo clips', 'Regenerate features.json with the clips'];
  const publishAt = deploy.jobs.build.steps.findIndex((step) => step.name === 'Configure Pages');
  assert.notEqual(publishAt, -1, 'deploy-website.yml must have a "Configure Pages" step');
  const deployChecks = deploy.jobs.build.steps
    .slice(0, publishAt)
    .filter((step) => !DEPLOY_ONLY.includes(step.name ?? ''));
  assert.ok(deployChecks.some((step) => step.name === 'Stop preview server'));
  assert.deepEqual(ci.jobs.checks.steps, deployChecks);
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
