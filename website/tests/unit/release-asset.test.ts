import { mkdtemp, mkdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { findHtmlFiles } from '../../src/lib/release-asset.ts';

describe('findHtmlFiles', () => {
	it('findHtmlFiles_should_ReturnNestedHtmlOnly_When_DirHasMixedFiles', async () => {
		const dir = await mkdtemp(join(tmpdir(), 'html-files-'));
		await mkdir(join(dir, 'tours'));
		await writeFile(join(dir, 'index.html'), '');
		await writeFile(join(dir, 'tours', 'a.html'), '');
		await writeFile(join(dir, 'app.js'), '');

		expect((await findHtmlFiles(dir)).sort()).toEqual([join(dir, 'index.html'), join(dir, 'tours', 'a.html')]);
	});

	it('findHtmlFiles_should_ReturnEmpty_When_DirMissing', async () => {
		expect(await findHtmlFiles(join(tmpdir(), 'no-such-dir-html-files'))).toEqual([]);
	});
});
