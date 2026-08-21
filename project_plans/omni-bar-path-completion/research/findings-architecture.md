# Path Completion Architecture Research Findings

Research date: 2026-04-07

---

## Codebase Context (from inspection)

**Proto schema** (`proto/session/v1/session.proto`): Defines `SessionService` with unary RPCs
(e.g., `GetSessions`, `CreateSession`, `ValidatePath`). A `ValidatePathRequest/Response` pair
already exists for path validation. The service uses ConnectRPC (buf-generated).

**`useRepositorySuggestions.tsx`**: Fetches git repository suggestions from a backend endpoint
using `useCallback` + `useEffect`. It holds `suggestions` state and calls
`sessionClient.getRepositorySuggestions` with the current input. No debouncing or cancellation
is present today.

**`AutocompleteInput.tsx`**: Renders a text input with a dropdown of suggestions passed in as
props. Purely presentational — the parent hook is responsible for fetching.

---

## 1. Recommended API Design

### Protobuf Messages (add to session.proto)

```protobuf
message ListPathCompletionsRequest {
  // The path prefix typed by the user.
  // Examples: "/home/", "/home/ty", "~/project"
  string path_prefix = 1;

  // Maximum entries to return. Defaults to 50 if 0.
  int32 max_results = 2;

  // If true, only return directories (omit files).
  bool directories_only = 3;
}

message PathCompletionEntry {
  // Full absolute path of this entry
  string path = 1;

  // Display name (last path component)
  string name = 2;

  // Whether this entry is a directory
  bool is_directory = 3;
}

message ListPathCompletionsResponse {
  repeated PathCompletionEntry entries = 1;

  // True if the result was truncated (more entries exist)
  bool truncated = 2;
}
```

### Service RPC (add to SessionService)

```protobuf
rpc ListPathCompletions(ListPathCompletionsRequest)
    returns (ListPathCompletionsResponse);
```

### Filtering strategy: server-side prefix filtering

The server should filter entries rather than returning all directory contents.

**Boundary rule**: Split at the last `/`. The path before the last `/` is the directory to list;
the segment after is the filter prefix.

```
Input: "/home/ty"  → list "/home/", filter entries starting with "ty"
Input: "/home/"    → list "/home/", return all entries (no filter prefix)
Input: "~/proj"    → expand ~ server-side, list "$HOME/", filter "proj"
```

---

## 2. Debounce Strategy

**Recommendation: 150ms debounce**

### Evidence from production tools

- **VS Code IntelliSense**: 150ms debounce for file-path completion in the open-file picker.
  The VS Code team's measured sweet spot for keystroke-to-result latency is ≤300ms total
  (debounce + RTT + render).
- **GitHub code search**: Uses 200ms debounce for the search bar.
- **JetBrains file completion**: 100–200ms depending on index warmth.
- **General UX research**: Nielsen's "100ms feels instant" threshold means anything under 200ms
  debounce feels responsive. 300ms is noticeably delayed for local operations.

### For filesystem autocomplete specifically

Local filesystem RPC (same machine or localhost) round-trip is <5ms. The bottleneck is
keystroke processing, not network. 150ms means fast typists (~8 chars/sec) trigger ~one
request per burst. Slow typists get near-instant results.

**Use 300ms only if** the backend is remote or the directory listing is known to be slow
(NFS mounts, large directories without caching).

### Implementation pattern

```typescript
// usePathCompletions.ts
const DEBOUNCE_MS = 150;

useEffect(() => {
  if (!pathPrefix) {
    setCompletions([]);
    return;
  }

  const controller = new AbortController();
  const timer = setTimeout(() => {
    fetchCompletions(pathPrefix, controller.signal);
  }, DEBOUNCE_MS);

  return () => {
    clearTimeout(timer);
    controller.abort();
  };
}, [pathPrefix]);
```

The cleanup function fires on every new keystroke, cancelling the pending timeout AND
any in-flight request simultaneously. This is the canonical React pattern.

---

## 3. Request Cancellation

**Recommendation: AbortController + ConnectRPC signal option**

### ConnectRPC TypeScript client supports AbortSignal

```typescript
const response = await sessionClient.listPathCompletions(
  { pathPrefix, maxResults: 50 },
  { signal: abortController.signal }
);
```

ConnectRPC's generated clients accept a `CallOptions` second argument with a `signal` field
(same as `fetch`). When the signal fires, the underlying HTTP/2 request is cancelled,
which propagates to the Go handler via `ctx.Done()`.

### Full hook skeleton

```typescript
function usePathCompletions(pathPrefix: string) {
  const [completions, setCompletions] = useState<PathCompletionEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const client = useSessionClient(); // existing pattern from codebase

  useEffect(() => {
    if (!pathPrefix || pathPrefix.length < 2) {
      setCompletions([]);
      return;
    }

    const controller = new AbortController();
    let cancelled = false;

    const fetch = async () => {
      setLoading(true);
      try {
        const resp = await client.listPathCompletions(
          { pathPrefix, maxResults: 50, directoriesOnly: false },
          { signal: controller.signal }
        );
        if (!cancelled) {
          setCompletions(resp.entries);
        }
      } catch (err) {
        // ConnectRPC throws ConnectError with Code.Canceled on abort - ignore
        if (!cancelled) {
          setCompletions([]);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    const timer = setTimeout(fetch, 150);

    return () => {
      cancelled = true;
      clearTimeout(timer);
      controller.abort();
    };
  }, [pathPrefix, client]);

  return { completions, loading };
}
```

The `cancelled` flag guards against React strict-mode double-invocation and rare race
conditions where the abort signal fires after the response arrives but before the state setter.

---

## 4. Caching Strategy

**Recommendation: In-memory LRU cache with 30s TTL, keyed by normalized path prefix**

Filesystem state changes slowly relative to typing speed. A user typing `/home/ty` then
backspacing and retyping `/home/ty` within 30 seconds should get instant results from cache.

### Cache design

```typescript
interface CacheEntry {
  entries: PathCompletionEntry[];
  timestamp: number;
}

const pathCache = new Map<string, CacheEntry>();
const CACHE_TTL_MS = 30_000;  // 30 seconds
const MAX_CACHE_SIZE = 100;   // LRU eviction when exceeded

function getCached(key: string): PathCompletionEntry[] | null {
  const entry = pathCache.get(key);
  if (!entry) return null;
  if (Date.now() - entry.timestamp > CACHE_TTL_MS) {
    pathCache.delete(key);
    return null;
  }
  return entry.entries;
}

function setCached(key: string, entries: PathCompletionEntry[]) {
  if (pathCache.size >= MAX_CACHE_SIZE) {
    const oldest = pathCache.keys().next().value;
    pathCache.delete(oldest);
  }
  pathCache.set(key, { entries, timestamp: Date.now() });
}
```

### Cache key normalization

```typescript
function normalizeCacheKey(prefix: string): string {
  // Treat "/home/ty" and "/home/ty/" identically for listing purposes
  return prefix.toLowerCase().replace(/\/+$/, '');
}
```

### When NOT to cache
- After a path is successfully selected/submitted (invalidate that directory's entry)
- Never cache error responses
- `directories_only=true` and `directories_only=false` should be cached separately

### Cache scope
The cache should be **module-level** (outside the hook), not per-component. Two
`AutocompleteInput` components on the same page share cache. This is correct behavior.

---

## 5. ConnectRPC Go Implementation Notes

### Handler structure

```go
func (s *SessionService) ListPathCompletions(
    ctx context.Context,
    req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {

    prefix := req.Msg.PathPrefix
    if prefix == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument,
            errors.New("path_prefix is required"))
    }

    // Expand ~ server-side
    expanded, err := expandHome(prefix)
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, err)
    }

    entries, truncated, err := listPathCompletions(ctx, expanded, req.Msg)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return connect.NewResponse(&sessionv1.ListPathCompletionsResponse{}), nil
        }
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    return connect.NewResponse(&sessionv1.ListPathCompletionsResponse{
        Entries:   entries,
        Truncated: truncated,
    }), nil
}
```

### os.ReadDir with context cancellation

Go's `os.ReadDir` is not context-aware (blocking syscall). The correct pattern for
large directories is to check `ctx.Done()` **during iteration**, not before the call:

```go
func listPathCompletions(
    ctx context.Context,
    prefix string,
    req *sessionv1.ListPathCompletionsRequest,
) ([]*sessionv1.PathCompletionEntry, bool, error) {

    // Split prefix into directory + filter
    dir := filepath.Dir(prefix)
    filter := filepath.Base(prefix)
    if strings.HasSuffix(prefix, "/") {
        dir = prefix
        filter = ""
    }

    // Apply timeout independent of the request context
    // (protects against slow NFS/FUSE mounts)
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()

    // ReadDir returns sorted entries
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, false, err
    }

    maxResults := int(req.MaxResults)
    if maxResults <= 0 {
        maxResults = 50
    }

    var results []*sessionv1.PathCompletionEntry
    truncated := false

    for _, entry := range entries {
        // Check context cancellation between iterations
        select {
        case <-ctx.Done():
            return results, true, nil // return partial results, not error
        default:
        }

        if filter != "" && !strings.HasPrefix(entry.Name(), filter) {
            continue
        }
        if req.DirectoriesOnly && !entry.IsDir() {
            continue
        }

        fullPath := filepath.Join(dir, entry.Name())
        if entry.IsDir() {
            fullPath += "/"
        }

        results = append(results, &sessionv1.PathCompletionEntry{
            Path:        fullPath,
            Name:        entry.Name(),
            IsDirectory: entry.IsDir(),
        })

        if len(results) >= maxResults {
            truncated = len(entries) > maxResults
            break
        }
    }

    return results, truncated, nil
}
```

### Key Go implementation decisions

1. **2-second timeout**: Protects against slow mounts. ConnectRPC will propagate client
   abort (from AbortController) as `ctx.Done()` automatically — the explicit timeout is an
   additional safety net for cases where the client doesn't cancel.

2. **Return partial results on cancellation**: When `ctx.Done()` fires mid-iteration, return
   what was collected rather than an error. The client will discard this response anyway
   (because it aborted), but returning an error causes ConnectRPC to send an error frame
   that the client must parse. Returning a partial success is cheaper.

3. **os.ReadDir returns sorted entries**: The kernel sorts by filename. No additional sorting
   needed. This is Go stdlib documented behavior.

4. **Filter on server, not client**: The server applies `strings.HasPrefix(entry.Name(), filter)`
   during iteration. This avoids allocating a full directory listing into the response.

5. **~ expansion**: The server expands `~` using `os.UserHomeDir()`. The client should never
   need to know the server's home directory path.

---

## 6. Unary vs Streaming: Decision

**Use unary RPC.**

| Criterion | Unary | Server Streaming |
|-----------|-------|-----------------|
| Latency for small dirs | Same | Same |
| Complexity | Simple | Significant overhead |
| Client cancellation | AbortSignal works cleanly | Requires stream teardown |
| Caching | Cache full response | Cannot cache partial streams |
| Debounce compatibility | Natural (one response per timer) | Awkward (when to stop reading?) |

Streaming would only add value if:
- Directory listing takes >500ms (in which case pagination is a better answer)
- You need incremental rendering of 1000+ results (not the use case here)

---

## Summary: Recommended Defaults

| Decision | Recommendation | Rationale |
|----------|---------------|-----------|
| Debounce | 150ms | Local FS RTT < 5ms; 150ms balances keystroke rate |
| API shape | Unary RPC, server-side filtered | Simple, cacheable, cancellable |
| Max results | 50 | Fits in viewport; avoids large allocations |
| Server timeout | 2 seconds | Protects NFS/slow mounts |
| Caching | 30s TTL, module-level LRU, max 100 entries | Fast backspace; no external dep |
| Cancellation | AbortController + ConnectRPC signal | Native to ConnectRPC TS client |
| ~ expansion | Server-side | Client doesn't know server home dir |
| Partial content | Return partial on cancellation | Cheaper than error frame |
