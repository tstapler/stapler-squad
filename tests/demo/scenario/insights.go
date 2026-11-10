package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Insights are computed server-side from the Claude JSONL transcripts the token
// store walks (session/tokens), so this scenario seeds transcript files rather
// than RPC state or a route fixture.
func insightsBundle(ref time.Time) Bundle { return Bundle{Transcripts: insightsTranscripts(ref)} }

type insightsSession struct {
	title    string
	model    string
	daysAgo  int
	hoursAgo int // offset within the day, so the last turn stays before ref
	turns    int
	baseCtx  int64   // context tokens on the first turn
	growth   int64   // extra context tokens per turn
	hitRate  float64 // share of context served from cache
	tools    []string
}

var insightsSessions = []insightsSession{
	{"storefront-dark-mode", "claude-sonnet-4-6", 0, 3, 42, 20_000, 600, 0.985, []string{"Read", "Edit", "Edit", "Write", "Bash"}},
	{"checkout-flaky-test", "claude-sonnet-4-6", 1, 5, 30, 22_000, 500, 0.80, []string{"Read", "Grep", "Bash", "Edit", "Bash"}},
	{"orders-pagination", "claude-opus-4-7", 2, 8, 26, 25_000, 800, 0.97, []string{"Read", "Edit", "Write", "Edit", "Bash"}},
	{"autoscale-tuning", "claude-sonnet-4-6", 3, 6, 22, 20_000, 500, 0.90, []string{"Read", "Grep", "Read", "Edit", "Bash"}},
	{"docs-site-cleanup", "claude-haiku-5-5", 4, 9, 14, 15_000, 400, 0.98, []string{"Read", "Write", "Edit", "Edit"}},
	{"search-index-refactor", "claude-opus-4-7", 5, 7, 34, 40_000, 2_500, 0.975, []string{"Read", "Edit", "Edit", "Grep", "Bash"}},
	{"payments-retries", "claude-sonnet-4-6", 5, 14, 24, 22_000, 600, 0.96, []string{"Read", "Edit", "Bash", "Edit"}},
	{"inventory-sync-api", "claude-sonnet-4-6", 6, 11, 36, 20_000, 700, 0.98, []string{"Read", "Write", "Edit", "Bash", "Edit"}},
}

func (s insightsSession) transcript(n int, ref time.Time) TranscriptFile {
	const turnGap = 25 * time.Second
	start := ago(ref, time.Duration(s.daysAgo*24+s.hoursAgo)*time.Hour)
	convID := fmt.Sprintf("00000000-0000-4000-8000-%012d", n+1)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	parent := any(nil)
	var last time.Time
	for i := 0; i < s.turns; i++ {
		at := start.Add(time.Duration(i) * turnGap)
		last = at
		ctx := s.baseCtx + int64(i)*s.growth
		cached := int64(float64(ctx) * s.hitRate)
		tool := s.tools[i%len(s.tools)]
		uuid := fmt.Sprintf("%s-a%03d", convID, i)
		if i > 0 && i%6 == 0 {
			userID := fmt.Sprintf("%s-u%03d", convID, i)
			_ = enc.Encode(map[string]any{
				"parentUuid": parent, "type": "user", "uuid": userID, "sessionId": convID,
				"timestamp": at.Add(-time.Second).UTC().Format(time.RFC3339Nano),
				"message":   map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "Looks good, keep going."}}},
			})
			parent = userID
		}
		_ = enc.Encode(map[string]any{
			"parentUuid": parent, "type": "assistant", "uuid": uuid, "sessionId": convID,
			"timestamp": at.UTC().Format(time.RFC3339Nano),
			"message": map[string]any{
				"id": "msg_" + uuid, "type": "message", "role": "assistant", "model": s.model, "stop_reason": "tool_use",
				"content": []map[string]any{{"type": "tool_use", "name": tool, "input": map[string]any{"file_path": fmt.Sprintf("src/module%d.ts", i%7)}}},
				"usage": map[string]any{
					"input_tokens":                ctx - cached,
					"output_tokens":               400 + int64((i*37)%11)*120,
					"cache_creation_input_tokens": 1_500 + int64((i*13)%5)*400,
					"cache_read_input_tokens":     cached,
				},
			},
		})
		parent = uuid
	}
	return TranscriptFile{
		ProjectDir: "-demo-shop-worktrees-" + s.title + "-" + fmt.Sprintf("%016x", 0xa11ce0000000+n),
		Name:       convID + ".jsonl",
		Body:       buf.String(),
		ModTime:    last,
	}
}

func insightsTranscripts(ref time.Time) []TranscriptFile {
	out := make([]TranscriptFile, len(insightsSessions))
	for i, s := range insightsSessions {
		out[i] = s.transcript(i, ref)
	}
	return out
}

// writeTranscripts writes transcripts where an isolated instance's token store
// looks (config.ResolveClaudeHistoryDir under --test-dir).
func writeTranscripts(dir string, files []TranscriptFile) error {
	for _, f := range files {
		projectDir := filepath.Join(dir, "claude-projects", f.ProjectDir)
		if err := os.MkdirAll(projectDir, 0o750); err != nil {
			return fmt.Errorf("seed transcripts: %w", err)
		}
		path := filepath.Join(projectDir, f.Name)
		if err := os.WriteFile(path, []byte(f.Body), 0o600); err != nil {
			return fmt.Errorf("seed transcripts: %w", err)
		}
		if err := os.Chtimes(path, f.ModTime, f.ModTime); err != nil {
			return fmt.Errorf("seed transcripts: %w", err)
		}
	}
	return nil
}
