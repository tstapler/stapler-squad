package namegen

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
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
} // 80 adjectives — max length: "twilight" (8 chars)

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
} // 80 nouns — max length: "albatross" (9 chars)

// Generate returns a name string in YYYYMMDD-adjective-noun-NN format.
// It does NOT create a directory — it only generates the name string.
func Generate() string {
	date := time.Now().Format("20060102")
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	num := rand.Intn(100)
	return fmt.Sprintf("%s-%s-%s-%02d", date, adj, noun, num)
}

// GenerateAndCreate creates a unique subdirectory inside baseDir and returns
// the full absolute path. It uses os.Mkdir (atomic) for the leaf to handle
// concurrent creation races. It retries up to maxAttempts times.
// On success, the directory exists on disk and is owned by this call.
// Returns error if maxAttempts exhausted or baseDir cannot be written.
func GenerateAndCreate(baseDir string, maxAttempts int) (string, error) {
	return GenerateAndCreateWithFn(baseDir, maxAttempts, Generate)
}

// GenerateAndCreateWithFn is like GenerateAndCreate but uses a custom name
// generation function — useful for deterministic testing.
func GenerateAndCreateWithFn(baseDir string, maxAttempts int, generateFn func() string) (string, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create one_off_base_dir %q: %w", baseDir, err)
	}

	info, err := os.Stat(baseDir)
	if err != nil {
		return "", fmt.Errorf("cannot stat one_off_base_dir %q: %w", baseDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("one_off_base_dir %q exists but is not a directory", baseDir)
	}

	for i := 0; i < maxAttempts; i++ {
		name := generateFn()
		fullPath := filepath.Join(baseDir, name)
		if err := os.Mkdir(fullPath, 0755); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique one-off directory after %d attempts", maxAttempts)
}

// ExportedWordLists returns the adjectives and nouns slices for testing.
func ExportedWordLists() ([]string, []string) {
	return adjectives, nouns
}
