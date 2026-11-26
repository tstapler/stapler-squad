package goalrunner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"syscall"
	"time"

	"github.com/tstapler/stapler-squad/session/tokens"
)

const (
	DefaultMaxTurns       = 20
	DefaultCostCeilingUSD = 2.00
	DefaultHeartbeat      = 60 * time.Second
	maxLineBytes          = 4 << 20
)

// Config describes one run. Zero values take the defaults above.
type Config struct {
	Binary         string // default "claude"
	Criteria       string
	MaxTurns       int
	CostCeilingUSD float64
	Heartbeat      time.Duration
	Dir            string
	// SettingsPath is passed as --settings; it carries the hooks for the run.
	SettingsPath string
	Logger       *slog.Logger
	// NewTicker is a test seam; nil uses time.NewTicker.
	NewTicker func(time.Duration) (<-chan time.Time, func())
}

func (c Config) maxTurns() int {
	if c.MaxTurns <= 0 {
		return DefaultMaxTurns
	}
	return c.MaxTurns
}

// Goal returns the /goal prompt; the judge evaluates the trailing turn bound.
func (c Config) Goal() string {
	return fmt.Sprintf("/goal %s or stop after %d turns", c.Criteria, c.maxTurns())
}

// Args builds the claude argument list (no shell involved).
func (c Config) Args() []string {
	args := []string{"-p", c.Goal(), "--output-format", "stream-json", "--verbose"}
	if c.SettingsPath != "" {
		args = append(args, "--settings", c.SettingsPath)
	}
	return args
}

type streamLine struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	IsError      bool    `json:"is_error"`
	EndReason    string  `json:"end_reason"`
	CostUSD      float64 `json:"cost_usd"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	NumTurns     int     `json:"num_turns"`
	Message      struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input         int64 `json:"input_tokens"`
			Output        int64 `json:"output_tokens"`
			CacheCreation int64 `json:"cache_creation_input_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type msgUsage struct {
	model                  string
	in, out, cacheW, cache int64
}

// spend estimates cumulative USD from assistant lines, keeping the per-message
// maximum because one API message repeats its usage across content blocks.
// An unpriced model is charged at the most expensive known rate.
type spend struct {
	pricing *tokens.PricingTable
	msgs    map[string]*msgUsage
}

func (s *spend) add(l streamLine) {
	if l.Type != "assistant" || l.Message.ID == "" {
		return
	}
	m := s.msgs[l.Message.ID]
	if m == nil {
		m = &msgUsage{}
		s.msgs[l.Message.ID] = m
	}
	if l.Message.Model != "" {
		m.model = l.Message.Model
	}
	u := l.Message.Usage
	m.in, m.out = max(m.in, u.Input), max(m.out, u.Output)
	m.cacheW, m.cache = max(m.cacheW, u.CacheCreation), max(m.cache, u.CacheRead)
}

func (s *spend) usd() float64 {
	var total float64
	for _, m := range s.msgs {
		p, ok := s.pricing.LookupByModel(m.model)
		if !ok {
			p = s.worst()
		}
		total += float64(m.in)/1e6*p.InputPricePerMTok + float64(m.out)/1e6*p.OutputPricePerMTok +
			float64(m.cacheW)/1e6*p.CacheWritePerMTok + float64(m.cache)/1e6*p.CacheReadPerMTok
	}
	return total
}

func (s *spend) worst() tokens.ModelPricing {
	var w tokens.ModelPricing
	for _, p := range s.pricing.Prices {
		w.InputPricePerMTok = max(w.InputPricePerMTok, p.InputPricePerMTok)
		w.OutputPricePerMTok = max(w.OutputPricePerMTok, p.OutputPricePerMTok)
		w.CacheWritePerMTok = max(w.CacheWritePerMTok, p.CacheWritePerMTok)
		w.CacheReadPerMTok = max(w.CacheReadPerMTok, p.CacheReadPerMTok)
	}
	return w
}

// reasonFromResult maps the final result object to an EndReason. An explicit
// end_reason wins; otherwise is_error and the max-turns subtype are used.
func reasonFromResult(l streamLine, maxTurns int) EndReason {
	switch EndReason(l.EndReason) {
	case EndGoalMet, EndTurnLimit, EndImpossible, EndError:
		return EndReason(l.EndReason)
	}
	switch {
	case l.Subtype == "error_max_turns":
		return EndTurnLimit
	case l.IsError:
		return EndError
	case maxTurns > 0 && l.NumTurns >= maxTurns:
		// The bound lives only in the /goal prompt, so a success at the bound is a turn stop.
		return EndTurnLimit
	default:
		return EndGoalMet
	}
}

// withDefaults fills the zero values of cfg.
func (c Config) withDefaults() Config {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Binary == "" {
		c.Binary = "claude"
	}
	if c.CostCeilingUSD <= 0 {
		c.CostCeilingUSD = DefaultCostCeilingUSD
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = DefaultHeartbeat
	}
	if c.NewTicker == nil {
		c.NewTicker = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		}
	}
	return c
}

// tracker folds stream lines into spend, turns and the final result.
type tracker struct {
	sp     *spend
	turns  int
	result *streamLine
}

// observe records one line and reports whether spend now exceeds the ceiling.
// Malformed lines are skipped.
func (t *tracker) observe(line string, ceiling float64) bool {
	var l streamLine
	if json.Unmarshal([]byte(line), &l) != nil {
		return false
	}
	t.sp.add(l)
	if l.Type == "assistant" {
		t.turns++
	}
	if l.Type == "result" {
		t.result = &l
		return false
	}
	return t.sp.usd() > ceiling
}

// lineReader scans stdout on its own goroutine so the main loop can select on it.
type lineReader struct {
	lines   chan string
	done    chan struct{}
	scanErr error // valid after done is closed
}

func startLineReader(ctx context.Context, cancel context.CancelFunc, stdout io.Reader) *lineReader {
	r := &lineReader{lines: make(chan string), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		defer close(r.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
		for sc.Scan() {
			select {
			case r.lines <- sc.Text():
			case <-ctx.Done():
				return
			}
		}
		if r.scanErr = sc.Err(); r.scanErr != nil {
			cancel() // an unreadable (oversized) line would stall the pipe and the ceiling
		}
	}()
	return r
}

func (t *tracker) outcome(cfg Config, exceeded bool, wall time.Duration, waitErr, scanErr error) Outcome {
	out := Outcome{Turns: t.turns, WallTime: wall, CostUSD: t.sp.usd()}
	switch {
	case exceeded:
		out.EndReason = EndCostExceeded
	case t.result != nil:
		out.EndReason = reasonFromResult(*t.result, cfg.maxTurns())
		out.CostUSD = t.result.CostUSD
		if out.CostUSD == 0 {
			out.CostUSD = t.result.TotalCostUSD
		}
		if t.result.NumTurns > 0 {
			out.Turns = t.result.NumTurns
		}
		if waitErr != nil && out.EndReason != EndError {
			// A non-zero exit after a result line is not a clean finish.
			out.EndReason, out.Err = EndError, waitErr
		}
	default:
		out.EndReason = EndError
		out.Err = errors.Join(errors.New("no result line in stream"), scanErr, waitErr)
	}
	return out
}

// Run launches claude, parses its stream-json stdout and returns the outcome.
// The ceiling kills the process on its own, independent of /goal's exit.
func Run(ctx context.Context, cfg Config) Outcome {
	start := time.Now()
	cfg = cfg.withDefaults()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, cfg.Binary, cfg.Args()...)
	cmd.Dir = cfg.Dir
	// Bounds Wait when a grandchild (Bash tool, hook) keeps the stdout pipe open
	// after claude is killed; the grandchild itself is not killed.
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{EndReason: EndError, Err: fmt.Errorf("goalrunner: stdout pipe: %w", err)}
	}
	cmd.Stderr = nil // discarded; stream-json carries errors on stdout
	if err := startRetryingTextBusy(cmd); err != nil {
		return Outcome{EndReason: EndError, Err: fmt.Errorf("goalrunner: start %s: %w", cfg.Binary, err), WallTime: time.Since(start)}
	}

	tr := &tracker{sp: &spend{pricing: tokens.DefaultPricingTable(), msgs: map[string]*msgUsage{}}}
	rd := startLineReader(runCtx, cancel, stdout)
	tick, stopTick := cfg.NewTicker(cfg.Heartbeat)
	defer stopTick()
	exceeded := false
	for !exceeded {
		select {
		case <-tick:
			cfg.Logger.Info("goal run heartbeat", "elapsed", time.Since(start).Round(time.Second),
				"turns", tr.turns, "est_cost_usd", tr.sp.usd(), "ceiling_usd", cfg.CostCeilingUSD)
		case line, ok := <-rd.lines:
			if !ok {
				goto finished
			}
			exceeded = tr.observe(line, cfg.CostCeilingUSD)
		}
	}
	cancel()
finished:
	waitErr := cmd.Wait()
	<-rd.done
	out := tr.outcome(cfg, exceeded, time.Since(start), waitErr, rd.scanErr)
	if ctx.Err() != nil && out.EndReason == EndError {
		out.Err = errors.Join(out.Err, ctx.Err())
	}
	return out
}

// startRetryingTextBusy retries Start on ETXTBSY: a freshly written executable
// can be briefly held open for writing by a concurrent fork (golang/go#22315).
func startRetryingTextBusy(cmd *exec.Cmd) error {
	var err error
	for range 5 {
		if err = cmd.Start(); !errors.Is(err, syscall.ETXTBSY) {
			return err
		}
		time.Sleep(20 * time.Millisecond) // bounded: 5 x 20ms
	}
	return err
}
