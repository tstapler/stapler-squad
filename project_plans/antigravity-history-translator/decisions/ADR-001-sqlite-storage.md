# Architecture Decision Record: SQLite for History Storage

## Status
Accepted

## Context
We need a robust way to store, query, and transfer CLI history between different programs and sessions. Traditional plain-text history files (like `.bash_history`) are susceptible to race conditions, parsing errors, data loss upon concurrent writes, and lack structured metadata (like execution duration, directory context, or exit codes).

## Decision
We will use an embedded SQLite database (`github.com/mattn/go-sqlite3`) as the centralized storage backend for the `antigravity-history-translator`. All translated history events will be ingested into this database, and all history queries for agents or session transfers will read from it.

## Consequences
### Positive
- **Concurrency:** SQLite handles concurrent reads and writes safely, preventing data corruption when multiple terminal sessions are active.
- **Structured Data:** We can easily store and query rich metadata (timestamp, command AST, working directory, inhibition status).
- **Performance:** Efficient querying and filtering of history without loading massive flat files into memory.

### Negative
- **Dependency:** Introduces a CGO dependency (`go-sqlite3`), which may complicate cross-compilation slightly compared to pure Go solutions.
- **Migration:** Requires a mechanism to initialize the schema and handle future schema migrations.
