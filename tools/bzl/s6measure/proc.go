package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Proc is one row of the process table.
type Proc struct {
	PID, PPID int
	RSSKB     int64
	Cmdline   string
}

// TreeRSS returns the summed RSS (KB) of every process whose cmdline contains any of match, plus all of their
// descendants. A process counted once however many roots reach it.
func TreeRSS(procs []Proc, match []string) (int64, int) {
	kids := map[int][]int{}
	byPID := map[int]Proc{}
	for _, p := range procs {
		byPID[p.PID] = p
		kids[p.PPID] = append(kids[p.PPID], p.PID)
	}
	seen := map[int]bool{}
	var walk func(int)
	walk = func(pid int) {
		if seen[pid] {
			return
		}
		seen[pid] = true
		for _, c := range kids[pid] {
			walk(c)
		}
	}
	for _, p := range procs {
		for _, m := range match {
			if m != "" && strings.Contains(p.Cmdline, m) {
				walk(p.PID)
				break
			}
		}
	}
	var sum int64
	for pid := range seen {
		sum += byPID[pid].RSSKB
	}
	return sum, len(seen)
}

// Peak tracks the maximum of a sample series.
type Peak struct {
	MaxKB   int64
	Samples int
}

func (p *Peak) Add(kb int64) {
	p.Samples++
	if kb > p.MaxKB {
		p.MaxKB = kb
	}
}

// ReadProcs reads /proc into a process table (best effort: vanished processes are skipped).
func ReadProcs() []Proc {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []Proc
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		st, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			continue
		}
		p := Proc{PID: pid}
		for _, ln := range strings.Split(string(st), "\n") {
			if v, ok := strings.CutPrefix(ln, "PPid:"); ok {
				p.PPID, _ = strconv.Atoi(strings.TrimSpace(v))
			} else if v, ok := strings.CutPrefix(ln, "VmRSS:"); ok {
				f := strings.Fields(v)
				if len(f) > 0 {
					p.RSSKB, _ = strconv.ParseInt(f[0], 10, 64)
				}
			}
		}
		cl, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		p.Cmdline = strings.ReplaceAll(string(cl), "\x00", " ")
		out = append(out, p)
	}
	return out
}
