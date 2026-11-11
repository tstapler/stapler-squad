import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

export interface Caption {
	tMs: number;
	text: string;
}

/** Reads public/clips/<clip>.transcript.json written beside the clip by the recorder. */
export function readCaptions(publicDir: string, clip: string): Caption[] {
	const path = join(publicDir, 'clips', `${clip}.transcript.json`);
	if (!existsSync(path)) throw new Error(`transcript for clip "${clip}" not found: ${path}`);
	const parsed = JSON.parse(readFileSync(path, 'utf8'));
	const list = Array.isArray(parsed) ? parsed : parsed?.captions;
	if (!Array.isArray(list) || list.length === 0) throw new Error(`transcript for clip "${clip}" is empty: ${path}`);
	return list.map((c: Caption) => ({ tMs: Number(c.tMs), text: String(c.text) }));
}
