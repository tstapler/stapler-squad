package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// SC1 and SC3 caps from ADR-010 Decision 2.
const (
	SC1MaxCount = 10
	SC3MaxCount = 5
)

// Candidate holds the measured inputs of the SC1 and SC3 criteria for one cluster.
type Candidate struct {
	// Lower is a cluster the core package imports (the core's own callers, not the cluster, hold the references).
	Lower bool
	// Relocated is the number of declarations moved into the core so the cluster no longer needs them.
	Relocated int
	// StayingUses is the number of distinct cluster declarations the staying files still use.
	StayingUses int
	// Importers are the non-test packages outside the original that import the cluster after the move.
	Importers []string
	// NaturalCallers are the importers that already imported the original package before the move.
	NaturalCallers []string
}

// SC1Count is the SC1 figure. ADR-010 amendment 2026-11-05: for a lower package it counts declarations relocated into
// the core, not the core's uses of the cluster; for any other cluster it counts the uses staying files still make.
func (c Candidate) SC1Count() int {
	if c.Lower {
		return c.Relocated
	}
	return c.StayingUses
}

// SC3Importers is the SC3 figure. ADR-010 amendment 2026-11-05: a lower package's natural callers (importers that
// already imported the original) are not counted; every importer counts for any other cluster.
func (c Candidate) SC3Importers() []string {
	if !c.Lower {
		return c.Importers
	}
	natural := map[string]bool{}
	for _, p := range c.NaturalCallers {
		natural[p] = true
	}
	var out []string
	for _, p := range c.Importers {
		if !natural[p] {
			out = append(out, p)
		}
	}
	return out
}

// SC1OK and SC3OK apply the caps.
func (c Candidate) SC1OK() bool { return c.SC1Count() <= SC1MaxCount }
func (c Candidate) SC3OK() bool { return len(c.SC3Importers()) <= SC3MaxCount }

// PrintCriteria writes the SC1 and SC3 verdicts and reports whether both pass.
func PrintCriteria(w io.Writer, c Candidate) bool {
	kind := "upper"
	if c.Lower {
		kind = "lower"
	}
	fmt.Fprintf(w, "SC1: %s (%s package, count=%d max=%d)\n", verdict(c.SC1OK()), kind, c.SC1Count(), SC1MaxCount)
	imp := c.SC3Importers()
	fmt.Fprintf(w, "SC3: %s (%s package, importers=%d max=%d [%s])\n", verdict(c.SC3OK()), kind, len(imp), SC3MaxCount, strings.Join(imp, " "))
	return c.SC1OK() && c.SC3OK()
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// runCriteria evaluates SC1 and SC3 from numbers the closure/move dry-runs report; exit 1 when either fails.
func runCriteria(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("criteria", flag.ContinueOnError)
	c := Candidate{}
	fs.BoolVar(&c.Lower, "lower", false, "the cluster is a lower package the core imports")
	fs.IntVar(&c.Relocated, "relocated", 0, "declarations relocated into the core")
	fs.IntVar(&c.StayingUses, "uses", 0, "distinct cluster declarations staying files still use")
	importers := fs.String("importers", "", "comma-separated non-test importers outside the original")
	natural := fs.String("natural", "", "comma-separated importers that already imported the original")
	if fs.Parse(args) != nil {
		fmt.Fprintln(os.Stderr, "usage: scoping criteria [--lower] --relocated N --uses N --importers a,b --natural a")
		return 2
	}
	c.Importers, c.NaturalCallers = splitList(*importers), splitList(*natural)
	if PrintCriteria(stdout, c) {
		return 0
	}
	return 1
}
