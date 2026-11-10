// @feature website-demos
import { describe, expect, test } from 'bun:test';
import * as fs from 'fs';
import * as path from 'path';
import { THEME_BACKGROUNDS, THEME_ID, missingQuotes, pathMatches, themeVerdict } from '../assertions';
import { BADGE_COLORS } from '../shots';

describe('themeVerdict', () => {
  test('a clean-themed page fails with the documented message', () => {
    expect(themeVerdict({ stored: 'grimdark', background: THEME_BACKGROUNDS.clean })).toBe('expected theme class grimdark, got clean');
  });
  test('grimdark passes', () => {
    expect(themeVerdict({ stored: 'grimdark', background: THEME_BACKGROUNDS.grimdark })).toBeNull();
  });
  test('an unknown background is reported, not accepted', () => {
    expect(themeVerdict({ stored: 'grimdark', background: 'rgb(1, 2, 3)' })).toContain('unknown');
  });
  test('the theme id matches the Go manifest constant', () => {
    const go = fs.readFileSync(path.resolve(__dirname, '../../../../tools/promo/internal/manifest/manifest.go'), 'utf8');
    expect(go).toContain(`const ThemeID = "${THEME_ID}"`);
  });
});

describe('THEME_BACKGROUNDS parity with web-app theme.css.ts', () => {
  const css = fs.readFileSync(path.resolve(__dirname, '../../../../web-app/src/styles/theme.css.ts'), 'utf8');
  const hexToRgb = (h: string) => `rgb(${[1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16)).join(', ')})`;
  const themeVar: Record<string, string> = { light: 'lightTheme', dark: 'darkTheme', matrix: 'matrixTheme', cyberpunk77: 'cyberpunk77Theme', grimdark: 'grimdarkTheme', clean: 'cleanTheme' };
  for (const [id, varName] of Object.entries(themeVar)) {
    test(`${id} background matches ${varName}`, () => {
      const block = css.slice(css.indexOf(`export const ${varName} = createTheme`));
      const hex = /background: "(#[0-9a-fA-F]{6})"/.exec(block)![1];
      expect(THEME_BACKGROUNDS[id]).toBe(hexToRgb(hex));
    });
  }
});

describe('landing and quotes', () => {
  test('flag-redirected page fails the path check', () => {
    expect(pathMatches('http://x/backlog/board', '/backlog/board')).toBe(true);
    expect(pathMatches('http://x/backlog/board/', '/backlog/board')).toBe(true);
    expect(pathMatches('http://x/', '/backlog/board')).toBe(false);
  });
  test('missingQuotes lists the quotes absent from the DOM text', () => {
    expect(missingQuotes('Approve the push now', ['Approve the push', 'Deny'])).toEqual(['Deny']);
  });
});

describe('annotation badge contrast (UX-18)', () => {
  const lum = (hex: string) => {
    const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
  };
  test('badge text on fill is at least 4.5:1', () => {
    const [a, b] = [lum(BADGE_COLORS.fill), lum(BADGE_COLORS.text)].sort((x, y) => y - x);
    expect((a + 0.05) / (b + 0.05)).toBeGreaterThanOrEqual(4.5);
  });
});
