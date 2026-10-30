// The site's palette must track the app's own WH40K theme, and the favicon must
// be the logo's mark -- both silently rot otherwise.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const read = (...parts: string[]) => readFileSync(join(WEBSITE_ROOT, ...parts), 'utf8');

const appTheme = read('..', 'web-app', 'src', 'styles', 'theme.css.ts');
const wh40kBlock = appTheme.slice(appTheme.indexOf('export const wh40kTheme'));
const appColor = (token: string) => wh40kBlock.match(new RegExp(`\\b${token}: "(#[0-9a-fA-F]{6})"`))?.[1];
const siteCss = read('src', 'styles', 'wh40k.css');

describe('website WH40K theme', () => {
	// site CSS variable -> app theme token it must equal
	const SHARED: [string, string][] = [
		['--sl-color-black', 'background'],
		['--sl-color-gray-6', 'cardBackground'],
		['--sl-color-gray-7', 'modalBackground'],
		['--sl-color-gray-5', 'borderColor'],
		['--sl-color-gray-1', 'textPrimary'],
		['--sl-color-gray-3', 'textSecondary'],
		['--sl-color-gray-4', 'textMuted'],
		['--sl-color-accent', 'primary'],
		['--sl-color-accent-high', 'primaryHover'],
	];

	it.each(SHARED)('themeCss_should_MatchAppToken_When_%s', (cssVar, token) => {
		const appValue = appColor(token);
		expect(appValue, `app wh40kTheme has no ${token}`).toBeDefined();
		const siteValue = siteCss.match(new RegExp(`${cssVar}: (#[0-9a-fA-F]{6})`))?.[1];
		expect(siteValue?.toLowerCase()).toBe(appValue?.toLowerCase());
	});

	it('themeCss_should_LoadFromAstroConfig_When_BuildingSite', () => {
		const config = read('astro.config.mjs');
		expect(config).toContain("customCss: ['./src/styles/wh40k.css']");
		expect(config).toContain("themes: ['vitesse-dark']");
	});
});

describe('favicon', () => {
	it('favicon_should_BeTheLogoStaples_When_Generated', () => {
		const logo = read('..', 'web-app', 'public', 'logo.svg');
		const favicon = read('public', 'favicon.svg');
		// Each of the logo's first three staples' path data appears verbatim in the favicon.
		const staplePaths = [...logo.matchAll(/<path d="(M(?:109|202|295),40[^"]*)"/g)].map((m) => m[1]);
		expect(staplePaths).toHaveLength(3);
		for (const d of staplePaths) expect(favicon).toContain(d);
	});

	it('favicon_should_BeLinkedWithPngFallbacks_When_ConfigRendered', () => {
		const config = read('astro.config.mjs');
		expect(config).toContain("favicon: '/favicon.svg'");
		expect(config).toContain('favicon-32.png');
		expect(config).toContain('apple-touch-icon.png');
	});
});
