// Story 3.1.1 / Tasks 3.1.1c, 3.1.1e: the tour frontmatter contract.
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { afterAll, describe, expect, it } from 'vitest';
import { parseFrontmatter } from '../../src/lib/tours/catalog.ts';
import { checkOrderSet, makeTourSchema, MAX_STEPS, MIN_STEPS } from '../../src/lib/tours/schema.ts';
import { buildWithFixtures, FIXTURE_ID, fixtureSteps, tourMdx, type FixtureBuild } from '../helpers/tour-fixture.ts';

const REPO_ROOT = join(import.meta.dirname, '..', '..', '..');
const DOCS_DIR = join(import.meta.dirname, '..', '..', 'src', 'content', 'docs');
const SCENES_DIR = join(REPO_ROOT, 'tests', 'e2e', 'website-demos', 'tours');

function validTour(overrides: Record<string, unknown> = {}) {
	return {
		title: 'T',
		outcome: 'O',
		clip: 'c',
		features: ['dashboard'],
		helpDoc: 'help/getting-started',
		order: 1,
		category: 'Visibility',
		audience: 'Anyone',
		durationMin: 2,
		steps: fixtureSteps(3),
		...overrides,
	};
}

describe('tour schema', () => {
	const schema = makeTourSchema(new Set(['help/getting-started']));

	it('tourSchema_should_Accept_When_ThreeToSixSteps_And_Reject_When_TwoOrSeven', () => {
		for (const n of [2, 7]) {
			expect(schema.safeParse(validTour({ steps: fixtureSteps(n) })).success, `${n} steps`).toBe(false);
		}
		for (let n = MIN_STEPS; n <= MAX_STEPS; n++) {
			expect(schema.safeParse(validTour({ steps: fixtureSteps(n) })).success, `${n} steps`).toBe(true);
		}
	});

	it('tourSchema_should_NameTheField_When_StepAltMissing', () => {
		const steps = fixtureSteps(3);
		delete (steps[1] as Partial<(typeof steps)[number]>).alt;
		const result = schema.safeParse(validTour({ steps }));
		expect(result.success).toBe(false);
		expect(JSON.stringify(result.error?.issues.map((i) => i.path))).toContain('["steps",1,"alt"]');
	});

	it('tourSchema_should_Reject_When_NarrationEqualsText', () => {
		const steps = fixtureSteps(3);
		steps[0].narration = steps[0].text;
		expect(schema.safeParse(validTour({ steps })).success).toBe(false);
	});

	it('tourSchema_should_Reject_When_HelpDocIsNotAWebsiteDocId', () => {
		const result = schema.safeParse(validTour({ helpDoc: 'help/does-not-exist' }));
		expect(result.success).toBe(false);
		expect(result.error?.issues[0].path).toEqual(['helpDoc']);
	});

	it('tourSchema_should_Reject_When_FeaturesEmptyOrCategoryUnknown', () => {
		expect(schema.safeParse(validTour({ features: [] })).success).toBe(false);
		expect(schema.safeParse(validTour({ category: 'Nope' })).success).toBe(false);
	});

	it('tourSchema_should_DefaultAnnotatedTrue_When_Omitted', () => {
		const parsed = schema.parse(validTour());
		expect(parsed.steps.every((s) => s.annotated === true)).toBe(true);
	});

	it('tourSchema_should_Reject_When_AltOutsideTwentyToThreeHundredChars', () => {
		const withAlt = (alt: string) => {
			const steps = fixtureSteps(3);
			steps[0].alt = alt;
			return schema.safeParse(validTour({ steps })).success;
		};
		expect(withAlt('x'.repeat(19))).toBe(false);
		expect(withAlt('x'.repeat(20))).toBe(true);
		expect(withAlt('x'.repeat(300))).toBe(true);
		expect(withAlt('x'.repeat(301))).toBe(false);
	});

	it('tourSchema_should_Reject_When_AltEqualsCaptionOrShotName', () => {
		const same = 'Marker 1 shows the sidebar list';
		const sameCaption = fixtureSteps(3);
		sameCaption[0].alt = same;
		sameCaption[0].caption = same;
		expect(schema.safeParse(validTour({ steps: sameCaption })).success, 'alt equals caption').toBe(false);
		const sameShot = fixtureSteps(3);
		sameShot[0].shot = 'zz-fixture-a-long-shot-name';
		sameShot[0].alt = 'zz-fixture-a-long-shot-name';
		expect(schema.safeParse(validTour({ steps: sameShot })).success, 'alt equals shot').toBe(false);
	});

	it('tourSchema_should_RequireMarkerWord_When_StepAnnotated_And_NotWhenUnannotated', () => {
		const steps = fixtureSteps(3);
		steps[0].caption = 'The dashboard header';
		expect(schema.safeParse(validTour({ steps })).success).toBe(false);
		(steps[0] as { annotated?: boolean }).annotated = false;
		expect(schema.safeParse(validTour({ steps })).success).toBe(true);
		steps[1].caption = 'MARKERS 1 and 2 show the tabs';
		expect(schema.safeParse(validTour({ steps })).success).toBe(true);
	});

	it('checkOrderSet_should_Report_When_OrderHasGapOrDoesNotStartAtOne', () => {
		const t = (order: number) => schema.parse(validTour({ order }));
		expect(checkOrderSet([{ id: 'tours/a', tour: t(1) }, { id: 'tours/b', tour: t(3) }])).toHaveLength(1);
		expect(checkOrderSet([{ id: 'tours/a', tour: t(2) }, { id: 'tours/b', tour: t(3) }])).toHaveLength(1);
		expect(checkOrderSet([{ id: 'tours/a', tour: t(2) }, { id: 'tours/b', tour: t(1) }])).toEqual([]);
	});

	it('checkOrderSet_should_ReportDuplicate_When_TwoToursShareOrder', () => {
		const a = { id: 'tours/a', tour: schema.parse(validTour({ order: 1 })) };
		const b = { id: 'tours/b', tour: schema.parse(validTour({ order: 1 })) };
		expect(checkOrderSet([a, b])).toEqual(['tours/b: tour.order 1 duplicates tours/a']);
		expect(checkOrderSet([a, { ...b, tour: { ...b.tour, order: 2 } }])).toEqual([]);
	});
});

// Cross-cutting 6f: a tour is exactly one .mdx plus one scene file, nothing else registered.
describe('tour sources vs scene files', () => {
	const mdx = (existsSync(join(DOCS_DIR, 'tours')) ? readdirSync(join(DOCS_DIR, 'tours')) : [])
		.filter((f) => f.endsWith('.mdx') && f !== 'index.mdx')
		.map((f) => f.replace(/\.mdx$/, ''))
		.sort();
	const scenes = (existsSync(SCENES_DIR) ? readdirSync(SCENES_DIR) : [])
		.filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts') && !f.startsWith('_'))
		.map((f) => f.replace(/\.ts$/, ''))
		.sort();

	it('tours_should_HaveMatchingSceneFile_When_MdxExists_AndViceVersa', () => {
		expect(scenes).toEqual(mdx);
	});
});

// Epic 3.2 content quality: real tours must satisfy the schema, the order set and the flag-note rule.
describe('real tours content quality', () => {
	const ids = readdirSync(join(DOCS_DIR, 'tours'))
		.filter((f) => f.endsWith('.mdx') && f !== 'index.mdx')
		.map((f) => f.replace(/\.mdx$/, ''))
		.sort();
	const tours = ids.map((id) => ({
		id,
		tour: parseFrontmatter(readFileSync(join(DOCS_DIR, 'tours', `${id}.mdx`), 'utf8')).tour as Record<string, unknown>,
	}));
	const schema = makeTourSchema();

	it('realTours_should_ParseAndHaveOrderExactlyOneToN', () => {
		const entries = tours.map(({ id, tour }) => ({ id: `tours/${id}`, tour: schema.parse(tour) }));
		expect(checkOrderSet(entries)).toEqual([]);
	});

	it('realTours_should_HaveNonEmptyFlagNote_Exactly_When_SceneExportsFlagsEnabled', () => {
		for (const { id, tour } of tours) {
			const scene = readFileSync(join(SCENES_DIR, `${id}.ts`), 'utf8');
			const flagged = /^export\s+const\s+flagsEnabled\b/m.test(scene);
			const hasNote = typeof tour.flagNote === 'string' && tour.flagNote.trim() !== '';
			expect(hasNote, `${id}: flagNote present iff scene exports flagsEnabled`).toBe(flagged);
		}
	});
});

describe('tour schema at build time', () => {
	const builds: FixtureBuild[] = [];
	afterAll(() => builds.forEach((b) => b.cleanup()));

	it('build_should_FailNamingStepAltField_When_StepAltMissing', () => {
		const steps = fixtureSteps(3);
		delete (steps[1] as Partial<(typeof steps)[number]>).alt;
		const b = buildWithFixtures({ tours: { [FIXTURE_ID]: tourMdx({ steps }) } });
		builds.push(b);
		expect(b.status).not.toBe(0);
		expect(b.output).toContain(`tours/${FIXTURE_ID}.mdx: tour.steps.1.alt`);
	}, 120_000);

	it('build_should_FailNamingTourAndField_When_HelpDocNotInCollection', () => {
		const b = buildWithFixtures({ tours: { [FIXTURE_ID]: tourMdx({ helpDoc: 'help/does-not-exist' }) } });
		builds.push(b);
		expect(b.status).not.toBe(0);
		expect(b.output).toContain(`tours/${FIXTURE_ID}.mdx: tour.helpDoc`);
	}, 120_000);

	it('build_should_FailNamingFile_When_TourObjectMissing', () => {
		const b = buildWithFixtures({ tours: { [FIXTURE_ID]: '---\ntitle: No tour\n---\n' } });
		builds.push(b);
		expect(b.status).not.toBe(0);
		expect(b.output).toContain(`tours/${FIXTURE_ID}.mdx: missing required "tour"`);
	}, 120_000);
});
