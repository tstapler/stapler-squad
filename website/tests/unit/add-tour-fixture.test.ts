// Story 3.1.1 "Adding a tour is one spec plus one entry" (R13, Task 3.1.1g).
// Needs website/scripts/expected-media.mjs (Task 2.1.2c) and `mediatool
// shard-plan` (Task 2.1.2d), both owned by Epic 2; until they exist this is
// skipped, not passing.
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it } from 'vitest';

const EXPECTED_MEDIA = join(import.meta.dirname, '..', '..', 'scripts', 'expected-media.mjs');
const SHARD_PLAN = join(import.meta.dirname, '..', '..', '..', 'tools', 'promo', 'cmd', 'mediatool', 'shardplan.go');

describe.skipIf(!existsSync(EXPECTED_MEDIA) || !existsSync(SHARD_PLAN))('add a tour with one spec and one mdx', () => {
	it.todo('newTourId_should_AppearInExpectedMediaAndExactlyOneShard_When_PairAddedInFixtureDir');
});
