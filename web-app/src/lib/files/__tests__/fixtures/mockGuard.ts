/** Modules the shared fixtures already mock; a files test must not re-mock them. */
export const HARNESS_MOCKED_MODULES: readonly string[] = [
  "@/lib/api/transport",
  "@/components/sessions/CodeEditor",
];

const MOCK_CALL = /jest\.(?:mock|doMock)\(\s*(["'`])([^"'`]+)\1/g;

/** Harness-covered modules that `source` mocks itself. */
export function findDuplicateMocks(source: string): string[] {
  const hits = [...source.matchAll(MOCK_CALL)].map((m) => m[2]);
  return hits.filter((mod) => HARNESS_MOCKED_MODULES.includes(mod));
}
