// The Go side (tools/promo/internal/manifest, contract_test.go) round-trips this same file, so
// a field rename on either side fails a test here or there.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { clipDurationMs, clipUrls, hasClip, hasShot, parseManifest, shotSize, shotUrl, stampParts } from '../../src/lib/tours/manifest.ts';

const EXAMPLE = join(__dirname, '..', '..', '..', 'tools', 'promo', 'schema', 'media-manifest.example.json');
const wire = () => JSON.parse(readFileSync(EXAMPLE, 'utf8'));

describe('media-manifest wire contract (Go example file)', () => {
	const m = parseManifest(wire());

	it('maps finishedAt to recordedAt and keeps the commit', () => {
		expect(m.recordedAt).toBe('2026-11-10T12:00:00Z');
		expect(stampParts(m)).toEqual({ date: '2026-11-10', sha: 'a1b2c3d' });
	});

	it('treats a recorded webm row as the clip, with its duration', () => {
		expect(hasClip(m, 'dashboard-lifecycle')).toBe(true);
		expect(clipDurationMs(m, 'dashboard-lifecycle')).toBe(45_000);
	});

	it('does not count a failed clip', () => {
		expect(hasClip(m, 'approval-review')).toBe(false);
	});

	it('maps image rows to shots with form factor and pixel size', () => {
		expect(hasShot(m, 'dashboard-overview')).toBe(true);
		expect(hasShot(m, 'dashboard-mobile', 'mobile')).toBe(true);
		expect(hasShot(m, 'dashboard-mobile', 'desktop')).toBe(false);
		expect(shotSize(m, 'dashboard-mobile', 'mobile')).toEqual({ width: 390, height: 844 });
		expect(shotSize(m, 'dashboard-overview')).toEqual({ width: 1280, height: 720 });
	});

	it('builds URLs that match the src paths the Go rows carry', () => {
		const rows = wire().entries as { name: string; format: string; src?: string; poster?: string }[];
		const urls = clipUrls('', 'dashboard-lifecycle');
		const webm = rows.find((r) => r.name === 'dashboard-lifecycle' && r.format === 'webm')!;
		const mp4 = rows.find((r) => r.name === 'dashboard-lifecycle' && r.format === 'mp4')!;
		expect(urls.webm).toBe(`/${webm.src}`);
		expect(urls.mp4).toBe(`/${mp4.src}`);
		expect(urls.poster).toBe(`/${webm.poster}`);
		expect(shotUrl('', 'dashboard-mobile')).toBe(`/${rows.find((r) => r.name === 'dashboard-mobile')!.src}`);
	});

	it('rejects a manifest without finishedAt', () => {
		const bad = wire();
		delete bad.finishedAt;
		expect(() => parseManifest(bad)).toThrow(/finishedAt/);
	});
});
