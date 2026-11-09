package tymux

import (
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// maxEchoDaemonGap bounds what counts as an echo; see the matching bound on echo_server.
const maxEchoDaemonGap = 2 * time.Second

// echoProbe splits echo_server using the daemon's per-frame send stamp (AttachEvent
// sent_at_unix_micros, capability output.sent_at). Daemon and squad share a host, so the stamp
// and time.Now are comparable. A frame with stamp 0 comes from a daemon without the capability
// and is ignored, which is the capability gate. The zero value records to termlat.Default.
type echoProbe struct {
	rec *termlat.Recorder
	now func() time.Time
	// lastInputNs is the wall-clock start of the newest keystroke send not yet followed by a
	// stamped output frame; 0 when none is waiting.
	lastInputNs atomic.Int64
}

func (p *echoProbe) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *echoProbe) recorder() *termlat.Recorder {
	if p.rec != nil {
		return p.rec
	}
	return termlat.Default
}

// noteInput marks the start of a keystroke send, before any wait for the stream's locks.
func (p *echoProbe) noteInput(at time.Time) { p.lastInputNs.Store(at.UnixNano()) }

// noteOutput handles one output frame carrying the daemon's stamp. Every stamped frame records
// output_transit; the first one after a keystroke also records echo_daemon. Like echo_server,
// echo_daemon is an upper bound under bulk output because the first frame need not be the echo.
func (p *echoProbe) noteOutput(sentAtMicros uint64) {
	if sentAtMicros == 0 {
		return
	}
	recv := p.clock()
	sent := time.UnixMicro(int64(sentAtMicros))
	transit := recv.Sub(sent)
	if transit < 0 {
		return // stamp from the future: the clocks disagree, so neither sample is trustworthy
	}
	rec := p.recorder()
	rec.Record(termlat.StageOutputTransit, tymuxLabels, transit)
	in := p.lastInputNs.Swap(0)
	if in == 0 {
		return
	}
	if d := sent.Sub(time.Unix(0, in)); d >= 0 && d <= maxEchoDaemonGap {
		rec.Record(termlat.StageEchoDaemon, tymuxLabels, d)
	}
}
