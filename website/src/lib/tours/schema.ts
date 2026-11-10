// Tour frontmatter contract (ADR-001, Story 3.1.1): the nested `tour:` object
// required on every doc under tours/ (except index.mdx). Kept free of
// astro:content imports so unit tests can parse fixtures directly.
import { z } from 'astro/zod';
import { CATEGORY_ORDER } from '../../../scripts/build-features.mjs';

export const MIN_STEPS = 3;
export const MAX_STEPS = 6;
export const TOURS_DIR = 'tours';
export const ALT_MIN = 20;
export const ALT_MAX = 300;
export const MARKER_WORD = /\bmarkers?\b/i;

const categories = CATEGORY_ORDER as [string, ...string[]];

const nonEmpty = (field: string) => z.string().trim().min(1, `${field} must not be empty`);

export const tourStepSchema = z
	.object({
		text: nonEmpty('step.text'),
		shot: nonEmpty('step.shot'),
		alt: nonEmpty('step.alt')
			.pipe(z.string().min(ALT_MIN, `step.alt must be at least ${ALT_MIN} characters`).max(ALT_MAX, `step.alt must be at most ${ALT_MAX} characters`)),
		caption: nonEmpty('step.caption'),
		// What happens on screen: the transcript line (WCAG 1.2.1/1.2.3).
		narration: nonEmpty('step.narration'),
		why: z.string().trim().min(1).optional(),
		quote: z.string().trim().min(1).optional(),
		annotated: z.boolean().default(true),
	})
	.refine((s) => s.narration !== s.text, {
		path: ['narration'],
		message: 'step.narration must differ from step.text (narration describes the screen, text is the instruction)',
	})
	.refine((s) => s.alt !== s.caption && s.alt !== s.shot, {
		path: ['alt'],
		message: 'step.alt must differ from step.caption and step.shot (describe the screen, not the label)',
	})
	.refine((s) => !s.annotated || MARKER_WORD.test(s.caption), {
		path: ['caption'],
		message: 'an annotated step caption must name its markers (the word "marker" or "markers")',
	});

export type TourStep = z.infer<typeof tourStepSchema>;

/** `docIds`: valid website docs collection ids for `helpDoc`; omit to skip that check. */
export function makeTourSchema(docIds?: ReadonlySet<string>) {
	return z.object({
		title: nonEmpty('tour.title'),
		outcome: nonEmpty('tour.outcome'),
		clip: nonEmpty('tour.clip'),
		features: z.array(nonEmpty('tour.features[]')).min(1, 'tour.features needs at least one registry id'),
		helpDoc: nonEmpty('tour.helpDoc').refine((id) => !docIds || docIds.has(id), {
			message: 'tour.helpDoc is not a website docs collection id (e.g. help/getting-started)',
		}),
		order: z.number().int().positive(),
		category: z.enum(categories),
		audience: nonEmpty('tour.audience'),
		durationMin: z.number().positive(),
		flagNote: z.string().trim().min(1).optional(),
		phoneFigure: z.string().trim().min(1).optional(),
		steps: z.array(tourStepSchema).min(MIN_STEPS).max(MAX_STEPS),
	});
}

export type Tour = z.infer<ReturnType<typeof makeTourSchema>>;

/** One parsed tour page: docs-collection id plus its frontmatter. */
export interface TourEntry {
	id: string;
	tour: Tour;
}

/** `tour.order` must be exactly 1..N across the set (Cross-cutting 7f). Returns error strings. */
export function checkOrderSet(tours: readonly TourEntry[]): string[] {
	const errors: string[] = [];
	const seen = new Map<number, string>();
	for (const { id, tour } of tours) {
		const other = seen.get(tour.order);
		if (other) errors.push(`${id}: tour.order ${tour.order} duplicates ${other}`);
		else seen.set(tour.order, id);
	}
	const n = tours.length;
	for (const { id, tour } of tours) {
		if (tour.order > n) errors.push(`${id}: tour.order ${tour.order} is outside 1..${n}; orders must be exactly 1..${n} with no gaps`);
	}
	return errors;
}

/** Docs-collection ids of tour pages (excludes the index). */
export function isTourId(id: string): boolean {
	return id.startsWith(`${TOURS_DIR}/`) && id !== `${TOURS_DIR}` && id !== `${TOURS_DIR}/index`;
}
