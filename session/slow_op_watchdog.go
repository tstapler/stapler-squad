package session

import (
	"bytes"
	"runtime/pprof"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// entSchemaCreateSlowAfter is far above any legitimate schema create (tens of
// milliseconds on an in-memory or small on-disk database).
const entSchemaCreateSlowAfter = 2 * time.Minute

// watchSlowOp logs a full goroutine dump if the returned stop func has not been
// called within after. It never cancels or alters the watched operation; it
// only makes a hang diagnosable (see ent schema create in NewEntRepository).
func watchSlowOp(name string, after time.Duration) (stop func()) {
	t := time.AfterFunc(after, func() {
		var buf bytes.Buffer
		if p := pprof.Lookup("goroutine"); p != nil {
			_ = p.WriteTo(&buf, 2)
		}
		log.Error("operation still running; goroutine dump follows", "op", name, "after", after.String(), "goroutines", buf.String())
	})
	return func() { t.Stop() }
}
