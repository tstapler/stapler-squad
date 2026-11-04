/** Narrows away null/undefined, throwing if the value is absent. Used in tests in place of `!`. */
export function defined<T>(value: T | null | undefined, what = "value"): T {
  if (value === null || value === undefined) {
    throw new Error(`expected ${what} to be defined`);
  }
  return value;
}
