/**
 * Terminal Output Filters
 *
 * Filter pipeline for terminal output processing. Supports filtering,
 * transformation, and sanitization of terminal output before it reaches
 * the xterm.js terminal.
 *
 * Filters are applied in sequence and can be composed. Each filter
 * can transform content, block content, or tag content for downstream
 * processing.
 */

import { stripAnsiCodes } from "./CopyManager";

/**
 * A filter function that processes terminal output.
 * Return null to drop the line entirely.
 * Return a string to transform the line.
 * Return undefined (or the input) to pass through unchanged.
 */
export type OutputFilterFunction = (
  line: string,
  context: FilterContext
) => string | null | undefined;

/**
 * Context passed to each filter function.
 */
export interface FilterContext {
  /** Session ID for context-aware filtering */
  sessionId: string;
  /** Line number in the terminal output */
  lineNumber: number;
  /** Previous line (if available) */
  previousLine?: string;
  /** Tags from upstream processing */
  tags: string[];
  /** Session metadata */
  sessionName?: string;
  /** Whether this output is from an error stream */
  isStderr: boolean;
}

/**
 * Configuration for an output filter.
 */
export interface OutputFilterConfig {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  priority: number;
  filter: OutputFilterFunction;
}

/**
 * Built-in filter types for common use cases.
 */
export type FilterType =
  | "strip-ansi"
  | "error-highlight"
  | "timestamp-inject"
  | "line-limit"
  | "custom";

/**
 * Configuration for a built-in filter.
 */
export interface BuiltInFilterConfig {
  type: FilterType;
  enabled?: boolean;
  options?: Record<string, unknown>;
}

/**
 * Default filter configurations.
 */
export function getDefaultFilters(): BuiltInFilterConfig[] {
  return [
    {
      type: "strip-ansi",
      enabled: false, // Usually keep ANSI for terminal rendering
      options: {},
    },
    {
      type: "error-highlight",
      enabled: true,
      options: {
        patterns: [
          "\\b(ERROR|FATAL|PANIC)\\b",
          "\\b(exception|Exception)\\b",
          "\\b(failed|failure)\\b",
        ],
        prefix: "⚠️  ",
      },
    },
    {
      type: "timestamp-inject",
      enabled: false,
      options: {
        format: "YYYY-MM-DD HH:mm:ss",
      },
    },
    {
      type: "line-limit",
      enabled: true,
      options: {
        maxLines: 100,
      },
    },
  ];
}

/**
 * OutputFilterPipeline — manages a chain of output filters.
 *
 * Filters are applied in priority order (lowest priority runs first).
 * Each filter can transform, tag, or drop output lines.
 */
export class OutputFilterPipeline {
  private filters: Map<string, OutputFilterConfig> = new Map();
  private sessionId: string;

  constructor(sessionId: string) {
    this.sessionId = sessionId;
  }

  /**
   * Register a filter with the pipeline.
   */
  register(config: OutputFilterConfig): void {
    this.filters.set(config.id, config);
  }

  /**
   * Unregister a filter from the pipeline.
   */
  unregister(filterId: string): boolean {
    return this.filters.delete(filterId);
  }

  /**
   * Enable or disable a filter by ID.
   */
  setEnabled(filterId: string, enabled: boolean): boolean {
    const filter = this.filters.get(filterId);
    if (!filter) return false;
    filter.enabled = enabled;
    return true;
  }

  /**
   * Get the list of registered filters, sorted by priority.
   */
  getFilters(): OutputFilterConfig[] {
    return Array.from(this.filters.values())
      .filter((f) => f.enabled)
      .sort((a, b) => a.priority - b.priority);
  }

  /**
   * Apply all enabled filters to a line of terminal output.
   * Returns the filtered output, or null if the line was dropped.
   */
  applyFilters(
    line: string,
    context: Omit<FilterContext, "tags"> & { tags?: string[] }
  ): { output: string; tags: string[] } | null {
    let current: string = line;
    const tags: string[] = [...(context.tags ?? [])];

    const filters = this.getFilters();
    for (const filter of filters) {
      const result = filter.filter(current, { ...context, tags });
      if (result === null) {
        return null; // Line dropped by filter
      }
      if (result !== undefined) {
        current = result;
      }
    }

    return { output: current, tags };
  }

  /**
   * Apply filters to a batch of lines.
   */
  applyToBatch(
    lines: string[],
    contextFactory: (index: number, line: string) => Omit<FilterContext, "tags">
  ): string[] {
    const results: string[] = [];
    let prevLine: string | undefined;
    const tags: string[] = [];

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const ctx = { ...contextFactory(i, line), tags, previousLine: prevLine };
      const result = this.applyFilters(line, ctx);

      if (result !== null) {
        results.push(result.output);
        if (result.tags.length > 0) {
          tags.push(...result.tags);
        }
      }
      prevLine = line;
    }

    return results;
  }

  /**
   * Clear all registered filters.
   */
  clear(): void {
    this.filters.clear();
  }

  /**
   * Register standard filters on an existing pipeline instance.
   * Pre-registers error highlighter, strip-ansi, and line limiter filters.
   */
  static registerStandardFilters(pipeline: OutputFilterPipeline): void {
    OutputFilterPipeline.registerErrorHighlighter(pipeline);
    OutputFilterPipeline.registerAnsiStripFilter(pipeline);
    OutputFilterPipeline.registerLineLimiter(pipeline);
  }

  /**
   * Register the error highlighter filter.
   */
  static registerErrorHighlighter(pipeline: OutputFilterPipeline): void {
    pipeline.register({
      id: "error-highlight",
      name: "Error Highlighter",
      description: "Adds warning prefix to error-like lines",
      enabled: true,
      priority: 0,
      filter: (line, ctx) => {
        const errorPatterns = [
          /\b(ERROR|FATAL|PANIC)\b/,
          /\b(exception|Exception)\b/,
          /\b(failed|failure)\b/,
        ];
        const isError = errorPatterns.some((p) => p.test(line));
        if (isError) {
          ctx.tags.push("error");
          return `⚠️  ${line}`;
        }
        return undefined;
      },
    });
  }

  /**
   * Register the ANSI strip filter.
   */
  static registerAnsiStripFilter(pipeline: OutputFilterPipeline): void {
    pipeline.register({
      id: "strip-ansi",
      name: "Strip ANSI",
      description: "Remove ANSI escape sequences from output",
      enabled: false,
      priority: 10,
      filter: (line) => stripAnsiCodes(line),
    });
  }

  /**
   * Register the line limiter filter.
   */
  static registerLineLimiter(pipeline: OutputFilterPipeline): void {
    pipeline.register({
      id: "line-limiter",
      name: "Line Limiter",
      description: "Prevent excessive output",
      enabled: true,
      priority: 100,
      filter: (line, ctx) => {
        const maxLen = typeof ctx.options?.maxLineLength === "number"
          ? ctx.options.maxLineLength
          : 10000;
        if (line.length > maxLen) {
          return line.slice(0, maxLen) + "…[truncated]";
        }
        return undefined;
      },
    });
  }

  getSessionId(): string {
    return this.sessionId;
  }
}
