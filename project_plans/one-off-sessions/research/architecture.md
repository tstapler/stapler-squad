# Research: Architecture — Name Generation Strategy

## Summary

No name-generation packages exist in the current codebase or `go.mod`. The app uses `github.com/google/uuid` for session UUIDs. For one-off names, the recommended approach is to embed curated word lists directly in a Go source file (no external package needed), use `math/rand` from Go's stdlib for selection, and `time.Now().Format("20060102")` for the date prefix.

---

## 1. Current ID/Name Generation in the Codebase

### UUID usage
- `github.com/google/uuid` is already a dependency (`go.mod` line ~19).
- `session/instance.go:640`: `UUID: uuid.New().String()` — generates UUID v4 for session identity.
- `session/checkpoint.go:11`: `return uuid.New().String()` — checkpoint IDs.

### No human-readable name generation exists
Grepping for `petname`, `moby`, `namesgenerator`, `adjective`, `wordlist`, `random.*name` across `go.mod` and all Go source files returns zero results. The codebase has never needed generated names before — sessions are always named by the user.

### No `math/rand` or `crypto/rand` in session/server code
The only `rand` usage is in auth (`server/auth/session.go`, `server/tls.go`), event subscriber, and push service — none relevant to name generation.

---

## 2. Options for Name Generation

### Option A: Embed word lists in Go source (Recommended)

Create a new file `session/namegen/namegen.go` with:

```go
package namegen

import (
    "fmt"
    "math/rand"
    "time"
)

var adjectives = []string{
    "agile", "amber", "ancient", "azure", "bold", "brave", "bright", "calm",
    "clever", "coastal", "cosmic", "crisp", "daring", "dawn", "deep", "distant",
    "eager", "early", "elegant", "emerald", "fierce", "fleet", "flowing", "frosty",
    "gentle", "gilded", "glowing", "golden", "grand", "green", "hidden", "hollow",
    "humble", "icy", "jade", "jolly", "keen", "kind", "lively", "lunar",
    "mellow", "misty", "mossy", "mystic", "noble", "northern", "ocean", "open",
    "patient", "peaceful", "polar", "proud", "quiet", "rapid", "rising", "rocky",
    "royal", "rustic", "serene", "sharp", "silver", "sleek", "solar", "solid",
    "steady", "stellar", "still", "sunny", "swift", "tidal", "twilight", "vast",
    "vibrant", "vivid", "warm", "western", "wild", "windy", "wise", "witty",
}

var nouns = []string{
    "albatross", "badger", "bear", "beaver", "bison", "buck", "buffalo", "cardinal",
    "cedar", "cliff", "cloud", "condor", "coral", "crane", "creek", "crow",
    "dune", "eagle", "elm", "falcon", "fern", "finch", "fjord", "fox",
    "glacier", "glen", "grove", "gull", "harbor", "hawk", "heath", "heron",
    "hill", "ibis", "jay", "juniper", "kelp", "kite", "lagoon", "lark",
    "loon", "lynx", "maple", "marsh", "meadow", "mesa", "mink", "moose",
    "moss", "oak", "osprey", "otter", "owl", "peak", "pine", "plover",
    "pond", "raven", "reef", "ridge", "robin", "rock", "salmon", "sedge",
    "shore", "sparrow", "spruce", "starling", "stone", "storm", "swallow", "swan",
    "swift", "teal", "tern", "thistle", "thrush", "tide", "trail", "vale",
}

// Generate returns a name in YYYYMMDD-adjective-noun-NN format.
// Uses the current date in local time.
func Generate() string {
    date := time.Now().Format("20060102")
    adj := adjectives[rand.Intn(len(adjectives))]
    noun := nouns[rand.Intn(len(nouns))]
    num := rand.Intn(100)
    return fmt.Sprintf("%s-%s-%s-%02d", date, adj, noun, num)
}
```

**Pros**:
- Zero new dependencies.
- Binary stays self-contained (requirement: "word list embedded in binary").
- Easy to curate and review (50+ adjectives, 50+ nouns — well above minimum).
- O(1) per call (pure memory, no disk/network).
- All names are lowercase letters + hyphens + digits: URL-safe, shell-safe.

**Cons**:
- Word list maintenance is manual.
- Need to verify ≥ 50 adjectives and ≥ 50 nouns (list above has 80 of each).

### Option B: Third-party package (`petname`)

`github.com/dustinkirkland/golang-petname` provides `petname.Generate(words, separator)` with adjective-noun-name patterns. Well-maintained, used by Docker container name generation.

**Cons**: Adds a new dependency; name format (adjective-noun) doesn't include the numeric suffix and date natively; would need wrapping anyway.

### Option C: Docker's `namesgenerator`

`github.com/moby/moby/pkg/namesgenerator` — the classic Docker random name package. Internal to the moby project; importing it brings transitive dependencies.

**Verdict**: Not worth it for this use case.

---

## 3. Collision Handling

The requirements call for max 10 attempts with error after that. Using `math/rand.Intn(100)` gives 100 possible suffixes per adjective-noun combination. With 80 adjectives × 80 nouns × 100 numbers = 640,000 possible names per date. Collision probability for the first attempt is negligible under normal usage.

Implementation:
```go
func GenerateUnique(baseDir string, maxAttempts int) (string, error) {
    for i := 0; i < maxAttempts; i++ {
        name := Generate()
        fullPath := filepath.Join(baseDir, name)
        if _, err := os.Stat(fullPath); os.IsNotExist(err) {
            return fullPath, nil
        }
    }
    return "", fmt.Errorf("failed to generate unique directory name after %d attempts", maxAttempts)
}
```

---

## 4. Name Length Validation

Requirements say names must be ≤ 32 chars (to stay within title display limits — though the directory name has no such limit).

The directory name format is: `YYYYMMDD-adjective-noun-NN`
= 8 (date) + 1 (-) + len(adj) + 1 (-) + len(noun) + 1 (-) + 2 (number) = 13 + len(adj) + len(noun)

With the word lists above, max adjective length is "twilight" (8 chars), max noun length is "albatross" (9 chars):
- Max name length = 13 + 8 + 9 = 30 chars ✓ (under 32)

Average case is much shorter. All words should be checked against a 32-char total limit during word list curation.

Note: The requirements clarify "Word lists must produce names ≤ 32 characters total" — this refers to the **directory name**, not the session title. The session title is user-provided and governed by `MaxTitleLength = 32` independently.

---

## 5. `math/rand` vs `crypto/rand`

For directory name generation, `math/rand` is sufficient:
- Names are not security-sensitive.
- O(1) performance requirement is met.
- Go 1.20+ seeds `math/rand` automatically (global source is auto-seeded).

Since the `go.mod` declares `go 1.25.0`, the global `rand` functions are safe without explicit seeding.

---

## 6. Recommended Package Location

New file: `session/namegen/namegen.go`

This keeps name generation logic isolated, testable, and importable by `server/services/session_service.go` without circular dependencies.

Alternatively: `session/oneoff/namegen.go` if other one-off-specific logic lives alongside it.

---

## 7. Testing Strategy

```go
func TestGenerate_Format(t *testing.T) {
    name := Generate()
    // Must match: YYYYMMDD-word-word-NN
    re := regexp.MustCompile(`^\d{8}-[a-z]+-[a-z]+-\d{2}$`)
    assert.Regexp(t, re, name)
    assert.LessOrEqual(t, len(name), 32)
}

func TestGenerate_ShellSafe(t *testing.T) {
    for i := 0; i < 1000; i++ {
        name := Generate()
        assert.Regexp(t, `^[a-z0-9-]+$`, name)
    }
}
```
