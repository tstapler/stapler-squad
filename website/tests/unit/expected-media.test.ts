import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { buildExpectedMedia, writeExpectedMedia } from '../../scripts/expected-media.mjs';

const FIX = path.join(__dirname, 'fixtures', 'expected-media');
const options = { toursDir: path.join(FIX, 'tours'), featureMediaPath: path.join(FIX, 'feature-media.json') };

describe('expected-media.mjs', () => {
	it('lists tour clips with both renditions and stills without formats, sorted and de-duplicated', () => {
		expect(buildExpectedMedia(options)).toEqual({
			version: 1,
			media: [
				{ name: '01-dashboard' },
				{ name: 'alpha', formats: ['mp4', 'webm'] },
				{ name: 'alpha-1' },
				{ name: 'alpha-2' },
				{ name: 'alpha-mobile' },
				{ name: 'beta', formats: ['mp4', 'webm'] },
				{ name: 'shared-shot' },
			],
		});
	});

	it('skips the index page and tolerates a missing feature-media.json', () => {
		const names = buildExpectedMedia({ ...options, featureMediaPath: path.join(FIX, 'absent.json') }).media.map((m) => m.name);
		expect(names).not.toContain('01-dashboard');
		expect(names).toContain('beta');
	});

	it('fails loudly on a tour page without a tour object', () => {
		const dir = mkdtempSync(path.join(tmpdir(), 'tours-'));
		writeFileSync(path.join(dir, 'bad.mdx'), '---\ntitle: Bad\n---\n');
		expect(() => buildExpectedMedia({ toursDir: dir, featureMediaPath: path.join(FIX, 'absent.json') })).toThrow(/bad\.mdx/);
	});

	it('writes JSON in the shape the Go ExpectedMedia reader consumes', () => {
		const out = path.join(mkdtempSync(path.join(tmpdir(), 'em-')), 'expected-media.json');
		writeExpectedMedia(out, options);
		const parsed = JSON.parse(readFileSync(out, 'utf8'));
		expect(parsed.version).toBe(1);
		expect(parsed.media[1]).toEqual({ name: 'alpha', formats: ['mp4', 'webm'] });
	});
});
