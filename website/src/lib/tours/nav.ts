// Pure ordering/grouping helpers behind TourGrid and TourFooter. `tours` must
// already be the BUILT set (smoke mode removes unrecorded tours at the loader),
// so skipping unrecorded tours is by construction (Cross-cutting 6c, 7f).
import { CATEGORY_ORDER } from '../../../scripts/build-features.mjs';
import type { TourEntry } from './schema.ts';

export const START_HERE_MAX_ORDER = 3;
export const GENERIC_HELP_DOC = 'help/getting-started';

export const byOrder = (tours: readonly TourEntry[]): TourEntry[] =>
	[...tours].sort((a, b) => a.tour.order - b.tour.order);

/** Start-here strip: built tours with `tour.order` 1-3, ascending. */
export const startHere = (tours: readonly TourEntry[]): TourEntry[] =>
	byOrder(tours).filter((t) => t.tour.order <= START_HERE_MAX_ORDER);

/** Category groups in README order; categories with no built tour are omitted. */
export function groupByCategory(tours: readonly TourEntry[]): Array<{ category: string; tours: TourEntry[] }> {
	return (CATEGORY_ORDER as string[])
		.map((category) => ({ category, tours: byOrder(tours).filter((t) => t.tour.category === category) }))
		.filter((g) => g.tours.length > 0);
}

/** Neighbours in ascending `order`; undefined at the ends (last tour's next becomes "All tours"). */
export function prevNext(tours: readonly TourEntry[], id: string): { prev?: TourEntry; next?: TourEntry } {
	const sorted = byOrder(tours);
	const i = sorted.findIndex((t) => t.id === id);
	if (i < 0) return {};
	return { prev: sorted[i - 1], next: sorted[i + 1] };
}

/** Never promises a topic page that does not exist (generic help gets the generic label). */
export const helpLabel = (helpDoc: string): string =>
	helpDoc === GENERIC_HELP_DOC ? 'Getting started guide' : 'Read the help page';
