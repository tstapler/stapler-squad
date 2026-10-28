import { expect, type APIRequestContext } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const rpc = (name: string) => `${BASE_URL}/api/session.v1.SessionService/${name}`;
const json = { 'Content-Type': 'application/json' };

export async function flagEnabled(request: APIRequestContext, name: string): Promise<boolean> {
  const res = await request.post(rpc('GetFeatureFlags'), { headers: json, data: {} });
  const flags = (await res.json()).flags as { name: string; enabled?: boolean }[];
  return flags.find((f) => f.name === name)?.enabled ?? false;
}

/** Sets a flag on the isolated test server and reads it back (a silently ignored write must fail). */
export async function setFlag(request: APIRequestContext, name: string, enabled: boolean): Promise<void> {
  await request.post(rpc('UpdateFeatureFlag'), { headers: json, data: { name, enabled } });
  expect(await flagEnabled(request, name)).toBe(enabled);
}
