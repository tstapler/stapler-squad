import * as path from 'path';

/** Theme storageState files, keyed by project name suffix; one copy per server slot (each has its own origin). */
export const THEME_FIXTURES: Record<string, string> = {
  matrix: 'matrix',
  cyberpunk77: 'cyberpunk77',
  wh40k: 'wh40k',
  clean: 'clean',
};

export function themeFixturePath(name: string, slot: number): string {
  return path.join(__dirname, '..', 'fixtures', `${name}-theme.${slot}.json`);
}
