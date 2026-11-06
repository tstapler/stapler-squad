export interface SourceFile {
  path: string;
  content: string;
}

// Only a bare quoted file name counts as a reference, so path strings in prose or
// fixtures (e.g. "web-app/build/__tests__/bun/x.bun.ts") are not mistaken for runInBun calls.
const BARE_BUN_SCRIPT = /["'`]([\w.-]+\.bun\.ts)["'`]/g;
const RUN_IN_BUN_IMPORT = /from\s+["'][^"']*run-in-bun["']/;

function basename(p: string): string {
  return p.slice(p.lastIndexOf("/") + 1);
}

function referencedScripts(content: string): string[] {
  return [...content.matchAll(BARE_BUN_SCRIPT)].map((m) => m[1]);
}

/** Violations of "every *.bun.ts is driven through runInBun by some *.test.ts". */
export function findBunCoverageViolations(scripts: SourceFile[], tests: SourceFile[]): string[] {
  const violations: string[] = [];
  for (const script of scripts) {
    const name = basename(script.path);
    const driven = tests.some((t) => RUN_IN_BUN_IMPORT.test(t.content) && referencedScripts(t.content).includes(name));
    if (!driven) {
      violations.push(`${script.path} is not driven by any test that imports run-in-bun`);
    }
  }
  for (const test of tests) {
    if (referencedScripts(test.content).length > 0 && !RUN_IN_BUN_IMPORT.test(test.content)) {
      violations.push(`${test.path} mentions a .bun.ts script but does not import run-in-bun`);
    }
  }
  return violations;
}
