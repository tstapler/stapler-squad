// Package main compares two Bazel execution logs from clean output bases and fails when an action produced
// different output digests (Task 7.7a). Input is the JSON-lines SpawnExec stream written by
// --execution_log_json_file (or by converting a --execution_log_compact_file log).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type digest struct {
	Hash string `json:"hash"`
}

type outputFile struct {
	Path   string `json:"path"`
	Digest digest `json:"digest"`
}

// Spawn is the subset of SpawnExec the comparison needs. proto3 JSON omits false booleans.
type Spawn struct {
	TargetLabel   string       `json:"targetLabel"`
	Mnemonic      string       `json:"mnemonic"`
	ListedOutputs []string     `json:"listedOutputs"`
	Cacheable     bool         `json:"cacheable"`
	ActualOutputs []outputFile `json:"actualOutputs"`
}

func (s Spawn) key() string {
	outs := append([]string(nil), s.ListedOutputs...)
	sort.Strings(outs)
	return s.TargetLabel + "\x00" + s.Mnemonic + "\x00" + strings.Join(outs, "\x00")
}

// ReadLog parses a JSON-lines execution log. A repeated action keeps its last execution.
func ReadLog(r io.Reader) ([]Spawn, error) {
	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<20))
	var out []Spawn
	for {
		var s Spawn
		if err := dec.Decode(&s); err == io.EOF {
			return out, nil
		} else if err != nil {
			return nil, fmt.Errorf("execution log entry %d: %w", len(out)+1, err)
		}
		out = append(out, s)
	}
}

// Downgrade is one accepted row of hermetic-downgrades.txt.
type Downgrade struct {
	Action, Tool, Pin, Reason, Accepted string
}

// ParseDowngrades reads `action | tool | pin | reason | owner-accepted` rows; # comments and blanks are skipped.
func ParseDowngrades(r io.Reader) ([]Downgrade, error) {
	var out []Downgrade
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "|")
		if len(f) != 5 {
			return nil, fmt.Errorf("line %d: want 5 `|`-separated columns (action, tool, pin, reason, owner-accepted), got %d", n, len(f))
		}
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
			if f[i] == "" {
				return nil, fmt.Errorf("line %d: empty column %d", n, i+1)
			}
		}
		out = append(out, Downgrade{f[0], f[1], f[2], f[3], f[4]})
	}
	return out, sc.Err()
}

// Mismatch is one action whose output digests differ between the two logs.
type Mismatch struct {
	Mnemonic, Label, Path, DigestA, DigestB string
	Accepted                                bool
}

// Report is the comparison outcome.
type Report struct {
	Compared   int
	OnlyA      int
	OnlyB      int
	Mismatches []Mismatch
	// NowMatching lists accepted-downgrade mnemonics that matched in this run (candidates to drop the tag and the row).
	NowMatching []string
}

// Failed reports whether any mismatch is not an accepted downgrade.
func (r Report) Failed() bool {
	for _, m := range r.Mismatches {
		if !m.Accepted {
			return true
		}
	}
	return false
}

// Compare matches cacheable actions of a and b by label, mnemonic and listed outputs and compares output digests.
// accepted holds mnemonics from hermetic-downgrades.txt: their mismatches are reported but do not fail.
func Compare(a, b []Spawn, accepted map[string]bool) Report {
	idx := func(ss []Spawn) map[string]Spawn {
		m := map[string]Spawn{}
		for _, s := range ss {
			if s.Cacheable {
				m[s.key()] = s
			}
		}
		return m
	}
	ma, mb := idx(a), idx(b)
	var rep Report
	matched := map[string]bool{}
	mismatched := map[string]bool{}
	for _, k := range sortedKeys(ma) {
		sb, ok := mb[k]
		if !ok {
			rep.OnlyA++
			continue
		}
		rep.Compared++
		sa := ma[k]
		ms := diffOutputs(sa, sb, accepted[sa.Mnemonic])
		if len(ms) == 0 {
			matched[sa.Mnemonic] = true
			continue
		}
		mismatched[sa.Mnemonic] = true
		rep.Mismatches = append(rep.Mismatches, ms...)
	}
	for k := range mb {
		if _, ok := ma[k]; !ok {
			rep.OnlyB++
		}
	}
	for m := range matched {
		if accepted[m] && !mismatched[m] {
			rep.NowMatching = append(rep.NowMatching, m)
		}
	}
	sort.Strings(rep.NowMatching)
	return rep
}

func diffOutputs(a, b Spawn, accepted bool) []Mismatch {
	da, db := outputDigests(a), outputDigests(b)
	paths := map[string]bool{}
	for p := range da {
		paths[p] = true
	}
	for p := range db {
		paths[p] = true
	}
	var out []Mismatch
	for _, p := range sortedKeys(paths) {
		if da[p] != db[p] {
			out = append(out, Mismatch{Mnemonic: a.Mnemonic, Label: a.TargetLabel, Path: p, DigestA: da[p], DigestB: db[p], Accepted: accepted})
		}
	}
	return out
}

func outputDigests(s Spawn) map[string]string {
	m := map[string]string{}
	for _, f := range s.ActualOutputs {
		m[f.Path] = f.Digest.Hash
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
