import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

const REPO_ROOT = path.resolve(__dirname, '..', '..', '..');
// Every recorded or generated media output: a local run must never be able to stage these.
const SAMPLES = [
	'website/expected-media.json',
	'website/public/clips/dashboard-lifecycle.webm',
	'website/public/shots/dashboard-lifecycle-1.png',
	'website/public/media-manifest.json',
	'dom-text/dashboard-lifecycle.txt',
	'ocr-text/dashboard-lifecycle-1.txt',
	'tests/e2e/website-demo-output/media-manifest.json',
];

describe('media outputs are gitignored', () => {
	it.each(SAMPLES)('%s', (sample) => {
		// check-ignore exits 0 when the path is ignored, 1 when it is not.
		expect(() => execFileSync('git', ['check-ignore', '-q', '--no-index', sample], { cwd: REPO_ROOT })).not.toThrow();
	});
});
