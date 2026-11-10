// Story 3.1.1 / Tasks 3.1.1c, 3.1.1e: the tour frontmatter contract.
import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { afterAll, describe, expect, it } from 'vitest';
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
		.filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts'))
		.map((f) => f.replace(/\.ts$/, ''))
		.sort();

	it('tours_should_HaveMatchingSceneFile_When_MdxExists_AndViceVersa', () => {
		expect(scenes).toEqual(mdx);
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
