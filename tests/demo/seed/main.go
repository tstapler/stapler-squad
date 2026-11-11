// seed seeds mock demo data into a data directory so the stapler-squad server
// can be started with pre-populated data for E2E video recording.
//
// Usage:
//
//	go run ./tests/demo/seed <dir> [count]                        (legacy: default scenario)
//	go run ./tests/demo/seed --scenario a,b [--ref RFC3339] <dir>  (pre-boot file stage)
//	go run ./tests/demo/seed rpc --scenario a,b [--ref RFC3339] --url <server> [--workdir <dir>]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	demo "github.com/tstapler/stapler-squad/tests/demo"
	"github.com/tstapler/stapler-squad/tests/demo/scenario"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "rpc" {
		return runRPC(args[1:])
	}
	return runFiles(args)
}

type options struct {
	scenarios string
	ref       string
	url       string
	workdir   string
	rest      []string
}

func parse(name string, args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&o.scenarios, "scenario", scenario.DefaultName, "comma-separated scenario names")
	fs.StringVar(&o.ref, "ref", "", "reference instant (RFC3339); defaults to now")
	fs.StringVar(&o.url, "url", "", "server base URL (rpc stage)")
	fs.StringVar(&o.workdir, "workdir", "", "scratch dir for fixture repo and terminal scripts (rpc stage)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.rest = fs.Args()
	return o, nil
}

func (o options) refTime() (time.Time, error) {
	if o.ref == "" {
		return time.Now(), nil
	}
	return time.Parse(time.RFC3339, o.ref)
}

func (o options) compose() (*scenario.Composite, time.Time, error) {
	ref, err := o.refTime()
	if err != nil {
		return nil, ref, fmt.Errorf("invalid --ref: %w", err)
	}
	deps := scenario.Deps{WriteSessions: writeRecords, WriteDefault: demo.SeedDirectory, WorkDir: o.workdir}
	c, err := scenario.Compose(strings.Split(o.scenarios, ","), deps)
	return c, ref, err
}

func runFiles(args []string) error {
	o, err := parse("seed", args)
	if err != nil {
		return err
	}
	if len(o.rest) < 1 {
		return fmt.Errorf("usage: seed [--scenario a,b] <dir> [count]")
	}
	dir := o.rest[0]
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create dir: %w", err)
	}
	if len(o.rest) >= 2 {
		return legacy(dir, o.rest[1])
	}
	c, ref, err := o.compose()
	if err != nil {
		return err
	}
	if err := c.SeedFiles(dir, ref); err != nil {
		return err
	}
	fmt.Printf("seeded scenarios %s into %s\n", c.Name(), dir)
	return nil
}

// legacy keeps `seed <dir> <count>` working unchanged.
func legacy(dir, countArg string) error {
	count, err := strconv.Atoi(countArg)
	if err != nil {
		return fmt.Errorf("invalid count: %w", err)
	}
	if err := demo.SeedDirectory(dir, count); err != nil {
		return err
	}
	fmt.Printf("seeded %d demo sessions into %s\n", count, dir)
	return nil
}

func runRPC(args []string) error {
	o, err := parse("seed rpc", args)
	if err != nil {
		return err
	}
	if o.url == "" {
		return fmt.Errorf("--url is required")
	}
	if o.workdir == "" {
		return fmt.Errorf("--workdir is required")
	}
	c, ref, err := o.compose()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := c.SeedRPC(ctx, scenario.HTTPClient{BaseURL: o.url}, ref); err != nil {
		return err
	}
	fmt.Printf("rpc-seeded scenarios %s into %s\n", c.Name(), o.url)
	return nil
}
