import { describe, expect, it } from 'vitest';
import { GENERIC_HELP_DOC, groupByCategory, helpLabel, prevNext, startHere } from '../../src/lib/tours/nav.ts';
import type { TourEntry } from '../../src/lib/tours/schema.ts';
import { formatWatch, stampParts } from '../../src/lib/tours/manifest.ts';

const t = (id: string, order: number, category = 'Visibility'): TourEntry =>
	({ id: `tours/${id}`, tour: { order, category, title: id } }) as unknown as TourEntry;

describe('tour navigation helpers', () => {
	const all = [t('d', 4), t('b', 2, 'Review & Approval'), t('a', 1), t('c', 3, 'Organisation')];

	it('startHere_should_ReturnOrders1To3Ascending_When_AllBuilt', () => {
		expect(startHere(all).map((x) => x.id)).toEqual(['tours/a', 'tours/b', 'tours/c']);
	});

	it('startHere_should_ShowOnlyRecordedSubset_When_FewerThanThreeBuilt', () => {
		expect(startHere([t('a', 1)]).map((x) => x.id)).toEqual(['tours/a']);
		expect(startHere([])).toEqual([]);
	});

	it('groupByCategory_should_OmitEmptyCategories_And_FollowReadmeOrder', () => {
		expect(groupByCategory(all).map((g) => g.category)).toEqual(['Visibility', 'Organisation', 'Review & Approval']);
	});

	it('prevNext_should_FollowOrder_And_OmitAtEnds', () => {
		expect(prevNext(all, 'tours/b')).toMatchObject({ prev: { id: 'tours/a' }, next: { id: 'tours/c' } });
		expect(prevNext(all, 'tours/a').prev).toBeUndefined();
		expect(prevNext(all, 'tours/d').next).toBeUndefined();
		expect(prevNext([t('a', 1)], 'tours/a')).toEqual({ prev: undefined, next: undefined });
	});

	it('helpLabel_should_NotPromiseTopicPage_When_GenericGettingStarted', () => {
		expect(helpLabel(GENERIC_HELP_DOC)).toBe('Getting started guide');
		expect(helpLabel('help/enable-pi-support')).toBe('Read the help page');
	});

	it('formatWatch_should_FormatMinutesSeconds_And_OmitWhenUnknown', () => {
		expect(formatWatch(45_000)).toBe('0:45');
		expect(formatWatch(60_000)).toBe('1:00');
		expect(formatWatch(undefined)).toBeUndefined();
	});

	it('stampParts_should_ReturnDateAndShortSha', () => {
		expect(stampParts({ commit: 'a1b2c3d4e5', recordedAt: '2026-11-10T12:00:00Z', entries: [] })).toEqual({ date: '2026-11-10', sha: 'a1b2c3d' });
	});
});
