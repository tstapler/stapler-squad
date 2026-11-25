package goalrunner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
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

// Run launches claude, parses its stream-json stdout and returns the outcome.
// The ceiling kills the process on its own, independent of /goal's exit.
func Run(ctx context.Context, cfg Config) Outcome {
	start := time.Now()
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	bin := cfg.Binary
	if bin == "" {
		bin = "claude"
	}
	ceiling := cfg.CostCeilingUSD
	if ceiling <= 0 {
		ceiling = DefaultCostCeilingUSD
	}
	hb := cfg.Heartbeat
	if hb <= 0 {
		hb = DefaultHeartbeat
	}
	newTicker := cfg.NewTicker
	if newTicker == nil {
		newTicker = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, cfg.Args()...)
	cmd.Dir = cfg.Dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{EndReason: EndError, Err: err}
	}
	cmd.Stderr = nil // discarded; stream-json carries errors on stdout
	if err := cmd.Start(); err != nil {
		return Outcome{EndReason: EndError, Err: err, WallTime: time.Since(start)}
	}

	var (
		sp       = &spend{pricing: tokens.DefaultPricingTable(), msgs: map[string]*msgUsage{}}
		turns    int
		result   *streamLine
		exceeded bool
		lines    = make(chan string)
		done     = make(chan struct{})
		scanErr  error
	)
	go func() {
		defer close(done)
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-runCtx.Done():
				return
			}
		}
		if scanErr = sc.Err(); scanErr != nil {
			cancel() // an unreadable (oversized) line would stall the pipe and the ceiling
		}
	}()

	tick, stopTick := newTicker(hb)
	defer stopTick()
loop:
	for {
		select {
		case <-tick:
			log.Info("goal run heartbeat", "elapsed", time.Since(start).Round(time.Second),
				"turns", turns, "est_cost_usd", sp.usd(), "ceiling_usd", ceiling)
		case line, ok := <-lines:
			if !ok {
				break loop
			}
			var l streamLine
			if json.Unmarshal([]byte(line), &l) != nil {
				continue // malformed line: skip
			}
			sp.add(l)
			if l.Type == "assistant" {
				turns++
			}
			if l.Type == "result" {
				r := l
				result = &r
				continue
			}
			if sp.usd() > ceiling {
				exceeded = true
				cancel()
				break loop
			}
		}
	}
	waitErr := cmd.Wait()
	<-done

	out := Outcome{Turns: turns, WallTime: time.Since(start), CostUSD: sp.usd()}
	switch {
	case exceeded:
		out.EndReason = EndCostExceeded
	case result != nil:
		out.EndReason = reasonFromResult(*result, cfg.maxTurns())
		out.CostUSD = result.CostUSD
		if out.CostUSD == 0 {
			out.CostUSD = result.TotalCostUSD
		}
		if result.NumTurns > 0 {
			out.Turns = result.NumTurns
		}
	default:
		out.EndReason = EndError
		out.Err = errors.Join(errors.New("no result line in stream"), scanErr, waitErr)
	}
	return out
}
