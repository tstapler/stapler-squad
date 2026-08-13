/**
 * Feature catalog type definitions.
 *
 * A Feature is the single source of truth for a product capability: it links
 * the RPC IDs, component paths, test IDs, and metadata together in one place.
 */
export interface Feature {
  /** Kebab-case unique identifier, e.g. "session-create". */
  readonly id: string;
  readonly title: string;
  /** Markdown-formatted description. */
  readonly description: string;
  /** Proto scope:action RPC IDs this feature exposes, e.g. ["session:create"]. */
  readonly rpcIds: readonly string[];
  /** Repo-relative paths to the primary React components. */
  readonly componentPaths: readonly string[];
  /** Playwright describe > test names or Jest test IDs that cover this feature. */
  readonly testIds: readonly string[];
  readonly status: 'stable' | 'experimental' | 'deprecated';
  /** Semver string, e.g. "1.4.0". */
  readonly since: string;
}
