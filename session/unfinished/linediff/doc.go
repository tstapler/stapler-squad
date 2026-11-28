// Package linediff counts inserted and deleted lines between two file contents
// (hybrid Myers with an LCS fallback and cap) and memoizes the per-file result.
// A bottom leaf: it never imports session/unfinished, which owns the file reads.
package linediff
