// Package repostate holds per-repository derived scanner state.
package repostate

// RepoKey identifies one repository: the canonical (symlink-resolved) common
// .git directory the gogitstore registry keys on. Obtain it from
// gogitstore's CommonDirKey, never by recomputing the path.
type RepoKey string
