package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestPortSessionHistory_ClaudeToAgy(t *testing.T) {
	// Create temporary directory for home
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempHome)
	defer os.Setenv("HOME", origHome)

	uuid := "550e8400-e29b-41d4-a716-446655440000"
	workspace := "/home/test/myproject"

	// Create Claude projects directory and mock transcript using ClaudeProjectDirName
	claudeProjectDir := filepath.Join(tempHome, ".claude", "projects", ClaudeProjectDirName(workspace))
	err := os.MkdirAll(claudeProjectDir, 0700)
	if err != nil {
		t.Fatalf("failed to create claude projects dir: %v", err)
	}

	claudeLogPath := filepath.Join(claudeProjectDir, uuid+".jsonl")
	f, err := os.Create(claudeLogPath)
	if err != nil {
		t.Fatalf("failed to create claude log file: %v", err)
	}

	mockClaudeTurns := []map[string]interface{}{
		{
			"type":      "user",
			"timestamp": "2026-06-25T20:00:00Z",
			"message": map[string]interface{}{
				"role":    "user",
				"content": "hello world",
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-06-25T20:01:00Z",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "hi there",
					},
					map[string]interface{}{
						"type":  "tool_use",
						"name":  "run_command",
						"input": map[string]interface{}{"command": "ls"},
					},
				},
			},
		},
		{
			"type":      "user",
			"timestamp": "2026-06-25T20:02:00Z",
			"message": map[string]interface{}{
				"role":    "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_xxx",
						"content":     "file contents here",
					},
				},
			},
		},
	}

	for _, turn := range mockClaudeTurns {
		data, err := json.Marshal(turn)
		if err != nil {
			t.Fatalf("failed to marshal turn: %v", err)
		}
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	// Mock Instance
	inst := &Instance{
		Title: "test-session",
		Path:  workspace,
		claudeSession: &ClaudeSessionData{
			ConversationUUID: uuid,
		},
	}

	// Run porting
	ctx := context.Background()
	err = PortSessionHistory(ctx, "claude", "agy", inst)
	if err != nil {
		t.Fatalf("PortSessionHistory failed: %v", err)
	}

	// Assertions
	// 1. Check brain transcript files
	agyBrainDir := filepath.Join(tempHome, ".gemini", "antigravity-cli", "brain", uuid, ".system_generated", "logs")
	transcriptPath := filepath.Join(agyBrainDir, "transcript.jsonl")
	if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
		t.Fatalf("antigravity transcript file does not exist")
	}

	tf, err := os.Open(transcriptPath)
	if err != nil {
		t.Fatalf("failed to open transcript file: %v", err)
	}
	defer tf.Close()

	decoder := json.NewDecoder(tf)
	var steps []map[string]interface{}
	for {
		var step map[string]interface{}
		if err := decoder.Decode(&step); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to decode step: %v", err)
		}
		steps = append(steps, step)
	}

	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	if steps[0]["type"] != "USER_INPUT" || steps[0]["content"] != "hello world" {
		t.Errorf("step 0 incorrect: %v", steps[0])
	}
	if steps[1]["type"] != "PLANNER_RESPONSE" || steps[1]["content"] != "hi there" {
		t.Errorf("step 1 incorrect: %v", steps[1])
	}
	if steps[2]["type"] != "USER_INPUT" || steps[2]["content"] != "file contents here" {
		t.Errorf("step 2 incorrect: %v", steps[2])
	}

	// 2. Check history entry
	agyHistPath := filepath.Join(tempHome, ".gemini", "antigravity-cli", "history.jsonl")
	histData, err := os.ReadFile(agyHistPath)
	if err != nil {
		t.Fatalf("failed to read history file: %v", err)
	}

	var histEntry map[string]interface{}
	if err := json.Unmarshal(histData, &histEntry); err != nil {
		t.Fatalf("failed to parse history entry: %v", err)
	}

	if histEntry["conversationId"] != uuid || histEntry["workspace"] != workspace {
		t.Errorf("history entry incorrect: %v", histEntry)
	}

	// 3. Check SQLite DB
	dbPath := filepath.Join(tempHome, ".gemini", "antigravity-cli", "conversations", uuid+".db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("sqlite database file does not exist")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM steps").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query steps table: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows in steps table, got %d", count)
	}

	var stepType int
	err = db.QueryRow("SELECT step_type FROM steps WHERE idx=0").Scan(&stepType)
	if err != nil {
		t.Fatalf("failed to query step 0: %v", err)
	}
	if stepType != 14 {
		t.Errorf("expected step_type 14 for USER_INPUT, got %d", stepType)
	}
}

func TestPortSessionHistory_AgyToClaude(t *testing.T) {
	// Create temporary directory for home
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempHome)
	defer os.Setenv("HOME", origHome)

	uuid := "770e8400-e29b-41d4-a716-446655440000"
	workspace := "/home/test/myproject"

	// Create Agy history.jsonl
	agyDir := filepath.Join(tempHome, ".gemini", "antigravity-cli")
	err := os.MkdirAll(agyDir, 0700)
	if err != nil {
		t.Fatalf("failed to create agy dir: %v", err)
	}

	agyHistPath := filepath.Join(agyDir, "history.jsonl")
	hf, err := os.Create(agyHistPath)
	if err != nil {
		t.Fatalf("failed to create agy history file: %v", err)
	}

	histEntry := map[string]interface{}{
		"display":        "some prompt",
		"timestamp":      1782445102303,
		"workspace":      workspace,
		"conversationId": uuid,
	}
	histData, _ := json.Marshal(histEntry)
	hf.Write(histData)
	hf.Write([]byte("\n"))
	hf.Close()

	// Create Agy transcript
	agyBrainDir := filepath.Join(agyDir, "brain", uuid, ".system_generated", "logs")
	err = os.MkdirAll(agyBrainDir, 0700)
	if err != nil {
		t.Fatalf("failed to create agy brain dir: %v", err)
	}

	mockAgySteps := []map[string]interface{}{
		{
			"step_index": 0,
			"source":     "USER_EXPLICIT",
			"type":       "USER_INPUT",
			"status":     "DONE",
			"created_at": "2026-06-25T20:00:00Z",
			"content":    "hello agy",
		},
		{
			"step_index": 1,
			"source":     "MODEL",
			"type":       "PLANNER_RESPONSE",
			"status":     "DONE",
			"created_at": "2026-06-25T20:01:00Z",
			"content":    "hi agy response",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"name": "view_file",
					"args": map[string]interface{}{"path": "test.go"},
				},
			},
		},
	}

	transcriptPath := filepath.Join(agyBrainDir, "transcript_full.jsonl")
	tf, err := os.Create(transcriptPath)
	if err != nil {
		t.Fatalf("failed to create agy transcript file: %v", err)
	}

	for _, step := range mockAgySteps {
		data, _ := json.Marshal(step)
		tf.Write(data)
		tf.Write([]byte("\n"))
	}
	tf.Close()

	// Mock Instance
	inst := &Instance{
		Title: "test-session",
		Path:  workspace,
	}

	// Run porting
	ctx := context.Background()
	err = PortSessionHistory(ctx, "agy", "claude", inst)
	if err != nil {
		t.Fatalf("PortSessionHistory failed: %v", err)
	}

	// Assertions
	// 1. Check Claude transcript file
	claudeProjectDir := filepath.Join(tempHome, ".claude", "projects", ClaudeProjectDirName(workspace))
	claudeLogPath := filepath.Join(claudeProjectDir, uuid+".jsonl")
	if _, err := os.Stat(claudeLogPath); os.IsNotExist(err) {
		t.Fatalf("claude log file does not exist at %s", claudeLogPath)
	}

	cf, err := os.Open(claudeLogPath)
	if err != nil {
		t.Fatalf("failed to open claude log file: %v", err)
	}
	defer cf.Close()

	decoder := json.NewDecoder(cf)
	var turns []map[string]interface{}
	for {
		var turn map[string]interface{}
		if err := decoder.Decode(&turn); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to decode turn: %v", err)
		}
		turns = append(turns, turn)
	}

	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}

	if turns[0]["type"] != "user" {
		t.Errorf("turn 0 incorrect type: %v", turns[0]["type"])
	}
	if turns[1]["type"] != "assistant" {
		t.Errorf("turn 1 incorrect type: %v", turns[1]["type"])
	}
}

func TestPortSessionHistory_LiveClaude(t *testing.T) {
	// Parse actual real Claude JSONL session if present in the home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get user home directory")
	}

	liveSessionUUID := "ccf24ef9-2cbe-4777-a40d-64c507b5bc52"
	liveProjPath := "-home-tstapler--stapler-squad-workspaces-d685c4b1a423cca3-worktrees-stapler-squad-think-18bada272b0b7edf-session"
	liveLogPath := filepath.Join(home, ".claude", "projects", liveProjPath, liveSessionUUID+".jsonl")

	if _, err := os.Stat(liveLogPath); os.IsNotExist(err) {
		t.Skipf("live session log file not found at %s — skipping live integration check", liveLogPath)
	}

	// Create temp home to output agy files during porting
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempHome)
	defer os.Setenv("HOME", origHome)

	// Copy the real log file into the expected temp path
	tempProjectsDir := filepath.Join(tempHome, ".claude", "projects", liveProjPath)
	if err := os.MkdirAll(tempProjectsDir, 0700); err != nil {
		t.Fatalf("failed to create temp projects dir: %v", err)
	}
	tempLogPath := filepath.Join(tempProjectsDir, liveSessionUUID+".jsonl")

	src, err := os.Open(liveLogPath)
	if err != nil {
		t.Fatalf("failed to open source live log: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempLogPath)
	if err != nil {
		t.Fatalf("failed to create dest temp log: %v", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("failed to copy live log to temp: %v", err)
	}

	inst := &Instance{
		Title: "live-port-test",
		Path:  "/home/tstapler/Programming/stapler-squad",
		claudeSession: &ClaudeSessionData{
			ConversationUUID: liveSessionUUID,
		},
	}

	err = PortSessionHistory(context.Background(), "claude", "agy", inst)
	if err != nil {
		t.Fatalf("failed to port live session: %v", err)
	}

	// Assert SQLite database is healthy
	dbPath := filepath.Join(tempHome, ".gemini", "antigravity-cli", "conversations", liveSessionUUID+".db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("target SQLite DB was not created at %s", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open SQLite DB: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM steps").Scan(&count); err != nil {
		t.Fatalf("failed to query steps table: %v", err)
	}
	t.Logf("Successfully ported %d steps from live Claude session into Antigravity DB!", count)
	if count == 0 {
		t.Errorf("expected steps to be populated")
	}
}

func TestPortSessionHistory_LiveAgy(t *testing.T) {
	// Parse actual real Antigravity JSONL session if present in the home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get user home directory")
	}

	liveSessionUUID := "ffcbbc7d-8d41-41f5-9698-b7caa032ddc5"
	liveLogPath := filepath.Join(home, ".gemini", "antigravity-cli", "brain", liveSessionUUID, ".system_generated", "logs", "transcript.jsonl")

	if _, err := os.Stat(liveLogPath); os.IsNotExist(err) {
		t.Skipf("live session log file not found at %s — skipping live integration check", liveLogPath)
	}

	// Create temp home to output Claude files during porting
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempHome)
	defer os.Setenv("HOME", origHome)

	// Copy real history.jsonl and brain directory
	agyDir := filepath.Join(tempHome, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(filepath.Join(agyDir, "brain", liveSessionUUID, ".system_generated", "logs"), 0700); err != nil {
		t.Fatalf("failed to create temp brain dir: %v", err)
	}

	srcLog, err := os.Open(liveLogPath)
	if err != nil {
		t.Fatalf("failed to open source live agy log: %v", err)
	}
	defer srcLog.Close()

	dstLog, err := os.Create(filepath.Join(agyDir, "brain", liveSessionUUID, ".system_generated", "logs", "transcript.jsonl"))
	if err != nil {
		t.Fatalf("failed to create dest temp agy log: %v", err)
	}
	defer dstLog.Close()

	if _, err := io.Copy(dstLog, srcLog); err != nil {
		t.Fatalf("failed to copy live agy log to temp: %v", err)
	}

	// Copy history.jsonl
	srcHistPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	dstHistPath := filepath.Join(agyDir, "history.jsonl")
	srcHist, err := os.Open(srcHistPath)
	if err == nil {
		defer srcHist.Close()
		dstHist, err := os.Create(dstHistPath)
		if err == nil {
			defer dstHist.Close()
			io.Copy(dstHist, srcHist)
		}
	}

	workspace := "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-transfer_18bc8392fbd12153"
	inst := &Instance{
		Title: "live-port-test-agy",
		Path:  workspace,
	}

	err = PortSessionHistory(context.Background(), "agy", "claude", inst)
	if err != nil {
		t.Fatalf("failed to port live session: %v", err)
	}

	// Assert Claude transcript is written using ClaudeProjectDirName
	claudeProjectDir := filepath.Join(tempHome, ".claude", "projects", ClaudeProjectDirName(workspace))
	claudeLogPath := filepath.Join(claudeProjectDir, liveSessionUUID+".jsonl")

	if _, err := os.Stat(claudeLogPath); os.IsNotExist(err) {
		t.Fatalf("target Claude log was not created at %s", claudeLogPath)
	}

	cf, err := os.Open(claudeLogPath)
	if err != nil {
		t.Fatalf("failed to open Claude log file: %v", err)
	}
	defer cf.Close()

	decoder := json.NewDecoder(cf)
	var count int
	for {
		var turn map[string]interface{}
		if err := decoder.Decode(&turn); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to decode turn: %v", err)
		}
		count++
	}
	t.Logf("Successfully ported %d turns from live Antigravity session into Claude projects JSONL!", count)
	if count == 0 {
		t.Errorf("expected turns to be populated")
	}
}
