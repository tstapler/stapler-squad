// Package scenario defines named, deterministic seed scenarios for the promo
// tours. A scenario is pure data (a Bundle) built from one reference instant,
// so the same scenario seeded with two reference instants differs only by that
// shift. All data is fictional.
package scenario

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Seeded modes recorded on every fixture so the recorder knows how it reached
// the server (Task 0.1.1f fallback).
const (
	SeededFile  = "file"
	SeededRPC   = "rpc"
	SeededRoute = "route"
)

// DefaultName is the scenario that reproduces tests/demo's mockSessions().
const DefaultName = "default"

// FileSeeder seeds pre-boot state into a data directory (no server needed).
type FileSeeder interface {
	Name() string
	SeedFiles(dir string, ref time.Time) error
}

// RPCSeeder seeds post-boot state through the running server.
type RPCSeeder interface {
	Name() string
	SeedRPC(ctx context.Context, c Client, ref time.Time) error
}

// SessionWriter persists session records into a data directory. The seed CLI
// injects it so this package never imports the session/ent stack.
type SessionWriter func(dir string, recs []SessionRecord) error

// DefaultWriter seeds the legacy mock sessions (tests/demo SeedDirectory).
type DefaultWriter func(dir string, count int) error

// Deps are the injected side-effect seams.
type Deps struct {
	WriteSessions SessionWriter
	WriteDefault  DefaultWriter
	// DefaultCount is the number of legacy mock sessions (default 6).
	DefaultCount int
	// WorkDir holds the fixture repo and canned-terminal scripts for RPC seeding.
	WorkDir string
}

// builder produces one scenario's Bundle for a reference instant.
type builder struct {
	name  string
	build func(ref time.Time) Bundle
}

// Names returns every registered scenario name, sorted.
func Names() []string {
	bs := builders()
	names := make([]string, 1, len(bs)+1)
	names[0] = DefaultName
	for _, b := range bs {
		names = append(names, b.name)
	}
	sort.Strings(names)
	return names
}

func builders() []builder {
	return []builder{
		{"dashboard", dashboardBundle},
		{"approvals", approvalsBundle},
		{"backlog", backlogBundle},
		{"insights", insightsBundle},
		{"notifications", notificationsBundle},
		{"automation", automationBundle},
		{"fanout", fanoutBundle},
		{"files", filesBundle},
		{"history", historyBundle},
	}
}

// BuildBundle returns one named scenario's data.
func BuildBundle(name string, ref time.Time) (Bundle, error) {
	for _, b := range builders() {
		if b.name == name {
			return b.build(ref), nil
		}
	}
	return Bundle{}, fmt.Errorf("unknown scenario %q (known: %s)", name, strings.Join(Names(), ", "))
}

// Composite is the composition of several scenarios; it seeds both stages.
type Composite struct {
	names []string
	deps  Deps
}

// Name joins the composed scenario names.
func (c *Composite) Name() string { return strings.Join(c.names, ",") }

// Compose validates names and returns a seeder for their union. A duplicate
// record id across scenarios is an error naming both scenarios.
func Compose(names []string, deps Deps) (*Composite, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("no scenarios given")
	}
	if _, err := mergeBundles(names, time.Time{}); err != nil {
		return nil, err
	}
	if deps.DefaultCount == 0 {
		deps.DefaultCount = 6
	}
	return &Composite{names: names, deps: deps}, nil
}

// Bundle returns the merged data for ref (the default scenario contributes
// nothing here: its sessions come from the legacy writer).
func (c *Composite) Bundle(ref time.Time) (Bundle, error) {
	return mergeBundles(c.names, ref)
}

func mergeBundles(names []string, ref time.Time) (Bundle, error) {
	var merged Bundle
	owner := map[string]string{}
	for _, name := range names {
		if name == DefaultName {
			if _, dup := owner["scenario:"+DefaultName]; dup {
				return Bundle{}, fmt.Errorf("scenario %q listed twice", name)
			}
			owner["scenario:"+DefaultName] = name
			continue
		}
		b, err := BuildBundle(name, ref)
		if err != nil {
			return Bundle{}, err
		}
		for _, key := range b.keys() {
			if prev, dup := owner[key]; dup {
				return Bundle{}, fmt.Errorf("duplicate id %q in scenarios %q and %q", key, prev, name)
			}
			owner[key] = name
		}
		merged.append(b)
	}
	return merged, nil
}

// SeedFiles runs the pre-boot stage: legacy sessions (default scenario) and
// bundle session records.
func (c *Composite) SeedFiles(dir string, ref time.Time) error {
	for _, n := range c.names {
		if n == DefaultName {
			if c.deps.WriteDefault == nil {
				return fmt.Errorf("default scenario needs Deps.WriteDefault")
			}
			if err := c.deps.WriteDefault(dir, c.deps.DefaultCount); err != nil {
				return fmt.Errorf("seed default: %w", err)
			}
		}
	}
	b, err := c.Bundle(ref)
	if err != nil {
		return err
	}
	if len(b.Sessions) == 0 {
		return nil
	}
	if c.deps.WriteSessions == nil {
		return fmt.Errorf("scenarios %s need Deps.WriteSessions", c.Name())
	}
	return c.deps.WriteSessions(dir, b.Sessions)
}
