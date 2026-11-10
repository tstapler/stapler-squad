// Command mediatool holds the promo media subcommands:
//
//	mediatool fresh    freshness, budget and reuse checks over the media manifest
//	mediatool summary  append the manifest table to $GITHUB_STEP_SUMMARY
//	mediatool shard-plan  print the record-job matrix (round-robin over scene ids)
package main

import (
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv)) }

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: mediatool fresh|summary|shard-plan [flags]")
		return 2
	}
	switch args[0] {
	case "fresh":
		return runFresh(args[1:], stdout, stderr, getenv)
	case "summary":
		return runSummary(args[1:], stdout, stderr, getenv)
	case "shard-plan":
		return runShardPlan(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "mediatool: unknown subcommand %q\n", args[0])
		return 2
	}
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return fmt.Sprint([]string(*s)) }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }
