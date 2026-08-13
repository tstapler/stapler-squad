// seed seeds mock demo sessions into a data directory so the stapler-squad
// server can be started with pre-populated data for E2E video recording.
//
// Usage: go run ./tests/demo/seed <dir> [count]
package main

import (
	"fmt"
	"os"
	"strconv"

	demo "github.com/tstapler/stapler-squad/tests/demo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: seed <dir> [count]\n")
		os.Exit(1)
	}
	dir := os.Args[1]
	count := 6
	if len(os.Args) >= 3 {
		var err error
		count, err = strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid count: %v\n", err)
			os.Exit(1)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create dir: %v\n", err)
		os.Exit(1)
	}
	if err := demo.SeedDirectory(dir, count); err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("seeded %d demo sessions into %s\n", count, dir)
}
