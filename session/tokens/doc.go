// Package tokens provides JSONL-based token usage parsing and aggregation
// for Claude Code sessions.
//
// Privacy guarantee: ParseResult and all derived types carry only token counts,
// tool names (e.g. "Bash", "mcp__datadog__search_logs"), skill names (short
// /command strings), and aggregated statistics. The actual text of user prompts,
// assistant responses, file contents, or command outputs is NEVER stored.
//
// Architecture:
//   - Parser.ParseFile reads a Claude JSONL transcript file line-by-line using a
//     10MB bufio.Scanner buffer. Each line is a JSON object; malformed lines are
//     skipped without returning an error.
//   - TokenStore caches parsed results keyed by file path, invalidating on modtime
//     change. A background walker pre-parses all JSONL files on startup; fsnotify
//     callbacks keep the cache fresh for active sessions.
//   - PricingTable maps normalized model family names to USD-per-MTok rates and
//     computes estimated cost from a ParseResult.
//   - Associator links a ParseResult to a stapler-squad session by conversation UUID,
//     project path prefix, or timestamp proximity.
package tokens
