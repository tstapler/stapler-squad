# Path Completion: Pitfalls and Edge Cases

Research date: 2026-04-07
Scope: Go server-side path completion API (macOS/darwin) + React debounced frontend

---

## Summary Table

| # | Pitfall | Risk | Mitigation |
|---|---------|------|------------|
| 1 | Permission errors on unreadable directories | Medium | Return structured error, surface in UI |
| 2 | Symlinks reported as files, not directories | **High** | Stat the target, re-classify as dir if needed |
| 3 | Tilde (`~`) not expanded by Go | **High** | Expand server-side before any path op |
| 4 | Blocking on slow/network filesystems (NFS, iCloud) | **High** | Goroutine + timeout channel (context does NOT work) |
| 5 | React stale results from out-of-order responses | **High** | AbortController + request generation counter |
| 6 | Large directories (node_modules, 10k+ entries) | Medium | Cap at 500 entries, return `truncated: true` |
| 7 | Hidden files appearing unexpectedly | Low | Hide by default; show when query prefix is `.` |
| 8 | Path normalization edge cases | Medium | Normalize on server before every ReadDir call |

---

## Pitfall 1: Permission Errors

**Description:**
A directory may exist and be visible in its parent listing but be unreadable by the process
(e.g., `/private/var/db`, `/root`, system-owned directories). `os.ReadDir` returns a non-nil error.
If this is not handled distinctly, the client has no way to know whether the path is invalid
or simply inaccessible.

**Risk: Medium**

**Recommended mitigation:**
- Detect `errors.Is(err, fs.ErrPermission)` and return ConnectRPC `CodePermissionDenied`
- Do NOT return the raw OS error string to the client
- In the UI, show a non-fatal inline message: "Cannot read directory (permission denied)" while keeping the path field editable

```go
entries, err := os.ReadDir(resolvedPath)
if err != nil {
    if errors.Is(err, fs.ErrPermission) {
        return nil, connect.NewError(connect.CodePermissionDenied,
            fmt.Errorf("directory not readable"))
    }
    if errors.Is(err, fs.ErrNotExist) {
        return nil, connect.NewError(connect.CodeNotFound,
            fmt.Errorf("path does not exist"))
    }
    return nil, connect.NewError(connect.CodeInternal,
        fmt.Errorf("could not list directory"))
}
```

---

## Pitfall 2: Symlinks to Directories

**Description:**
`os.ReadDir` uses `lstat` under the hood. A symlink to a directory will have `DirEntry.Type()`
returning `fs.ModeSymlink`, NOT `fs.ModeDir`. If you use `entry.IsDir()` alone, symlinks to
directories will be classified as files and the user will be unable to navigate into them.

**Risk: High** — Breaks navigation into any symlinked directory (e.g., `/usr/local` → `../Cellar`,
Docker volume mounts, Homebrew prefixes).

**Recommended mitigation:**

```go
func isDirectory(dirPath string, entry fs.DirEntry) bool {
    if entry.IsDir() {
        return true
    }
    // Check if symlink pointing to a directory
    if entry.Type()&fs.ModeSymlink != 0 {
        target := filepath.Join(dirPath, entry.Name())
        info, err := os.Stat(target) // follows symlink
        if err == nil && info.IsDir() {
            return true
        }
    }
    return false
}
```

**Additional consideration:** Circular symlinks (A→B→A) will cause `os.Stat` to fail with
"too many levels of symbolic links". This error should be silently ignored — treat the entry
as a file. Do not recurse into symlinked directories during listing (only the immediate entry
needs classification).

---

## Pitfall 3: Tilde (`~`) Expansion

**Description:**
Go's standard library has no tilde expansion. `os.ReadDir("~/projects")` will look for a
literal directory named `~` in the current working directory and return `ErrNotExist`.
The `~` is a shell convention; neither the OS nor Go's path packages understand it.

**Risk: High** — The single most common path users type starts with `~/`. Without expansion,
the feature is nearly unusable.

**When to expand:** Server-side, before any filesystem operation. The browser has no knowledge
of the server's home directory. Do not expand client-side.

**Recommended mitigation:**

```go
// ExpandTilde replaces a leading ~ with the current user's home directory.
// Only expands ~/ and ~ alone; does not expand ~username.
func ExpandTilde(path string) (string, error) {
    if path == "~" {
        return os.UserHomeDir()
    }
    if strings.HasPrefix(path, "~/") {
        home, err := os.UserHomeDir()
        if err != nil {
            return "", fmt.Errorf("cannot determine home directory: %w", err)
        }
        return filepath.Join(home, path[2:]), nil
    }
    return path, nil
}
```

**Client-side display:** The React UI should display `~` in the input as the user typed it.
The API response should include the expanded absolute path for subsequent calls.

**Edge cases:**
- `~username` expansion — do not support; return an error or treat as literal
- `os.UserHomeDir()` can fail in sandboxed/container environments — handle the error, do not panic

---

## Pitfall 4: Blocking on Slow/Network Filesystems

**Description:**
NFS mounts, iCloud Drive (`~/Library/Mobile Documents/`), Dropbox, network drives, and Docker
volume mounts can make `os.ReadDir` block for seconds or indefinitely.

**Critical:** Go contexts do **not** cancel blocking syscalls. A goroutine blocked in
`opendir(2)` or `getdents(2)` will remain blocked even if the context is cancelled.
This means `context.WithTimeout` does NOT unblock `os.ReadDir`. This is a known, documented
limitation of Go filesystem operations.

**Risk: High** — A single request to `/Volumes/NFS-mount/` could block a handler goroutine
for 60 seconds (NFS default `timeo=600`), exhausting the server under load.

**Recommended mitigation:** Run `os.ReadDir` in a separate goroutine with a result channel,
and select on the context or a dedicated timeout.

```go
const readDirTimeout = 3 * time.Second

type readDirResult struct {
    entries []fs.DirEntry
    err     error
}

func readDirWithTimeout(ctx context.Context, path string) ([]fs.DirEntry, error) {
    ch := make(chan readDirResult, 1) // buffered: goroutine won't block on send
    go func() {
        entries, err := os.ReadDir(path)
        ch <- readDirResult{entries, err}
    }()

    select {
    case result := <-ch:
        return result.entries, result.err
    case <-time.After(readDirTimeout):
        // Goroutine is leaked until the OS unblocks it (acceptable tradeoff)
        return nil, fmt.Errorf("directory listing timed out after %v", readDirTimeout)
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

**The goroutine leak:** The spawned goroutine will block until the OS gives up. This is a
known, accepted limitation. The buffered channel (`make(chan ..., 1)`) ensures the goroutine
can eventually write and exit without blocking on the send, preventing a second resource leak.

**Known slow paths on macOS:**
- `~/Library/Mobile Documents/` — iCloud Drive, may stall on metadata sync
- `/Volumes/*` — any network or external mount
- Paths inside Docker containers via FUSE drivers

---

## Pitfall 5: React Race Conditions (Stale Results)

**Description:**
With a debounced input, two requests can be in-flight simultaneously. If the user types
`~/pro`, then quickly types `~/projects/`, the first request (for `~/pro`) may complete after
the second (for `~/projects/`). Without cancellation, the stale results from the first request
overwrite the current results.

**Risk: High** — Users see flickering or incorrect completions.

**Recommended mitigation — AbortController pattern:**

```typescript
const abortControllerRef = useRef<AbortController | null>(null);

const fetchCompletions = useCallback(async (path: string) => {
  // Cancel any in-flight request
  if (abortControllerRef.current) {
    abortControllerRef.current.abort();
  }
  abortControllerRef.current = new AbortController();
  const signal = abortControllerRef.current.signal;

  try {
    const results = await listDirectory(path, { signal });
    if (!signal.aborted) {
      setCompletions(results);
    }
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      return; // Expected — ignore
    }
    setError(err);
  }
}, []);
```

**Cleanup on unmount:**

```typescript
useEffect(() => {
  return () => {
    abortControllerRef.current?.abort();
  };
}, []);
```

**Additional defense — generation counter:**

```typescript
const generationRef = useRef(0);

const fetchCompletions = useCallback(async (path: string) => {
  const generation = ++generationRef.current;
  const results = await listDirectory(path);
  if (generation !== generationRef.current) {
    return; // A newer request superseded this one
  }
  setCompletions(results);
}, []);
```

---

## Pitfall 6: Large Directories (10,000+ Entries)

**Description:**
`os.ReadDir` reads **and sorts** all entries before returning. A directory with 10,000 entries
(e.g., `node_modules`, a large media archive, or a flat migration directory) will:
1. Allocate ~10,000 `DirEntry` structs
2. Sort them all (O(n log n))
3. Serialize and transmit them all to the client

At 10,000 entries, sorting is fast (milliseconds), but the completion dropdown becomes unusable.

**Risk: Medium** — Performance degrades predictably; no crash, but UX is poor.

**Recommended mitigation:** Filter first (by prefix match), then cap results:

```go
const maxCompletionResults = 500

func filterAndLimit(entries []fs.DirEntry, prefix string, showHidden bool) []fs.DirEntry {
    var matched []fs.DirEntry
    for _, e := range entries {
        name := e.Name()
        if !showHidden && strings.HasPrefix(name, ".") {
            continue
        }
        if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
            continue
        }
        matched = append(matched, e)
        if len(matched) >= maxCompletionResults {
            break
        }
    }
    return matched
}
```

Return a `truncated: bool` field in the response so the UI can show "Showing first 500 results — type more to narrow down".

**Alternative for extreme cases:** Use `os.Open` + `(*os.File).ReadDir(n)` to read only N entries
at a time without sorting the entire directory. Only needed for directories with 100k+ entries.

```go
f, err := os.Open(path)
if err != nil { ... }
defer f.Close()
entries, err := f.ReadDir(1000) // Read first 1000 unsorted entries
```

---

## Pitfall 7: Hidden Files and Directories

**Description:**
Files and directories starting with `.` are conventionally hidden on macOS/Unix. Users generally
do not want to see `.git`, `.DS_Store`, `.env`, etc. in path completions unless they explicitly
type a leading dot.

**Risk: Low** — Primarily a UX concern.

**Recommended behavior (matches shell completion convention):**
- By default, exclude entries beginning with `.`
- If the partial name the user has typed after the last `/` starts with `.`, include hidden entries

```go
func shouldShowEntry(name string, typedPrefix string) bool {
    isHidden := strings.HasPrefix(name, ".")
    userIsTypingHidden := strings.HasPrefix(typedPrefix, ".")
    if isHidden && !userIsTypingHidden {
        return false
    }
    return true
}
```

**Special macOS hidden entries to always exclude:**
- `.DS_Store` — never useful in a path picker
- `.localized` — macOS locale marker
- `.Spotlight-V100`, `.fseventsd`, `.Trashes` — volume metadata

---

## Pitfall 8: Path Normalization Edge Cases

**Description:**
Users (and auto-complete logic) can produce malformed paths that cause unexpected behavior if
not normalized before use.

**Risk: Medium** — Some edge cases cause `ErrNotExist` silently; others may be security-relevant
(path traversal).

**Edge cases and behavior without normalization:**

| Input | Problem | Normalized form |
|-------|---------|----------------|
| `/foo//bar` | Double-slash may confuse prefix extraction | `/foo/bar` |
| `/foo/bar/` | Trailing slash may confuse prefix-matching logic | `/foo/bar` |
| `/foo/./bar` | Prefix extraction is wrong | `/foo/bar` |
| `/foo/../bar` | Resolves to `/bar` — prefix extraction gives wrong parent | `/bar` |
| `` (empty string) | `os.ReadDir("")` reads the current working directory | Return error |
| `/` | Valid root — must not strip the trailing slash | `/` |

**Recommended server-side normalization sequence:**

```go
func normalizePath(raw string) (dir string, prefix string, err error) {
    if raw == "" {
        return "", "", fmt.Errorf("empty path")
    }

    // 1. Expand tilde
    expanded, err := ExpandTilde(raw)
    if err != nil {
        return "", "", err
    }

    // 2. Clean (resolves .., ., double slashes)
    cleaned := filepath.Clean(expanded)

    // 3. Determine if the user is mid-entry or at a directory boundary
    //    If raw ends with /, list the directory itself
    //    Otherwise, split into parent dir + partial name prefix
    if strings.HasSuffix(raw, "/") || raw == "~" {
        return cleaned, "", nil
    }

    // Split into parent and the partial name being typed
    dir = filepath.Dir(cleaned)
    prefix = filepath.Base(cleaned)
    return dir, prefix, nil
}
```

**Security note:** `filepath.Clean` resolves `..` components, which prevents naive directory
traversal. For a local tool running as the same user, there is no additional security boundary
needed, but document this assumption.

**Trailing slash significance (the most subtle edge case):**
- No trailing slash: list the parent dir and filter by prefix (the typed name)
- Trailing slash: list the directory itself with no prefix filter

---

## Additional macOS-Specific Notes

**iCloud Drive placeholder files:**
Files in `~/Library/Mobile Documents/` may be iCloud placeholders (not yet downloaded).
These appear in `os.ReadDir` results but accessing their content may trigger a download and
cause I/O delays. The timeout mitigation in Pitfall 4 covers this.

**System Integrity Protection (SIP) paths:**
Paths like `/System/Library/`, `/usr/bin/` are protected even for root. `os.ReadDir` will
return `ErrPermission`. Handle the same as Pitfall 1.

**Case sensitivity:**
macOS HFS+/APFS is case-insensitive by default. Prefix matching should be case-insensitive
on macOS. Use `strings.ToLower` for comparison but preserve original casing in results.

---

## Recommended Implementation Order

1. Tilde expansion (Pitfall 3) — required for basic usability
2. Path normalization (Pitfall 8) — required for correctness
3. Symlink classification (Pitfall 2) — required for directory navigation
4. Permission/not-found error mapping (Pitfall 1) — required for good error UX
5. Result capping at 500 (Pitfall 6) — required for performance
6. Hidden file filtering (Pitfall 7) — expected UX behavior
7. React AbortController (Pitfall 5) — required for correctness under fast typing
8. ReadDir timeout (Pitfall 4) — required for robustness on network filesystems
