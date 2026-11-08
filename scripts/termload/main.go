// Command termload measures keystroke echo latency against a running stapler-squad while the
// session produces heavy output. It speaks the same Connect-over-WebSocket StreamTerminal
// protocol as the browser: it starts a bulk-output loop through the input path, then types
// one marker byte at a time and times each until that byte comes back in an output frame.
//
// Point it at an isolated instance (see CLAUDE.md "Manual/interactive testing"), never the
// live service. The session must be running a shell:
//
//	go run ./scripts/termload -url http://127.0.0.1:62871 -session perf-shell -bulk=true
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/protocol"
)

// marker is a byte the bulk loop (digits and newlines) never emits, so seeing it in output
// means the typed byte was echoed.
const marker = 'z'

type config struct {
	baseURL  string
	session  string
	duration time.Duration
	interval time.Duration
	timeout  time.Duration
	bulk     bool
	cols     int
	rows     int
	resync   time.Duration
}

func main() {
	var c config
	flag.StringVar(&c.baseURL, "url", "http://127.0.0.1:62871", "instance base URL")
	flag.StringVar(&c.session, "session", "", "session title (required)")
	flag.DurationVar(&c.duration, "duration", 20*time.Second, "how long to type")
	flag.DurationVar(&c.interval, "interval", 100*time.Millisecond, "gap between keystrokes")
	flag.DurationVar(&c.timeout, "timeout", 3*time.Second, "give up on one echo after this long")
	flag.BoolVar(&c.bulk, "bulk", true, "run a heavy-output loop in the session while typing")
	flag.IntVar(&c.cols, "cols", 120, "terminal columns")
	flag.IntVar(&c.rows, "rows", 40, "terminal rows")
	flag.DurationVar(&c.resync, "resync-every", 0, "send a mid-stream CurrentPaneRequest this often (a refit/resync storm); 0 disables")
	flag.Parse()
	if c.session == "" {
		fmt.Fprintln(os.Stderr, "-session is required")
		os.Exit(2)
	}
	if err := run(c); err != nil {
		fmt.Fprintln(os.Stderr, "termload:", err)
		os.Exit(1)
	}
}

type client struct {
	wmu     sync.Mutex // gorilla allows one concurrent writer
	conn    *websocket.Conn
	session string
	out     chan []byte // decoded output payloads, in order
	errc    chan error
}

func dial(c config) (*client, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	scheme := map[string]string{"http": "ws", "https": "wss"}[u.Scheme]
	wsURL := fmt.Sprintf("%s://%s/api/session.v1.SessionService/StreamTerminal", scheme, u.Host)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	cl := &client{conn: conn, session: c.session, out: make(chan []byte, 4096), errc: make(chan error, 1)}

	hello := &sessionv1.TerminalData{SessionId: c.session, Data: &sessionv1.TerminalData_CurrentPaneRequest{
		CurrentPaneRequest: &sessionv1.CurrentPaneRequest{IncludeEscapes: true, TargetCols: proto.Int32(int32(c.cols)), TargetRows: proto.Int32(int32(c.rows))},
	}}
	body, err := proto.Marshal(hello)
	if err != nil {
		return nil, err
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("Content-Type: application/connect+proto\r\n\r\n")); err != nil {
		return nil, err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, body)); err != nil {
		return nil, err
	}
	go cl.readLoop()
	return cl, nil
}

func (cl *client) readLoop() {
	for {
		kind, msg, err := cl.conn.ReadMessage()
		if err != nil {
			cl.errc <- err
			return
		}
		if kind != websocket.BinaryMessage {
			continue
		}
		env, _, err := protocol.ParseEnvelope(msg)
		if err != nil || env.IsEndStream() {
			continue
		}
		data := env.Data
		if env.IsCompressed() {
			zr, zerr := gzip.NewReader(bytes.NewReader(data))
			if zerr != nil {
				continue
			}
			data, _ = io.ReadAll(zr)
		}
		var td sessionv1.TerminalData
		if proto.Unmarshal(data, &td) != nil {
			continue
		}
		if o := td.GetOutput(); o != nil {
			cl.out <- o.Data
		}
	}
}

func (cl *client) send(b []byte) error {
	td := &sessionv1.TerminalData{SessionId: cl.session, Data: &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: b}}}
	body, err := proto.Marshal(td)
	if err != nil {
		return err
	}
	return cl.write(protocol.CreateEnvelope(0, body))
}

func (cl *client) write(frame []byte) error {
	cl.wmu.Lock()
	defer cl.wmu.Unlock()
	return cl.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// resyncLoop asks for the current pane every d until stop closes, as a refit storm would.
func (cl *client) resyncLoop(d time.Duration, c config, stop <-chan struct{}) {
	req := &sessionv1.TerminalData{SessionId: cl.session, Data: &sessionv1.TerminalData_CurrentPaneRequest{
		CurrentPaneRequest: &sessionv1.CurrentPaneRequest{IncludeEscapes: true, TargetCols: proto.Int32(int32(c.cols)), TargetRows: proto.Int32(int32(c.rows))},
	}}
	body, err := proto.Marshal(req)
	if err != nil {
		return
	}
	for {
		select {
		case <-stop:
			return
		case <-time.After(d):
			_ = cl.write(protocol.CreateEnvelope(0, body))
		}
	}
}

// awaitMarker drains output until a frame contains the marker byte, or the timeout passes.
func (cl *client) awaitMarker(timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case o := <-cl.out:
			if bytes.IndexByte(o, marker) >= 0 {
				return true
			}
		case <-deadline:
			return false
		case err := <-cl.errc:
			fmt.Fprintln(os.Stderr, "stream ended:", err)
			return false
		}
	}
}

func run(c config) error {
	cl, err := dial(c)
	if err != nil {
		return err
	}
	defer cl.conn.Close()
	time.Sleep(2 * time.Second) // let the first paint land

	if c.bulk {
		if err := cl.send([]byte("while :; do seq 1 4000; done\r")); err != nil {
			return err
		}
		time.Sleep(time.Second)
	}

	if c.resync > 0 {
		stop := make(chan struct{})
		defer close(stop)
		go cl.resyncLoop(c.resync, c, stop)
	}

	var res results
	end := time.Now().Add(c.duration)
	for time.Now().Before(end) {
		start := time.Now()
		if err := cl.send([]byte{marker}); err != nil {
			return err
		}
		res.sent++
		if cl.awaitMarker(c.timeout) {
			res.echoesMs = append(res.echoesMs, float64(time.Since(start))/float64(time.Millisecond))
		} else {
			res.lost++
		}
		if rest := c.interval - time.Since(start); rest > 0 {
			time.Sleep(rest)
		}
	}
	if c.bulk {
		_ = cl.send([]byte{3}) // Ctrl-C ends the loop
	}
	res.print(c)
	return nil
}

type results struct {
	echoesMs   []float64
	sent, lost int
}

func (r results) print(c config) {
	echoes := r.echoesMs
	sort.Float64s(echoes)
	pct := func(p float64) float64 {
		if len(echoes) == 0 {
			return 0
		}
		i := int(p/100*float64(len(echoes))+0.999999) - 1
		return echoes[max(0, min(i, len(echoes)-1))]
	}
	fmt.Printf("session=%s bulk=%v sent=%d echoed=%d lost=%d\n", c.session, c.bulk, r.sent, len(echoes), r.lost)
	fmt.Printf("echo_ms p50=%.1f p95=%.1f p99=%.1f max=%.1f\n", pct(50), pct(95), pct(99), pct(100))
	fmt.Println(strings.Repeat("-", 40))
}
