# Roll back the go-git fork

stapler-squad consumes `github.com/tstapler/go-git` (a public fork of `go-git/go-git`, branch `ssq/v5`) through
one `replace` directive in the root `go.mod`. Rolling back to stock upstream means deleting that line.
See ADR-002 in `project_plans/go-git-fork-full-git-replacement/decisions/`.

## When to roll back

A fork tag misbehaves (build break, test failure, or a runtime regression traced to a fork patch).

## Steps

1. Delete this line from `go.mod`:

   ```
   replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.N
   ```

2. Pin the upstream release the fork was based on (keep the `require` version equal to the fork's base, here `v5.19.3`):

   ```bash
   go get github.com/go-git/go-git/v5@v5.19.3
   go mod tidy
   go mod verify
   ```

3. Confirm the stock module is in use (no `=>` in the output):

   ```bash
   go list -m github.com/go-git/go-git/v5
   ```

4. Build and test:

   ```bash
   make ci
   ```

Import paths never change (they stay `github.com/go-git/go-git/v5`), so no source edits are needed.

## Fork tags must stay rollbackable

Every fork tag must pass the steps above. A tag that adds an exported symbol the app calls, without a
build-tag fallback to stock go-git, fails the rollback and must not be adopted.

## Verified

Steps 1 to 3 plus `go build ./...` and `go test ./session/git/...` were exercised against `v5.19.3-ssq.1`, which carries no source patches. The full `make ci` run is the acceptance check for each future fork tag. Results are recorded in
`project_plans/go-git-fork-full-git-replacement/implementation/gates.md` (Epic 1.2).
