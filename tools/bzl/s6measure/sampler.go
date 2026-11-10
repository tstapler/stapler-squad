package main

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sampler polls /proc every 500 ms and tracks the peak combined RSS of the registered output bases' process trees.
type Sampler struct {
	mu         sync.Mutex
	matches    []string
	cancels    []func()
	started    bool
	stop       chan struct{}
	done       chan struct{}
	peak       Peak
	WatchdogKB int64
	Fired      bool
}

// Add registers an output base (its bazel server cmdline contains --output_base=<ob>) and starts polling.
func (s *Sampler) Add(ob string, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.matches = append(s.matches, "--output_base="+ob)
	s.cancels = append(s.cancels, cancel)
	if s.started {
		return
	}
	s.started, s.stop, s.done = true, make(chan struct{}), make(chan struct{})
	go func() {
		defer close(s.done)
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.mu.Lock()
				kb, _ := TreeRSS(ReadProcs(), s.matches)
				s.peak.Add(kb)
				if s.WatchdogKB > 0 && kb > s.WatchdogKB && !s.Fired {
					s.Fired = true
					for _, c := range s.cancels {
						c()
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

// Stop ends polling and returns the peak combined RSS in KB.
func (s *Sampler) Stop() int64 {
	s.mu.Lock()
	st, dn := s.stop, s.done
	s.mu.Unlock()
	if st != nil {
		close(st)
		<-dn
	}
	return s.peak.MaxKB
}

// Median of a series, -1 when empty.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return -1
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// ParseLoad1 parses the first /proc/loadavg field, -1 on error.
func ParseLoad1(s string) float64 {
	f := strings.Fields(s)
	if len(f) == 0 {
		return -1
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return -1
	}
	return v
}

// ParseMemAvailGB reads MemAvailable from /proc/meminfo text, -1 when absent.
func ParseMemAvailGB(s string) float64 {
	for _, ln := range strings.Split(s, "\n") {
		if v, ok := strings.CutPrefix(ln, "MemAvailable:"); ok {
			f := strings.Fields(v)
			if len(f) > 0 {
				kb, err := strconv.ParseFloat(f[0], 64)
				if err == nil {
					return kb / 1024 / 1024
				}
			}
		}
	}
	return -1
}
