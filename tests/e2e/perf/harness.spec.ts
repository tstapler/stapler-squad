// @feature perf:harness
/**
 * Guards on the perf harness itself (Story 1.1.3): project selection, live-instance refusal, dynamic port and
 * expected-fail semantics. None of these measure the app, so they are cheap and need no seeded fixture.
 */
import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import * as path from 'path';
import { assertNotLiveInstance } from './fixture';
import { budgetViolations, classifyViolations, loadBudgets } from './budgets';

const E2E_DIR = path.join(__dirname, '..');

function listTests(args: string[], env: Record<string, string> = {}): string[] {
  const out = execFileSync('bunx', ['playwright', 'test', '--list', ...args], {
    cwd: E2E_DIR,
    encoding: 'utf8',
    env: { ...process.env, ...env, PW_PERF: env.PW_PERF ?? '' },
  });
  return out.split('\n').map((l) => l.trim()).filter((l) => l.startsWith('['));
}

test.describe('perf harness', () => {
  test('perfProject_should_ListOnlyPerfSpecs_When_ProjectPerfSelected_AndNoneByDefault', async () => {
    test.setTimeout(120000);
    const defaults = listTests([]);
    expect(defaults.length).toBeGreaterThan(0);
    expect(defaults.filter((l) => l.includes('perf/') || l.startsWith('[perf]'))).toEqual([]);

    const perf = listTests(['--project=perf']);
    expect(perf.length).toBeGreaterThan(0);
    expect(perf.filter((l) => !l.startsWith('[perf]') || !l.includes('perf/'))).toEqual([]);
  });

  test('fixture_should_UseDynamicFreePortAndPrivateTmuxSocket_When_GlobalSetupStartsServer', async () => {
    const baseUrl = process.env.TEST_SERVER_URL;
    test.skip(!baseUrl, 'TEST_SERVER_URL is only exported when global-setup started the isolated server');
    expect(() => assertNotLiveInstance(baseUrl as string)).not.toThrow();
    const port = new URL(baseUrl as string).port;
    expect(['8543', '8444']).not.toContain(port);
    // The server global-setup spawned carries --test-mode and a private --test-dir.
    const procs = execFileSync('ps', ['-axo', 'args='], { encoding: 'utf8' }).split('\n');
    expect(procs.some((p) => p.includes('stapler-squad') && p.includes('--test-mode') && p.includes('--test-dir'))).toBe(true);
  });

  test('perfSpec_should_Abort_When_BaseUrlPointsAtPort8543', async () => {
    expect(() => assertNotLiveInstance('http://localhost:8543')).toThrow(/live instance/);
    expect(() => assertNotLiveInstance('http://127.0.0.1:8444')).toThrow(/live instance/);
    expect(() => assertNotLiveInstance('http://localhost:8544')).not.toThrow();
  });

  test('expectedFailAssertions_should_NotBeSkipped_When_OwningStoryNotYetLanded', async () => {
    const over = { GetFeatureFlags: { median: 14 }, ListWorkflows: { median: 3 }, LogClientEvents: { median: 5 } };
    const violations = budgetViolations(over, 16);
    expect(violations.sort()).toEqual(['GetFeatureFlags<=1', 'ListWorkflows<=1']);
    // A recorded expectedFail is annotated, not dropped; a regression outside the list still fails.
    const { unexpected, expectedFails } = classifyViolations(violations, ['GetFeatureFlags<=1']);
    expect(expectedFails).toEqual(['GetFeatureFlags<=1']);
    expect(unexpected).toEqual(['ListWorkflows<=1']);
  });

  test('perMethodBudgets_should_MatchPlanNumbers_When_Window16s', async () => {
    expect(loadBudgets(16)).toEqual({
      GetFeatureFlags: 1,
      ListProgramsConfig: 1,
      LogClientEvents: 5,
      GetFileTransportCapabilities: 1,
      ListWorkflows: 1,
    });
    expect(budgetViolations({ LogClientEvents: { median: 56 } }, 16)).toEqual(['LogClientEvents<=5']);
    expect(budgetViolations({ LogClientEvents: { median: 5 } }, 16)).toEqual([]);
  });
});
