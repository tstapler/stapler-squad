'use strict';

// AC #7: this item's comment-gating changes must not affect any existing
// blocking check. Parses the real workflow YAML (not a copy) and asserts on
// the specific steps AC #7 names, so a future edit that accidentally loosens
// one of these is caught here instead of only by a human re-reading a diff.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');

const WORKFLOWS_DIR = path.join(__dirname, '..', '..', '..', '.github', 'workflows');

function loadWorkflow(name) {
  return yaml.load(fs.readFileSync(path.join(WORKFLOWS_DIR, name), 'utf8'));
}

function findStep(doc, jobName, stepName) {
  const job = doc.jobs[jobName];
  assert.ok(job, `job "${jobName}" not found`);
  const step = (job.steps || []).find((s) => s.name === stepName);
  assert.ok(step, `step "${stepName}" not found in job "${jobName}"`);
  return step;
}

test('uxAnalysis_AxeStep_should_NotHaveContinueOnError_When_ParsedFromRealYAML', () => {
  const doc = loadWorkflow('ux-analysis.yml');
  const step = findStep(doc, 'ux-analysis', 'Run Axe accessibility tests');
  assert.notEqual(step['continue-on-error'], true);
});

test('build_CheckNewRPCsHaveTestsStep_should_NotHaveContinueOnError_When_ParsedFromRealYAML', () => {
  // Moved from the `test` job to `test-affected` (registry-validation.yml's
  // removal, folded its exact-match divergence check into this step) -- see
  // that commit's message for why the two checks were redundant.
  const doc = loadWorkflow('build.yml');
  const step = findStep(doc, 'test-affected', 'Check new RPCs have tests');
  assert.notEqual(step['continue-on-error'], true);
});
