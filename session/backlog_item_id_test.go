package session

import (
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestBacklogItemID_should_RoundTripWithBlPrefix_When_NewGeneratedAndParsed(t *testing.T) {
	id, err := NewBacklogItemID()
	if err != nil {
		t.Fatalf("NewBacklogItemID: %v", err)
	}

	s := id.String()
	if !strings.HasPrefix(s, BacklogItemIDPrefix) {
		t.Fatalf("String() = %q, want prefix %q", s, BacklogItemIDPrefix)
	}

	parsed, err := ParseBacklogItemID(s)
	if err != nil {
		t.Fatalf("ParseBacklogItemID(%q): %v", s, err)
	}
	if parsed.String() != s {
		t.Fatalf("round trip mismatch: got %q, want %q", parsed.String(), s)
	}
	if !parsed.IsValid() {
		t.Fatalf("parsed id should be valid")
	}
}

func TestParseBacklogItemID_should_ReturnDescriptiveError_When_InputMalformed(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing prefix", "01HXYZABCDEFGHJKMNPQRSTVWX"},
		{"empty string", ""},
		{"wrong prefix", "uuid_01HXYZABCDEFGHJKMNPQRSTVWX"},
		{"prefix but garbage ulid", "bl_not-a-valid-ulid"},
		{"prefix but truncated ulid", "bl_01HXYZ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ParseBacklogItemID(tt.input)
			if err == nil {
				t.Fatalf("ParseBacklogItemID(%q) = %v, nil; want error", tt.input, id)
			}
			if !strings.Contains(err.Error(), tt.input) {
				t.Errorf("error %q does not mention the offending input %q", err.Error(), tt.input)
			}
			if id.IsValid() {
				t.Errorf("ParseBacklogItemID(%q) returned a valid id alongside an error", tt.input)
			}
		})
	}
}

// TestNewBacklogItemID_should_GenerateUniqueMonotonicIDs_When_CalledConcurrently
// exercises the shared ulid.Monotonic entropy source under real goroutine
// concurrency (not just same-millisecond single-threaded calls). This is the
// pattern oklog/ulid/v2 explicitly documents as unsafe without external
// synchronization, so this test is the regression guard for the
// backlogItemIDMu locking in NewBacklogItemID: without it, this test flakes
// with duplicate IDs or a data race under `go test -race`.
func TestNewBacklogItemID_should_GenerateUniqueMonotonicIDs_When_CalledConcurrently(t *testing.T) {
	const goroutines = 50
	const perGoroutine = 50

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []BacklogItemID
		errs    []error
	)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]BacklogItemID, 0, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				id, err := NewBacklogItemID()
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					return
				}
				local = append(local, id)
			}
			mu.Lock()
			results = append(results, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, err := range errs {
		t.Fatalf("NewBacklogItemID returned an error under concurrent load: %v", err)
	}

	want := goroutines * perGoroutine
	if len(results) != want {
		t.Fatalf("got %d generated ids, want %d", len(results), want)
	}

	seen := make(map[string]struct{}, want)
	for _, id := range results {
		s := id.String()
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate BacklogItemID generated under concurrency: %s", s)
		}
		seen[s] = struct{}{}
	}

	// Monotonic ordering: sorting the generated IDs by their string form
	// (lexicographic == chronological for ULIDs) must reproduce a strictly
	// increasing sequence with no equal adjacent elements, i.e. no two
	// concurrent callers were handed the same or a non-monotonic value by
	// the shared entropy source.
	strs := make([]string, len(results))
	for i, id := range results {
		strs[i] = id.String()
	}
	sorted := append([]string(nil), strs...)
	sort.Strings(sorted)
	for i := 1; i < len(sorted); i++ {
		if sorted[i] <= sorted[i-1] {
			t.Fatalf("non-strictly-increasing ids after sort at index %d: %q <= %q", i, sorted[i], sorted[i-1])
		}
	}
}

func TestBacklogItemData_PublicID_should_ReturnValueAndTrue_When_PublicIdSet(t *testing.T) {
	id, err := NewBacklogItemID()
	if err != nil {
		t.Fatalf("NewBacklogItemID: %v", err)
	}
	data := &BacklogItemData{PublicIDRaw: id.String()}

	got, ok := data.PublicID()
	if !ok {
		t.Fatalf("PublicID() ok = false, want true for set PublicIDRaw %q", data.PublicIDRaw)
	}
	if got.String() != id.String() {
		t.Fatalf("PublicID() = %q, want %q", got.String(), id.String())
	}
}

func TestBacklogItemData_PublicID_should_ReturnZeroValueAndFalse_When_PublicIdUnset(t *testing.T) {
	data := &BacklogItemData{PublicIDRaw: ""}

	got, ok := data.PublicID()
	if ok {
		t.Fatalf("PublicID() ok = true, want false for unset PublicIDRaw")
	}
	if got.IsValid() {
		t.Fatalf("PublicID() returned a valid id alongside ok=false: %q", got.String())
	}
}

func TestIsBacklogItemIDShape_should_MatchOnlyPrefixedStrings(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"bl_01HXYZABCDEFGHJKMNPQRSTVWX", true},
		{"550e8400-e29b-41d4-a716-446655440000", false},
		{"", false},
		{"bl_", true},
	}
	for _, tt := range tests {
		if got := IsBacklogItemIDShape(tt.input); got != tt.want {
			t.Errorf("IsBacklogItemIDShape(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
