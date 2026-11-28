package reviewqueuerpc

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain runs the tests hermetically: temp HOME, no ambient STAPLER_SQUAD_* state.
func TestMain(m *testing.M) {
	cleanup, err := svctest.UseTempHome()
	if err != nil {
		panic(err)
	}
	code := svctest.Main(m, func() {})
	cleanup()
	os.Exit(code)
}
