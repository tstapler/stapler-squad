import { describe, expect, it } from 'vitest';
import { releaseAssetUrl } from '../../src/lib/release-asset.ts';

describe('releaseAssetUrl', () => {
	it('releaseAssetUrl_should_ReturnExpectedUrl_When_GivenTagNameExt', () => {
		const url = releaseAssetUrl({ tag: 'website-demo-gifs-latest', name: 'review-queue', ext: 'webm' });

		expect(url).toBe(
			'https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/review-queue.webm',
		);
	});
});
