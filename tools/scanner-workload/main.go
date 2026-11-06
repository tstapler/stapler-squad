// Command scanner-workload builds a per-instance repo corpus, seeds the
// unfinished-work scanner's state file, and drives a seeded, go-git-only event
// stream against the corpus copy, so two manual stapler-squad instances scan
// the same repos and see the same events.
//
//	scanner-workload corpus  -instance A [-sources a,b,...] [-worktrees 3]
//	scanner-workload seed    -instance A
//	scanner-workload drive   -instance A -seed 1 [-cadence cadence.json]
//	scanner-workload verify  -instance-a A -instance-b B
//	scanner-workload cadence [-sources ...] [-days 90] [-o cadence.json]
//
// It never runs against the owner's real repos: corpus only reads them, and
// drive only touches paths listed in the corpus manifest.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

const defaultSourcesDir = "code/github.com/tstapler"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "scanner-workload:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: scanner-workload corpus|seed|drive|verify|cadence [flags]")
	}
	cmds := map[string]func([]string, io.Writer, io.Writer) error{
		"corpus": cmdCorpus, "seed": cmdSeed, "drive": cmdDrive, "verify": cmdVerify, "cadence": cmdCadence,
	}
	cmd, ok := cmds[args[0]]
	if !ok {
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
	return cmd(args[1:], stdout, stderr)
}

// instanceFlags registers the config-dir/instance/corpus-root trio.
type instanceFlags struct{ configDir, instance, corpusRoot string }

func (f *instanceFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.configDir, "config-dir", "", "instance config dir (overrides -instance)")
	fs.StringVar(&f.instance, "instance", "", "STAPLER_SQUAD_INSTANCE name; config dir is ~/.stapler-squad/instances/<name>")
	fs.StringVar(&f.corpusRoot, "corpus-root", "", "corpus root (default <config-dir>/corpus)")
}

func (f *instanceFlags) resolve() (configDir, root string, err error) {
	configDir, err = ResolveConfigDir(f.configDir, f.instance)
	if err != nil {
		return "", "", err
	}
	root = f.corpusRoot
	if root == "" {
		root = filepath.Join(configDir, "corpus")
	}
	return configDir, root, nil
}

func defaultSources() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return DiscoverSources(filepath.Join(home, defaultSourcesDir), "stapler-squad")
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cmdCorpus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("corpus", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var inst instanceFlags
	inst.register(fs)
	sources := fs.String("sources", "", "comma-separated owner repo paths (default: discovered under ~/"+defaultSourcesDir+")")
	worktrees := fs.Int("worktrees", 3, "linked worktrees per copied repo (W)")
	maxMB := fs.Int64("max-repo-mb", 4096, "skip repos larger than this many MB after -skip (0 disables)")
	maxRepos := fs.Int("max-repos", 6, "stop after this many copied repos (0 = no cap)")
	skip := fs.String("skip", ".git/lfs,.claude/worktrees,node_modules", "extra comma-separated paths left out of the copy (a name without a slash matches at any depth)")
	seed := fs.Bool("seed", true, "also seed unfinished_state.json after copying")
	if err := fs.Parse(args); err != nil {
		return err
	}
	configDir, root, err := inst.resolve()
	if err != nil {
		return err
	}
	srcs := splitList(*sources)
	if len(srcs) == 0 {
		srcs = defaultSources()
	}
	m, err := BuildCorpus(CorpusOptions{
		Sources: srcs, Root: root, Worktrees: *worktrees, MaxRepoMB: *maxMB,
		SkipPaths: splitList(*skip), MaxRepos: *maxRepos, Log: stderr,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "copied %d repos, skipped %d; manifest %s\n", len(m.Repos), len(m.Skipped), filepath.Join(root, manifestName))
	if *seed {
		if err := SeedState(configDir, m.RepoPaths()); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "seeded %s\n", filepath.Join(configDir, stateFile))
	}
	return nil
}

func cmdSeed(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var inst instanceFlags
	inst.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	configDir, root, err := inst.resolve()
	if err != nil {
		return err
	}
	m, err := ReadManifest(root)
	if err != nil {
		return fmt.Errorf("read manifest (run corpus first): %w", err)
	}
	if err := SeedState(configDir, m.RepoPaths()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "seeded %s with %d pinned repos\n", filepath.Join(configDir, stateFile), len(m.Repos))
	return nil
}

func cmdVerify(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var a, b instanceFlags
	fs.StringVar(&a.configDir, "config-dir-a", "", "config dir of instance A")
	fs.StringVar(&a.instance, "instance-a", "", "instance name A")
	fs.StringVar(&a.corpusRoot, "corpus-root-a", "", "corpus root A (default <config-dir-a>/corpus)")
	fs.StringVar(&b.configDir, "config-dir-b", "", "config dir of instance B")
	fs.StringVar(&b.instance, "instance-b", "", "instance name B")
	fs.StringVar(&b.corpusRoot, "corpus-root-b", "", "corpus root B (default <config-dir-b>/corpus)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ca, ra, err := a.resolve()
	if err != nil {
		return err
	}
	cb, rb, err := b.resolve()
	if err != nil {
		return err
	}
	if err := VerifyPinned(ca, ra, cb, rb); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "PinnedRepos identical on both instances (relative to each corpus root)")
	return nil
}

func cmdCadence(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("cadence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sources := fs.String("sources", "", "comma-separated owner repo paths (default: discovered)")
	days := fs.Int("days", 90, "window for commits/day")
	out := fs.String("o", "", "write JSON here instead of stdout")
	skip := fs.String("skip", ".git/lfs,.claude/worktrees,node_modules", "paths excluded from size_bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	srcs := splitList(*sources)
	if len(srcs) == 0 {
		srcs = defaultSources()
	}
	var rep CadenceReport
	now := time.Now() // measurement boundary only; the driver never reads it
	for _, s := range srcs {
		c, err := MeasureCadence(s, *days, now, splitList(*skip))
		if err != nil {
			fmt.Fprintf(stderr, "skip %s: %v\n", s, err)
			continue
		}
		rep.Repos = append(rep.Repos, c)
	}
	return writeJSON(rep, *out, stdout)
}

func cmdDrive(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("drive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var inst instanceFlags
	inst.register(fs)
	seed := fs.Uint64("seed", 1, "schedule seed")
	duration := fs.Duration("duration", 30*time.Minute, "logical run length")
	tick := fs.Duration("tick", time.Minute, "idle tick spacing")
	cadence := fs.String("cadence", "", "cadence JSON from the cadence subcommand")
	defCommits := fs.Float64("default-commits-per-day", 5, "cadence for repos missing from -cadence")
	speedup := fs.Float64("speedup", 1, "multiplier on the observed cadence (1 = real rate)")
	fsOps := fs.Float64("fs-ops-per-hour", 12, "edit/create/delete/gitignore events per repo per hour (initial guess; retune against non-commit-walk buckets)")
	logPath := fs.String("log", "", "write the op log here (default <corpus-root>/oplog-seed<seed>.jsonl)")
	fast := fs.Bool("no-wait", false, "apply ops back to back instead of pacing to logical offsets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, root, err := inst.resolve()
	if err != nil {
		return err
	}
	m, err := ReadManifest(root)
	if err != nil {
		return fmt.Errorf("read manifest (run corpus first): %w", err)
	}
	cfg := DriverConfig{
		Seed: *seed, Duration: *duration, Tick: *tick, Targets: TargetsFromManifest(root, m),
		DefaultCommitsPerDay: *defCommits, Speedup: *speedup, FSOpsPerHour: *fsOps,
	}
	if *cadence != "" {
		rep, err := readCadence(*cadence)
		if err != nil {
			return err
		}
		cfg.CommitsPerDay = rep.PerDay()
	}
	if !*fast {
		cfg.Clock = newRealClock()
	}
	if *logPath == "" {
		*logPath = filepath.Join(root, fmt.Sprintf("oplog-seed%d.jsonl", *seed))
	}
	f, err := os.Create(*logPath)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg.Out = f
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	l, err := Run(ctx, cfg)
	fmt.Fprintf(stdout, "ops=%d hash=%s log=%s\n", len(l.Ops), l.Hash(), *logPath)
	return err
}
