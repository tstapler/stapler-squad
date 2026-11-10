package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// minAvailGB is the host-protection floor: no Bazel run starts below this much MemAvailable.
const minAvailGB = 15.0

// Cmd is one external command. Every check tool runs commands only through Exec so unit tests can fake them.
type Cmd struct {
	Dir  string
	Env  []string // appended to the process environment
	Name string
	Args []string
}

func (c Cmd) String() string { return c.Name + " " + strings.Join(c.Args, " ") }

// Out is a finished command.
type Out struct {
	Stdout, Stderr string
	Code           int
}

// Exec runs a command to completion; err is non-nil only when it could not be started or ctx ended it.
type Exec interface {
	Run(ctx context.Context, c Cmd) (Out, error)
	Start(c Cmd) (BgProc, error)
}

// BgProc is a background process (a loopback cache server) the caller must Kill.
type BgProc interface {
	Kill() error
	Wait() error
}

type osProc struct{ cmd *exec.Cmd }

func (p osProc) Kill() error { return p.cmd.Process.Kill() }
func (p osProc) Wait() error { return p.cmd.Wait() }

func (osExec) Start(c Cmd) (BgProc, error) {
	cmd := exec.Command(c.Name, c.Args...) //nolint:norawexec,norawgitcli // dev-time measurement tool; the single exec seam, killed by the caller
	cmd.Dir, cmd.Env = c.Dir, append(os.Environ(), c.Env...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return osProc{cmd}, nil
}

// Host reads machine state and sleeps; faked in tests so quiet-host waits take no real time.
type Host interface {
	Load1() float64
	MemAvailGB() float64
	Sleep(time.Duration)
	Now() time.Time
}

type osExec struct{}

func (osExec) Run(ctx context.Context, c Cmd) (Out, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...) //nolint:norawexec,norawgitcli // dev-time measurement tool; the single exec seam, waited on
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	o := Out{Stdout: so.String(), Stderr: se.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		o.Code = ee.ExitCode()
		return o, ctx.Err()
	}
	return o, err
}

type osHost struct{}

func (osHost) Load1() float64        { return loadNow() }
func (osHost) MemAvailGB() float64   { return memAvailGB() }
func (osHost) Sleep(d time.Duration) { time.Sleep(d) }
func (osHost) Now() time.Time        { return time.Now() }

// Reading is the host state around one timed step; it feeds the not_quiet tagging.
type Reading struct {
	Label                      string
	LoadPre, LoadPost          float64
	MemPre, MemPost            float64
	NotQuiet, OverrideAccepted bool
}

// Gate refuses to start below minAvailGB, then applies the quiet-host rule: wait up to wait for 1-minute load
// <= 2.0, or with allowNoisy start at once and tag the reading not_quiet.
func Gate(h Host, label string, allowNoisy bool, wait time.Duration, warn func(string)) (Reading, error) {
	r := Reading{Label: label}
	if m := h.MemAvailGB(); m >= 0 && m < minAvailGB {
		return r, fmt.Errorf("%s: only %.1f GB available, below the %.0f GB floor", label, m, minAvailGB)
	}
	end := h.Now().Add(wait)
	for {
		r.LoadPre = h.Load1()
		if !IsNoisy(r.LoadPre, r.LoadPre) {
			break
		}
		if allowNoisy {
			r.OverrideAccepted = true
			warn(fmt.Sprintf("NOISY HOST OVERRIDE: starting %s at 1-min load %.2f", label, r.LoadPre))
			break
		}
		if h.Now().After(end) {
			return r, fmt.Errorf("%s: host not quiet (1-min load %.2f > %.1f) after %s", label, r.LoadPre, quietLoad1, wait)
		}
		h.Sleep(5 * time.Second)
	}
	r.MemPre = h.MemAvailGB()
	return r, nil
}

// Finish records the post-step state and the final not_quiet verdict.
func (r *Reading) Finish(h Host) {
	r.LoadPost, r.MemPost = h.Load1(), h.MemAvailGB()
	r.NotQuiet = IsNoisy(r.LoadPre, r.LoadPost)
}

// AsBuilds adapts readings to the shape NoisyEntry/AnyNoisy already take.
func AsBuilds(rs []Reading) []Build {
	out := make([]Build, len(rs))
	for i, r := range rs {
		out[i] = Build{Label: r.Label, LoadPre: r.LoadPre, LoadPost: r.LoadPost, MemAvailPre: r.MemPre, MemAvailPost: r.MemPost, NotQuiet: r.NotQuiet}
	}
	return out
}
