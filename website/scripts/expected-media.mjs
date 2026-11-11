#!/usr/bin/env node
// Emits website/expected-media.json: every media name the site consumes, in the shape
// tools/promo/internal/manifest.ExpectedMedia reads:
//   {"version":1,"media":[{"name":"dashboard-lifecycle","formats":["webm","mp4"]}, ...]}
// Clips need both renditions (Cross-cutting 7e); a still (step shot, phone figure, card
// shot) is satisfied by any recorded rendition, so it carries no `formats`.
// Stays .mjs because it reads Astro content frontmatter at build time with the site's own
// YAML tooling (ADR-002). Only media a page or card actually references is listed.
//
// Usage: node scripts/expected-media.mjs [--tours DIR] [--feature-media FILE] [--out FILE]

import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import yaml from 'js-yaml';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
export const DEFAULT_TOURS_DIR = path.join(__dirname, '..', 'src', 'content', 'docs', 'tours');
export const DEFAULT_FEATURE_MEDIA = path.join(__dirname, '..', 'src', 'content', 'feature-media.json');
export const DEFAULT_OUT = path.join(__dirname, '..', 'expected-media.json');
export const CLIP_FORMATS = ['webm', 'mp4'];

export function parseFrontmatter(source) {
	const match = source.match(/^---\r?\n([\s\S]*?)\r?\n---/);
	if (!match) return {};
	const data = yaml.load(match[1]);
	return data && typeof data === 'object' ? data : {};
}

function tourFiles(dir) {
	if (!existsSync(dir)) return [];
	return readdirSync(dir)
		.filter((f) => /\.mdx?$/.test(f) && !/^index\.mdx?$/.test(f) && !f.startsWith('_'))
		.sort();
}

function add(acc, name, formats) {
	if (typeof name !== 'string' || name.trim() === '') return;
	const entry = acc.get(name) ?? new Set();
	for (const f of formats) entry.add(f);
	acc.set(name, entry);
}

export function collectTourMedia(toursDir, acc) {
	for (const file of tourFiles(toursDir)) {
		const tour = parseFrontmatter(readFileSync(path.join(toursDir, file), 'utf8')).tour;
		if (!tour || typeof tour !== 'object') {
			throw new Error(`${file}: frontmatter has no \`tour:\` object`);
		}
		add(acc, tour.clip, CLIP_FORMATS);
		if (typeof tour.clip === 'string' && tour.clip.trim() !== '') add(acc, `${tour.clip}-poster`, []); // TourClip's poster frame
		add(acc, tour.phoneFigure, []);
		for (const step of Array.isArray(tour.steps) ? tour.steps : []) add(acc, step?.shot, []);
	}
}

export function collectFeatureMedia(featureMediaPath, acc) {
	if (!existsSync(featureMediaPath)) return;
	const cards = JSON.parse(readFileSync(featureMediaPath, 'utf8'));
	for (const card of Object.values(cards)) {
		add(acc, card?.clip, CLIP_FORMATS);
		add(acc, card?.shot, []);
	}
}

/** Sorted by name; a name used as both clip and still keeps the clip's formats. */
export function buildExpectedMedia({ toursDir = DEFAULT_TOURS_DIR, featureMediaPath = DEFAULT_FEATURE_MEDIA } = {}) {
	const acc = new Map();
	collectTourMedia(toursDir, acc);
	collectFeatureMedia(featureMediaPath, acc);
	const media = [...acc.keys()].sort().map((name) => {
		const formats = [...acc.get(name)].sort();
		return formats.length > 0 ? { name, formats } : { name };
	});
	return { version: 1, media };
}

export function writeExpectedMedia(out, options) {
	const expected = buildExpectedMedia(options);
	writeFileSync(out, `${JSON.stringify(expected, null, 2)}\n`);
	return expected;
}

function arg(argv, flag, fallback) {
	const i = argv.indexOf(flag);
	return i >= 0 && argv[i + 1] ? argv[i + 1] : fallback;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	const argv = process.argv.slice(2);
	const out = arg(argv, '--out', DEFAULT_OUT);
	const expected = writeExpectedMedia(out, {
		toursDir: arg(argv, '--tours', DEFAULT_TOURS_DIR),
		featureMediaPath: arg(argv, '--feature-media', DEFAULT_FEATURE_MEDIA),
	});
	console.log(`expected-media: ${expected.media.length} item(s) -> ${out}`);
}
