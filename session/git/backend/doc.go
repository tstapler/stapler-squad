// Package backend is the leaf package for the git backend seam: the Backend interface,
// typed results, RepoLocation, the Runner port, errors, cohort/mode types and the Router
// (Epic 1.1, Stories 1.1.1 and 1.1.2; none of those exist yet).
//
// It is a leaf: standard library, go.opentelemetry.io/otel/**, and the repo leaves log,
// telemetry and session/git/redact only. It must never import session/git, session/tmux,
// session/lifecycle, session/git/native or config; depguard enforces this (.golangci.yml,
// rule git_backend_leaf). go-git types are not exposed in its API.
package backend
