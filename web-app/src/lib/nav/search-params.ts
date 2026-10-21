// Pure parsers over a query string; no router import so they run under any impl.

type ParamSource = Pick<URLSearchParams, "get">;

export function stringParam(params: ParamSource, key: string): string | undefined {
  const value = params.get(key);
  return value === null || value === "" ? undefined : value;
}

/** Base-10 integer, or undefined when absent or not a whole number. */
export function intParam(params: ParamSource, key: string): number | undefined {
  const value = params.get(key);
  if (value === null || !/^-?\d+$/.test(value)) return undefined;
  return Number(value);
}

export function enumParam<T extends string>(params: ParamSource, key: string, allowed: readonly T[]): T | undefined {
  const value = params.get(key);
  return allowed.find((candidate) => candidate === value);
}
