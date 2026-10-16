import { createHash } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { checkUrl } from '../../scripts/check-asset-health.mjs';

function mockResponse({ status, body = '' }: { status: number; body?: string }) {
	return {
		status,
		arrayBuffer: async () => new TextEncoder().encode(body).buffer,
	};
}

function sha256Hex(content: string): string {
	return createHash('sha256').update(content).digest('hex');
}

describe('checkUrl', () => {
	it('checkAssetHealth_should_ExitNonZeroAndPrintUrl_When_HeadRequestReturns404', async () => {
		const url = 'https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/does-not-exist-yet.gif';
		const headFn = async () => mockResponse({ status: 404 });

		const result = await checkUrl(url, {}, { headFn });

		expect(result.ok).toBe(false);
		expect(result.url).toBe(url);
		expect(result.reason).toContain('404');
	});

	// Not in validation.md's pre-designed table, but required by the task's
	// acceptance criteria: prove the hash-mismatch path fires on a 2xx/3xx
	// response whose content no longer matches the manifest (pre-mortem
	// Failure #5 -- a --clobber re-publish serving stale/different bytes at
	// an otherwise-valid URL).
	it('checkAssetHealth_should_ExitNonZeroAndPrintBothHashes_When_ContentHashMismatches', async () => {
		const url = 'https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/review-queue.webm';
		const staleContent = 'old-clip-bytes';
		const liveContent = 'new-clobbered-clip-bytes';
		const expectedHash = sha256Hex(staleContent);
		const actualHash = sha256Hex(liveContent);

		const manifest = { 'review-queue.webm': expectedHash };
		const headFn = async () => mockResponse({ status: 200 });
		const getFn = async () => mockResponse({ status: 200, body: liveContent });

		const result = await checkUrl(url, manifest, { headFn, getFn });

		expect(result.ok).toBe(false);
		expect(result.expectedHash).toBe(expectedHash);
		expect(result.actualHash).toBe(actualHash);
		expect(result.reason).toContain(expectedHash);
		expect(result.reason).toContain(actualHash);
	});

	it('checkAssetHealth_should_ReportMissingManifestEntryViaHeadFailurePath_When_AssetNotYetPublishedToManifest', async () => {
		const url = 'https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/brand-new-clip.webm';
		const headFn = async () => mockResponse({ status: 200 });

		const result = await checkUrl(url, {}, { headFn });

		expect(result.ok).toBe(false);
		expect(result.expectedHash).toBeUndefined();
		expect(result.actualHash).toBeUndefined();
	});

	it('checkAssetHealth_should_ReportOk_When_HeadCleanAndHashMatchesManifest', async () => {
		const url = 'https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/review-queue.webm';
		const content = 'stable-clip-bytes';
		const hash = sha256Hex(content);
		const manifest = { 'review-queue.webm': hash };
		const headFn = async () => mockResponse({ status: 200 });
		const getFn = async () => mockResponse({ status: 200, body: content });

		const result = await checkUrl(url, manifest, { headFn, getFn });

		expect(result.ok).toBe(true);
	});
});
