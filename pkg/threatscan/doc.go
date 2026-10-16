// Package threatscan detects secrets and prompt-injection-style threats in text.
//
// Scan operates on raw content: callers that HTML-strip or truncate before scanning
// lose coverage for patterns such as html_hidden_display_none.
//
// Nothing returned by this package (ThreatMatch fields, errors, logs) ever contains the
// matched substring; only pattern IDs identify a hit.
//
// ScopeStrict (secrets) is live in the review gate's pre-check. The ScopeContext
// pattern categories (prompt injection, hidden HTML, role-play hijack, exfiltration)
// are dormant data: no production caller scans with ScopeContext or ScopeAll yet.
package threatscan
