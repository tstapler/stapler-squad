// Used by the next-nav-to-facade-tests codemod to turn a file's old `next/navigation` + `next/link`
// mock factories into one `@/lib/nav` mock (two jest.mock calls for one module would override each other).
import * as memory from "./impl.memory";

type Factory<T> = () => T;
type LinkModule = unknown;

const unwrapDefault = (mod: LinkModule): unknown =>
  mod && typeof mod === "object" && "__esModule" in mod ? (mod as unknown as { default: unknown }).default : mod;

/** In-memory defaults, then the old navigation mock's hooks, then the old link mock as `Link`. */
export function navMockFromNext({
  navigation,
  link,
}: {
  navigation?: Factory<Record<string, unknown>>;
  link?: Factory<LinkModule>;
}): Record<string, unknown> {
  return {
    ...memory,
    ...navigation?.(),
    ...(link ? { Link: unwrapDefault(link()) } : {}),
  };
}
