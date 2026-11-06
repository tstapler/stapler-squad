package unfinished

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/internal/dirwatch"
)

// Scanner tests assert on synchronous watcher delivery; FSEvents is
// asynchronous and its latency varies under load.
func TestMain(m *testing.M) {
	dirwatch.ForceFSNotify(true)
	os.Exit(m.Run())
}
