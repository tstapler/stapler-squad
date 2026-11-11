// Task 3.1.1i: contrast of the annotation badge and tour text tokens, computed
// from grimdark.css. Badge = accent fill with near-black text, drawn on the
// page background; pixel-level contrast over real screenshots is an owner check.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const CSS = readFileSync(join(import.meta.dirname, '..', '..', 'src', 'styles', 'grimdark.css'), 'utf8');

function token(name: string): string {
	const m = CSS.match(new RegExp(`${name}:\\s*(#[0-9a-fA-F]{6})`));
	if (!m) throw new Error(`token ${name} not found in grimdark.css`);
	return m[1];
}

function luminance(hex: string): number {
	const [r, g, b] = [1, 3, 5].map((i) => {
		const c = parseInt(hex.slice(i, i + 2), 16) / 255;
		return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
	});
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a: string, b: string): number {
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

describe('tour colour tokens', () => {
	it('badgeText_should_Reach4_5to1_On_BadgeFill', () => {
		expect(contrast(token('--sl-color-black'), token('--sl-color-accent'))).toBeGreaterThanOrEqual(4.5);
	});

	it('badgeShape_should_Reach3to1_Against_PageBackground', () => {
		expect(contrast(token('--sl-color-accent'), token('--sl-color-black'))).toBeGreaterThanOrEqual(3);
	});

	it('mutedTourText_should_Reach4_5to1_On_CardBackground', () => {
		for (const fg of ['--sl-color-gray-2', '--sl-color-gray-3']) {
			expect(contrast(token(fg), token('--sl-color-gray-6')), fg).toBeGreaterThanOrEqual(4.5);
		}
	});

	it('focusRing_should_Reach3to1_Against_PageBackground', () => {
		expect(contrast(token('--sl-color-accent-high'), token('--sl-color-black'))).toBeGreaterThanOrEqual(3);
	});
});
