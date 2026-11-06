import { describe, it, expect } from 'vitest';
import { parseIncludeDecision } from '../../scripts/sync-content.mjs';

describe('content-manifest IncludeDecision validation (Parse-Don\'t-Validate)', () => {
	it('contentManifestLoader_should_AcceptEntry_When_ExcludedEntryCarriesAReason', () => {
		const decision = parseIncludeDecision(
			{ source: 'docs/how-to/run-nil-safety-analysis.md', include: false, reason: 'dev-only tooling' },
			0,
		);

		expect(decision).toEqual({
			source: 'docs/how-to/run-nil-safety-analysis.md',
			include: false,
			reason: 'dev-only tooling',
		});
	});

	it('contentManifestLoader_should_RejectEntry_When_ExcludedEntryHasNoReason', () => {
		expect(() =>
			parseIncludeDecision({ source: 'docs/how-to/foo.md', include: false }, 0),
		).toThrowError(/reason/);
	});
});
