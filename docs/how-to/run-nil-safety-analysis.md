# Nil Safety Analysis

Comprehensive nil safety analysis tools to prevent panic-causing nil pointer dereferences.

## Running Analysis

```bash
make nil-safety        # Run all nil safety tools
make nilaway           # Advanced nil flow analysis (Uber NilAway)
go vet -nilness ./...  # Built-in Go nilness analyzer
go-nilcheck ./...      # Function pointer validation
```

## Tool Installation

```bash
make install-tools
# Or manually:
go install go.uber.org/nilaway/cmd/nilaway@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

## Full Static Analysis Toolchain

| Tool | Purpose |
|---|---|
| **NilAway** | Advanced nil pointer safety analysis (Uber) |
| **Staticcheck** | Production-grade static analyzer |
| **golangci-lint** | Meta-linter with multiple analyzers |
| **gosec** | Security-focused static analysis |
| **go vet** | Built-in Go static analysis |

```bash
make analyze      # Run all static analysis tools
make staticcheck  # Advanced static analysis
make security     # Security vulnerability scanning
make deadcode     # Find unreachable code
deadcode -test ./...  # Include test files
```

## Best Practices

1. Always run `make nil-safety` before committing
2. Use NilAway for the most comprehensive nil flow analysis
3. Include nil checks before pointer dereferences
4. Use defensive programming in overlay rendering (see `app/app.go:1225`)
