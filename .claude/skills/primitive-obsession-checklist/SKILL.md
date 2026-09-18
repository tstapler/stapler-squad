---
name: primitive-obsession-checklist
description: Use when adding a parameter to an existing Go function or writing a new function signature in stapler-squad, to catch same-typed parameter piles (two or more parameters of the same primitive type representing distinct domain concepts) before they merge — the exact failure mode newtypes like RepoRef/AccountRef exist to prevent.
---

# Primitive Obsession — Catch Same-Typed Parameter Piles Before They Merge

This repo is written almost entirely with Claude Code. LLMs default to long parameter lists
of interchangeable primitives (`func f(host, owner, repo, username string)`) instead of
bundling related values into a named type. Review every new or migrated function signature
against this checklist before merging.

## When to Use This Skill

- Before adding a parameter to an existing Go function
- Before writing a new Go function whose signature would take two or more parameters of the
  same primitive type
- Reviewing a PR or diff that touches a function signature with multiple `string`/`int`/
  `float64` parameters

## The Smell

A function takes **two or more parameters of the same underlying type** (`string`, `int`,
`float64`) that represent distinct domain concepts and could be silently swapped at a call
site with no compiler error.

**Wrong:**
```go
func GetIssue(ctx context.Context, host, username, owner, repo string, number int) (*Issue, error)
// GetIssue(ctx, owner, repo, host, username, number) compiles. Nothing catches the swap.
```

**Right:**
```go
func GetIssue(ctx context.Context, account AccountRef, repo RepoRef, number int) (*Issue, error)
// AccountRef{Host, Username} and RepoRef{owner, repo} — distinct types, can't be swapped
```

## The Precedent in This Repo

`github/repo_ref.go`'s `RepoRef` (unexported `owner`/`repo` fields, constructed only via
`NewRepoRef(owner, repo string) (RepoRef, error)` which validates both are non-empty) and
`github/keychain.go`'s `AccountRef{Username, Host string}` are the canonical newtypes for
this package. A 2026-08 migration found a duplicate, inferior plain-struct `RepoRef`
(exported fields, no validation) had been reintroduced in `github/repos.go`, colliding at
compile time with the original. Every call site passing raw `owner, repo string` pairs (or
building the exported-field struct literal) was converted to `gh.NewRepoRef(owner, repo)`.

## How to Apply

- Before adding a parameter to an existing function, or writing a new one: if it would be
  the **second or later parameter of the same primitive type**, stop and check whether a
  newtype/value object already exists in the package for that concept (`RepoRef`,
  `AccountRef`) before reaching for another bare `string`/`int`.
- If no such type exists yet and the concept is used in more than one function signature,
  introduce a smart constructor (unexported fields + validating `NewXxx` factory) — see the
  `type-driven-design` skill, and the Go-specific version in the `golang-development` skill's
  "Type-Driven Design (Avoiding Primitive Obsession)" section, for the full technique set
  (newtypes, phantom types, sum types, value objects).
- When migrating a function's call sites to a new type, grep for **every** struct-literal
  and field-access call site (`RepoRef{Owner: ...}`, `repo.Owner` used as a field) — a
  partial migration that leaves both the old shape and new type in play is what caused the
  compile-time collision this rule documents. `go build ./...` will not catch a stale call
  site that still compiles against a *different*, undeleted duplicate type — only removing
  the duplicate does.

## Why

Same-typed parameter lists let a caller swap two arguments and get a value that compiles but
is silently wrong at runtime — the exact failure mode `AccountRef`/`RepoRef` exist to
prevent. This is the same discipline as the `interface-pollution-checklist` skill (catch a
recurring LLM-shaped code smell with a named, checked-in checklist) applied to function
signatures instead of interfaces.
