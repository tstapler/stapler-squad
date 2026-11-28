// Package commitwalk walks commit history over go-git: the header-only parents
// reader, the merge-base paint-down, ahead/behind counting and the bounded
// reachable set, with the walk's process-wide counters. A bottom leaf: it never
// imports session/unfinished, which owns repository caching and locking.
package commitwalk
