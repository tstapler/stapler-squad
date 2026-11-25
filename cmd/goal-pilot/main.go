// Command goal-pilot runs one backlog item's criteria through Claude Code /goal
// via session/goalrunner. Operator-triggered and gated by the goal_mode flag;
// it prints the outcome as JSON and exits 0 on goal_met, 1 otherwise, 2 on misuse.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/goalrunner"
)

const maxCriteriaChars = 500

func main() {
	os.Exit(run(context.Background(), os.Args[1:], config.LoadConfig().GetFeatureFlag(config.GoalModeFlag), os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, flagOn bool, stdout, stderr io.Writer) int {
	if !flagOn {
		fmt.Fprintf(stderr, "goal-pilot: feature flag %q is off\n", config.GoalModeFlag)
		return 2
	}
	fs := flag.NewFlagSet("goal-pilot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	criteria := fs.String("criteria", "", "machine-checkable completion condition (item description)")
	dir := fs.String("dir", "", "working directory (the item's worktree)")
	turns := fs.Int("max-turns", goalrunner.DefaultMaxTurns, "turn bound added to the /goal condition")
	ceiling := fs.Float64("ceiling-usd", goalrunner.DefaultCostCeilingUSD, "kill the run past this estimated spend")
	settings := fs.String("settings", "", "settings JSON passed to claude --settings")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *criteria == "" {
		fmt.Fprintln(stderr, "goal-pilot: --criteria is required")
		return 2
	}
	c := *criteria
	if r := []rune(c); len(r) > maxCriteriaChars {
		c = string(r[:maxCriteriaChars])
	}
	out := goalrunner.Run(ctx, goalrunner.Config{
		Criteria: c, Dir: *dir, MaxTurns: *turns, CostCeilingUSD: *ceiling, SettingsPath: *settings,
	})
	rec := map[string]any{"end_reason": out.EndReason, "cost_usd": out.CostUSD, "turns": out.Turns,
		"wall_seconds": out.WallTime.Round(time.Second).Seconds()}
	if out.Err != nil {
		rec["error"] = out.Err.Error()
	}
	_ = json.NewEncoder(stdout).Encode(rec)
	if out.EndReason == goalrunner.EndGoalMet {
		return 0
	}
	return 1
}
