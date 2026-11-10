// Structural assertions on .github/workflows/deploy-website.yml and
// website-ci.yml: the PR gate must run the same checks as the deploy, and the
// deploy must record clips without committing anything. Parsed with js-yaml.
import { test } from 'vitest';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
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

type Step = WorkflowStep & {
  id?: string;
  if?: string;
  env?: Record<string, string>;
  with?: Record<string, unknown>;
  'continue-on-error'?: boolean | string;
};
type Job = {
  needs?: string | string[];
  if?: string;
  steps: Step[];
  permissions?: Record<string, string>;
  'timeout-minutes'?: number;
  strategy?: { matrix?: unknown };
};
type Workflow = { on: Record<string, unknown>; jobs: Record<string, Job> };

const loadDeploy = (): Workflow => yaml.load(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)) as Workflow;
const needsOf = (job: Job): string[] => (Array.isArray(job.needs) ? job.needs : job.needs ? [job.needs] : []);
const stepNamed = (job: Job, name: string): Step => {
  const step = job.steps.find((candidate) => candidate.name === name);
  assert.ok(step, `missing step "${name}"`);
  return step;
};

// Media is recorded in a shard matrix and ships in the Pages artifact; nothing
// may be committed back by CI. Shards merge in the build job.
test('deployWorkflow_should_RecordShardsAndShipMediaWithoutCommitting_When_YamlParsed', () => {
  const text = readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8);
  const doc = loadDeploy();

  const record = doc.jobs.record;
  assert.ok(record, 'deploy must have a record job');
  assert.ok(record.steps.some((step) => step.run?.includes('scripts/record-website-demos.sh')));
  assert.ok(record.steps.some((step) => step.run?.includes('--only')), 'each shard records only its tours');
  assert.match(String(record.strategy?.matrix), /fromJSON\(needs\.decide\.outputs\.matrix\)/, 'matrix comes from mediatool shard-plan');
  assert.ok(doc.jobs.decide.steps.some((step) => step.run?.includes('mediatool" shard-plan')), 'decide runs shard-plan');
  assert.ok(needsOf(record).includes('binary'), 'the binary is built once and passed to the shards');
  assert.ok(doc.jobs.build.steps.some((step) => step.uses?.includes('download-artifact') && step.with?.pattern === 'media-shard-*' && step.with?.['merge-multiple'] === true));
  assert.ok(needsOf(doc.jobs.build).includes('record'));

  const checkout = record.steps.find((step) => step.uses?.includes('actions/checkout'));
  assert.ok(checkout && !('lfs' in (checkout.with ?? {})), 'no lfs in the record checkout (it would mask the media guard)');

  const steps = doc.jobs.build.steps;
  const download = steps.findIndex((step) => step.name === 'Download media shards');
  const regenerate = steps.findIndex((step) => step.name === 'Regenerate features.json with the clips');
  const build = steps.findIndex((step) => step.name === 'Build site');
  assert.ok(download !== -1 && download < regenerate && regenerate < build, 'download -> regenerate -> build order');
  assert.equal(steps[download]['continue-on-error'], true, 'a failed shard must not stop the build while freshness is report-only');

  assert.ok(!/git (commit|push)/.test(text), 'deploy must not commit anything back');
  assert.ok(!/gh release (upload|create)/.test(text), 'media ships in the Pages artifact, not a release');
});

// freshnessEnforce lives in .github/promo-flags.json and is read by mediatool;
// the old warn-and-fallback step stays beside it until Task 5.1.2a.
test('deployWorkflow_should_RunFreshnessBesideLegacyFallback_When_YamlParsed', () => {
  const text = readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8);
  const doc = loadDeploy();
  const labels = doc.jobs.build.steps.map((step) => step.name ?? '');

  assert.ok(labels.indexOf('Freshness precheck (mediatool fresh)') !== -1);
  assert.ok(labels.indexOf('Freshness precheck (mediatool fresh)') < labels.indexOf('Warn when no clips were shipped'));
  assert.match(text, /# REMOVE IN 5\.1\.2/, 'legacy fallback is marked for removal by Task 5.1.2a');

  assert.ok(doc.jobs.mediafresh.steps.some((step) => step.run?.includes('mediatool" fresh')), 'mediafresh job holds the dist-level check');
  assert.ok(doc.jobs['termcheck-dist'].steps.some((step) => step.run?.includes('-mode=dist')));
  assert.ok(doc.jobs.record.steps.some((step) => step.run?.includes('-mode=media-text')), 'media-text blocks before upload');
  const upload = doc.jobs.record.steps.findIndex((step) => step.name === 'Upload shard media');
  const domScan = doc.jobs.record.steps.findIndex((step) => step.name === 'Scan DOM text for banned terms');
  assert.ok(domScan !== -1 && domScan < upload, 'DOM-text scan runs before the shard upload');
  assert.equal(doc.jobs.record.steps[domScan]['continue-on-error'], undefined, 'DOM-text hits block');

  // Deploy waits for both post-build gates; flag-driven enforcement makes them fail the job.
  assert.deepEqual(needsOf(doc.jobs.deploy).sort(), ['build', 'mediafresh', 'termcheck-dist']);
  assert.ok(!/PROMO_FRESHNESS_ENFORCE/.test(text), 'flags live in git, not repo variables');
});

// Task 2.4.1h: the OCR step is blocking (no continue-on-error), emits Tesseract TSV for the
// confidence rule, and runs before the shard upload.
test('OCR step blocks the upload and feeds the confidence rule', () => {
  const steps = loadDeploy().jobs.record.steps;
  const ocr = steps.findIndex((step) => step.name === 'OCR the shots and scan for banned terms');
  const upload = steps.findIndex((step) => step.name === 'Upload shard media');
  assert.ok(ocr !== -1 && ocr < upload, 'OCR scan runs before the shard upload');
  assert.equal(steps[ocr]['continue-on-error'], undefined, 'OCR hits block, independent of freshnessEnforce');
  assert.match(steps[ocr].run ?? '', /tesseract .* tsv/, 'TSV output carries word confidence');
  assert.match(steps[ocr].run ?? '', /-mode=media-text -dir "\$RUNNER_TEMP\/media-out\/ocr-text"/);
});

// Validation R16: notify covers every failure source and the flag-age reminder.
test('deployWorkflow_should_NotifyOnEveryFailureSource_When_YamlParsed', () => {
  const doc = loadDeploy();
  const needs = needsOf(doc.jobs.notify);
  for (const job of ['record', 'mediafresh', 'termcheck-dist', 'build', 'deploy']) {
    assert.ok(needs.includes(job), `notify must need ${job}`);
  }
  const flagAge = doc.jobs.notify.steps.find((step) => step.name?.startsWith('Remind about flags still false'));
  assert.ok(flagAge, 'notify has a flag-age step');
  assert.match(flagAge.run ?? '', /promo-flags\.json/);
  assert.match(flagAge.run ?? '', /\bjq\b/, 'flags are read with jq');
  assert.match(flagAge.run ?? '', /-ge 14/);
  assert.match(flagAge.if ?? '', /schedule/);
});

// G4: last-known-good is a named, retained, looked-up artifact, never a silent skip.
test('deployWorkflow_should_KeepLastKnownGoodAsNamedArtifact_When_YamlParsed', () => {
  const text = readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8);
  const doc = loadDeploy();

  const upload = doc.jobs['publish-lkg'].steps.find((step) => step.with?.name === 'promo-media-lkg');
  assert.ok(upload?.uses?.includes('upload-artifact'));
  assert.ok(Number(upload.with?.['retention-days']) >= 35, 'retention covers the 28-day age bound');
  assert.match(doc.jobs['publish-lkg'].if ?? '', /mode == 'record'/, 'a reuse run uploads no new LKG');
  assert.match(doc.jobs['publish-lkg'].if ?? '', /lkg_used != 'true'/);
  assert.match(doc.jobs['publish-lkg'].if ?? '', /clean == 'true'/, 'only a fully fresh run becomes LKG');
  assert.ok(needsOf(doc.jobs['publish-lkg']).includes('deploy'));

  for (const jobName of ['decide', 'build']) {
    assert.equal(doc.jobs[jobName].permissions?.actions, 'read', `${jobName} looks the artifact up with gh`);
  }
  const reuse = stepNamed(doc.jobs.build, 'Download last-known-good media (reuse)');
  const fallback = stepNamed(doc.jobs.build, 'Use last-known-good media (scheduled fallback)');
  for (const step of [reuse, fallback]) {
    assert.equal(step['continue-on-error'], undefined, `${step.name} must hard-fail when the artifact is missing`);
    assert.match(step.run ?? '', /mediatool" lkg/);
  }
  assert.match(fallback.if ?? '', /github\.event_name == 'schedule'/, 'push and dispatch never use last-known-good');
  assert.match(fallback.run ?? '', /-max-age-days 28/);
  assert.ok(!/uses:\s*actions\/cache(\/restore)?@/.test(text), 'LKG does not rely on actions/cache (7-day idle eviction)');
});

// Content-only pushes reuse the LKG media; schedule and dispatch always record.
test('deployWorkflow_should_DecideReuseOnlyForPushes_When_YamlParsed', () => {
  const doc = loadDeploy();
  const decide = stepNamed(doc.jobs.decide, 'Decide record or reuse');
  assert.match(decide.run ?? '', /GITHUB_EVENT_NAME" = push/);
  assert.match(decide.run ?? '', /fresh -decide-reuse/);
  assert.match(doc.jobs.record.if ?? '', /mode == 'record'/);
  assert.match(doc.jobs.binary.if ?? '', /mode == 'record'/);
  assert.match(stepNamed(doc.jobs.mediafresh, 'Freshness and budgets (mediatool fresh)').env?.MODE ?? '', /decide\.outputs\.mode/);
});

// The record ceiling is 30 min (budgets.json); the job timeout adds cold-cache setup only.
test('deployWorkflow_should_BoundJobRuntimes_When_YamlParsed', () => {
  const doc = loadDeploy();
  assert.ok((doc.jobs.record['timeout-minutes'] ?? 0) > 30 && (doc.jobs.record['timeout-minutes'] ?? 0) <= 45);
  for (const [name, job] of Object.entries(doc.jobs)) {
    if (name !== 'deploy') assert.ok(job['timeout-minutes'], `${name} needs a timeout-minutes cap`);
  }
});

// Legacy publishing is retired: no demo-publish workflow and no demo-gifs-latest release reference.
test('workflows_should_NotReferenceRetiredDemoPublish_When_Scanned', () => {
  const dir = path.join(repoRoot, '.github/workflows');
  assert.ok(!existsSync(path.join(dir, 'demo-publish.yml')), 'demo-publish.yml must be deleted');
  for (const file of readdirSync(dir)) {
    const text = readFileSync(path.join(dir, file), UTF8);
    assert.ok(!/(?<!website-)demo-gifs-latest/.test(text), `${file} references the retired demo-gifs-latest release`);
  }
});

// Phase 6, Story 6.3.1: deploy-website.yml's build job must run its
// blocking checks in the exact dependency order plan.md specifies --
// sync-content verify (before anything reads the CuratedDocs it checks) ->
// build (before any check needs dist/) -> check-internal-links ->
// check-readme-links -> Lighthouse CI -> upload the artifact only once every
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
  'Check README links',
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

test('deployWorkflow_should_FlagMissingStep_When_ReadmeLinkCheckStepRemoved', () => {
  const stepLabels = extractBuildStepLabels(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8));
  const tampered = stepLabels.filter((label) => !label.includes('Check README links'));

  const violation = findOrderViolation(tampered, REQUIRED_BUILD_STEP_ORDER);
  assert.ok(
    violation && violation.includes('Check README links'),
    'removing the README link check step must be caught by the same order check',
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

const WEBSITE_CI_WORKFLOW_PATH = path.join(repoRoot, '.github/workflows/website-ci.yml');

// The PR gate must run every check the deploy runs, byte-for-byte, or a PR can
// go green and still break the Pages deploy after merge (which is how the site
// went 25 deploys without publishing). Intentional differences are named below.
const DEPLOY_ONLY = [
  'Download demo clips',
  'Download promo plan',
  'Make promo tools executable',
  'Download media shards',
  'Stage shard media and merge manifests',
  'Download last-known-good media (reuse)',
  'Freshness precheck (mediatool fresh)',
  'Use last-known-good media (scheduled fallback)',
  'Warn when no clips were shipped',
  'Regenerate features.json with the clips',
];
const CI_ONLY_PREFIX = 'Smoke: ';

/** Drops TOURS_MEDIA_MODE from a step's env so the one intentional env difference does not fail equality. */
function withoutMediaMode(step: Step): Step {
  if (!step.env || !('TOURS_MEDIA_MODE' in step.env)) return step;
  const { TOURS_MEDIA_MODE: _mode, ...rest } = step.env;
  const copy: Step = { ...step };
  if (Object.keys(rest).length > 0) copy.env = rest;
  else delete copy.env;
  return copy;
}

test('websiteCiWorkflow_should_RunTheSameChecksAsDeploy_When_YamlParsed', () => {
  const deploy = loadDeploy();
  const ci = yaml.load(readFileSync(WEBSITE_CI_WORKFLOW_PATH, UTF8)) as Workflow;

  assert.deepEqual(Object.keys(ci.on), ['pull_request'], 'website-ci.yml must trigger only on pull_request');

  const publishAt = deploy.jobs.build.steps.findIndex((step) => step.name === 'Configure Pages');
  assert.notEqual(publishAt, -1, 'deploy-website.yml must have a "Configure Pages" step');
  const deployChecks = deploy.jobs.build.steps
    .slice(0, publishAt)
    .filter((step) => !DEPLOY_ONLY.includes(step.name ?? ''));
  assert.ok(deployChecks.some((step) => step.name === 'Stop preview server'));
  const ciChecks = ci.jobs.checks.steps.filter((step) => !step.name?.startsWith(CI_ONLY_PREFIX));
  assert.deepEqual(ciChecks.map(withoutMediaMode), deployChecks);
  assert.ok(ci.jobs.checks.steps.some((step) => step.name?.startsWith(CI_ONLY_PREFIX)), 'smoke steps exist');
});

// Cross-cutting 6c: TOURS_MEDIA_MODE=smoke is set only by website-ci, on the site build.
test('websiteCiWorkflow_should_SmokeRecordOneClipAgainstItsOwnBinary_When_YamlParsed', () => {
  const ciText = readFileSync(WEBSITE_CI_WORKFLOW_PATH, UTF8);
  const ci = yaml.load(ciText) as Workflow & { on: { pull_request: { paths: string[] } } };

  const withMode = ci.jobs.checks.steps.filter((step) => step.env?.TOURS_MEDIA_MODE !== undefined);
  assert.deepEqual(withMode.map((step) => step.name), ['Build site', 'Check README links']);
  assert.ok(withMode.every((step) => step.env?.TOURS_MEDIA_MODE === 'smoke'));
  assert.ok(!/TOURS_MEDIA_MODE/.test(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)), 'deploy builds in the default deploy mode');

  const record = ci.jobs.checks.steps.find((step) => step.name === 'Smoke: record the dashboard-lifecycle clip');
  assert.ok(record, 'smoke record step exists');
  assert.match(record.run ?? '', /--only dashboard-lifecycle --smoke/);
  assert.ok(ci.jobs.checks.steps.some((step) => step.run?.includes('go build -o stapler-squad')), 'binary is built from the PR');
  assert.ok(!ci.jobs.checks.steps.some((step) => step.uses?.includes('download-artifact')), 'no 1-day-retention binary download');

  for (const trigger of ['website/**', 'tests/e2e/website-demos/**', 'tests/demo/**']) {
    assert.ok(ci.on.pull_request.paths.includes(trigger), `smoke trigger path ${trigger}`);
  }
  assert.equal(Object.keys(ci.jobs).length, 1, 'website-ci stays one job (check budget)');
  assert.ok((ci.jobs.checks['timeout-minutes'] ?? 0) <= 30);
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

test('deployWorkflow_should_RetireTheReleaseAssetHealthCheck_When_YamlParsed', () => {
  for (const file of ['deploy-website.yml', 'website-ci.yml']) {
    const text = readFileSync(path.join(repoRoot, '.github/workflows', file), UTF8);
    assert.ok(!/check-asset-health|demo-gifs-latest/.test(text), `${file} no longer runs the release-URL health check`);
  }
});

// Every id the recorder can run must reach a shard: tours and shotlists by glob in shard-plan,
// legacy clips (clips.ts, not files) through -extra.
test('deployWorkflow_should_PlanEveryRecordableId_When_ShardPlanRuns', () => {
  const plan = yaml.load(readFileSync(DEPLOY_WEBSITE_WORKFLOW_PATH, UTF8)) as Workflow;
  const run = plan.jobs.decide.steps.find((step) => step.name === 'Plan shards')?.run ?? '';
  const extras = [...run.matchAll(/-extra\s+(\S+)/g)].map((m) => m[1]).sort();
  const clips = readFileSync(path.join(repoRoot, 'tests/e2e/website-demos/clips.ts'), UTF8);
  const legacy = [...clips.matchAll(/^\s{4}name: '([^']+)'/gm)].map((m) => m[1]).sort();
  assert.ok(legacy.length > 0, 'clips.ts names were parsed');
  assert.deepEqual(extras, legacy, 'legacy clips in clips.ts must be passed to shard-plan with -extra');
  assert.ok(!/-shotlists-dir/.test(run), 'shotlists/ is planned from its default directory');
  assert.ok(readdirSync(path.join(repoRoot, 'tests/e2e/website-demos/shotlists')).length > 0);
});

// docscoverage must never fall back to its merge-base default in CI: each event names its base
// (Task 4.1.4a). HEAD^1 is the pull_request merge commit's base, github.event.before is the push base.
test('buildWorkflow_should_PassExplicitBasePerEvent_When_DocsCoverageStepParsed', () => {
  const build = yaml.load(readFileSync(path.join(repoRoot, '.github/workflows/build.yml'), UTF8)) as Workflow;
  const steps = Object.values(build.jobs).flatMap((job) => job.steps ?? []);
  const step = steps.find((candidate) => candidate.name?.startsWith('Docs coverage'));
  assert.ok(step, 'build.yml has a Docs coverage step');
  const run = String((step as Step & { run?: string }).run);

  assert.match(run, /pull_request\)\s*BASE=HEAD\^1/);
  assert.match(run, /push\)\s*BASE="\$EVENT_BEFORE"/);
  assert.match(run, /\*\)\s*BASE=none/);
  assert.match(run, /docscoverage -base "\$BASE"/);
  assert.match(run, /docscoverage enforce -base "\$BASE"/, 'the enforce flag is read from the base blob, not the PR tree');
  assert.ok(!/promo-flags\.json/.test(run), 'the step does not read the flags file from the working tree');
  assert.equal(step.env?.EVENT_BEFORE, '${{ github.event.before }}');
});

// Task 4.1.4b: build.yml uploads the trend artifact on push; the weekly notify run reads it.
test('workflows_should_ProduceAndConsumeDocsCoverageTrend_When_YamlParsed', () => {
  const build = yaml.load(readFileSync(path.join(repoRoot, '.github/workflows/build.yml'), UTF8)) as Workflow;
  const upload = Object.values(build.jobs)
    .flatMap((job) => job.steps ?? [])
    .find((step) => step.name === 'Upload docs-coverage trend');
  assert.ok(upload, 'build.yml uploads the trend artifact');
  assert.equal(upload.with?.name, 'docs-coverage-trend');

  const trend = loadDeploy().jobs.notify.steps.find((step) => step.name === 'Docs coverage trend check');
  assert.ok(trend, 'notify has a trend step');
  assert.match(String(trend.if), /schedule/);
  assert.match(String((trend as Step & { run?: string }).run), /docscoverage trend/);
});
