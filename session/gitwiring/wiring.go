// Package gitwiring is the composition root for the git backend: the one place that may
// import backend, its implementations and config together (Epic 1.1, plan rule 5).
//
// It is empty until Story 1.1.1 (cohort parsing) and Story 1.1.2 (NewRouter). No package
// under session/git, session/vcs, session/vc or pkg/ may import it (depguard rule
// git_no_gitwiring_import in .golangci.yml); they receive their collaborators by
// constructor injection instead.
package gitwiring
