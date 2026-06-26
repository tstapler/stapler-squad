package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tstapler/stapler-squad/log"
)

// PortSessionHistory translates and syncs history between Claude Code and Antigravity CLI.
func PortSessionHistory(ctx context.Context, oldProgram, newProgram string, i *Instance) error {
	isOldClaude := strings.Contains(oldProgram, "claude")
	isNewClaude := strings.Contains(newProgram, "claude")
	isOldAgy := strings.Contains(oldProgram, "agy") || strings.Contains(oldProgram, "antigravity")
	isNewAgy := strings.Contains(newProgram, "agy") || strings.Contains(newProgram, "antigravity")

	if isOldClaude && isNewAgy {
		return portClaudeToAgy(i)
	} else if isOldAgy && isNewClaude {
		return portAgyToClaude(i)
	}

	return nil
}

func portClaudeToAgy(i *Instance) error {
	// 1. Get Claude Session ID
	uuid := i.GetClaudeConversationUUID()
	if uuid == "" {
		// Try to extract it
		i.tryExtractConversationUUID()
		uuid = i.GetClaudeConversationUUID()
	}
	if uuid == "" {
		log.Info("no claude conversation UUID found to port", "session", i.Title)
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// 2. Find Claude JSONL file
	claudeProjectsDir := filepath.Join(home, ".claude", "projects")
	var claudeLogPath string
	err = filepath.Walk(claudeProjectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == uuid+".jsonl" {
			claudeLogPath = path
			return fmt.Errorf("found") // Use error to break early
		}
		return nil
	})

	if claudeLogPath == "" {
		log.Warn("could not find Claude transcript file to port", "uuid", uuid)
		return nil
	}

	// 3. Parse Claude JSONL and translate turns
	file, err := os.Open(claudeLogPath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	type ClaudeTurn struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		} `json:"message"`
	}

	type AgyStep struct {
		StepIndex int           `json:"step_index"`
		Source    string        `json:"source"`
		Type      string        `json:"type"`
		Status    string        `json:"status"`
		CreatedAt string        `json:"created_at"`
		Content   string        `json:"content,omitempty"`
		ToolCalls []interface{} `json:"tool_calls,omitempty"`
	}

	var agySteps []AgyStep
	stepIdx := 0

	for {
		var turn ClaudeTurn
		if err := decoder.Decode(&turn); err == io.EOF {
			break
		} else if err != nil {
			continue
		}

		if turn.Type == "user" {
			contentStr := ""
			if s, ok := turn.Message.Content.(string); ok {
				contentStr = s
			}
			agySteps = append(agySteps, AgyStep{
				StepIndex: stepIdx,
				Source:    "USER_EXPLICIT",
				Type:      "USER_INPUT",
				Status:    "DONE",
				CreatedAt: turn.Timestamp,
				Content:   contentStr,
			})
			stepIdx++
		} else if turn.Type == "assistant" {
			textStr := ""
			var toolCalls []interface{}

			if contentList, ok := turn.Message.Content.([]interface{}); ok {
				for _, c := range contentList {
					if cMap, ok := c.(map[string]interface{}); ok {
						cType := cMap["type"]
						if cType == "text" {
							if txt, ok := cMap["text"].(string); ok {
								textStr = txt
							}
						} else if cType == "tool_use" {
							toolCalls = append(toolCalls, map[string]interface{}{
								"name": cMap["name"],
								"args": cMap["input"],
							})
						}
					}
				}
			}

			agySteps = append(agySteps, AgyStep{
				StepIndex: stepIdx,
				Source:    "MODEL",
				Type:      "PLANNER_RESPONSE",
				Status:    "DONE",
				CreatedAt: turn.Timestamp,
				Content:   textStr,
				ToolCalls: toolCalls,
			})
			stepIdx++
		}
	}

	// 4. Create Antigravity brain dir and write transcript files
	agyBrainDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", uuid, ".system_generated", "logs")
	if err := os.MkdirAll(agyBrainDir, 0700); err != nil {
		return err
	}

	transcriptPath := filepath.Join(agyBrainDir, "transcript.jsonl")
	transcriptFullPath := filepath.Join(agyBrainDir, "transcript_full.jsonl")

	for _, p := range []string{transcriptPath, transcriptFullPath} {
		f, err := os.Create(p)
		if err != nil {
			return err
		}
		for _, step := range agySteps {
			data, err := json.Marshal(step)
			if err != nil {
				f.Close()
				return err
			}
			f.Write(data)
			f.Write([]byte("\n"))
		}
		f.Close()
	}

	// 5. Append history entry
	agyHistoryPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	historyEntry := map[string]interface{}{
		"display":        fmt.Sprintf("Ported from Claude: %s", i.Title),
		"timestamp":      time.Now().UnixNano() / int64(time.Millisecond),
		"workspace":      i.GetWorkingDirectory(),
		"conversationId": uuid,
	}
	historyData, err := json.Marshal(historyEntry)
	if err == nil {
		f, err := os.OpenFile(agyHistoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.Write(historyData)
			f.Write([]byte("\n"))
			f.Close()
		}
	}

	// 6. Create SQLite DB
	dbPath := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", uuid+".db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create tables
	queries := []string{
		`CREATE TABLE IF NOT EXISTS trajectory_meta (
			trajectory_id TEXT PRIMARY KEY,
			cascade_id TEXT,
			trajectory_type INTEGER,
			source INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS steps (
			idx INTEGER PRIMARY KEY,
			step_type INTEGER DEFAULT 0,
			status INTEGER DEFAULT 0,
			has_subtrajectory INTEGER DEFAULT 0,
			metadata BLOB,
			error_details BLOB,
			permissions BLOB,
			task_details BLOB,
			render_info BLOB,
			step_payload BLOB,
			step_format INTEGER DEFAULT 0
		);`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	// Insert meta
	if _, err := db.Exec(`INSERT OR REPLACE INTO trajectory_meta (trajectory_id, cascade_id, trajectory_type, source) VALUES (?, ?, 0, 0);`, uuid, uuid); err != nil {
		return err
	}

	// Insert steps (simple placeholder steps so SQLite is structurally valid)
	for _, step := range agySteps {
		stepType := 15 // PLANNER_RESPONSE
		if step.Type == "USER_INPUT" {
			stepType = 14
		}
		if _, err := db.Exec(`INSERT OR REPLACE INTO steps (idx, step_type, status, has_subtrajectory) VALUES (?, ?, 3, 0);`, step.StepIndex, stepType); err != nil {
			return err
		}
	}

	log.Info("ported session history from Claude to Antigravity", "uuid", uuid, "session", i.Title)
	return nil
}

func portAgyToClaude(i *Instance) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// 1. Locate Agy Session ID
	// Since we are running in a workspace/worktree, let's search ~/.gemini/antigravity-cli/history.jsonl
	// for the workspace and retrieve the most recent conversationId.
	agyHistoryPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	if _, err := os.Stat(agyHistoryPath); os.IsNotExist(err) {
		log.Info("no antigravity history file found to port from", "session", i.Title)
		return nil
	}

	workspace := i.GetWorkingDirectory()
	if workspace == "" {
		return fmt.Errorf("workspace path empty")
	}

	// Read history.jsonl
	data, err := os.ReadFile(agyHistoryPath)
	if err != nil {
		return err
	}

	var uuid string
	lines := strings.Split(string(data), "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			if entry["workspace"] == workspace {
				if convID, ok := entry["conversationId"].(string); ok && convID != "" {
					uuid = convID
					break
				}
			}
		}
	}

	if uuid == "" {
		log.Info("no matching antigravity session found in history to port from", "workspace", workspace)
		return nil
	}

	// 2. Open Antigravity JSONL
	agyLogPath := filepath.Join(home, ".gemini", "antigravity-cli", "brain", uuid, ".system_generated", "logs", "transcript_full.jsonl")
	if _, err := os.Stat(agyLogPath); os.IsNotExist(err) {
		// try non-full fallback
		agyLogPath = filepath.Join(home, ".gemini", "antigravity-cli", "brain", uuid, ".system_generated", "logs", "transcript.jsonl")
		if _, err := os.Stat(agyLogPath); os.IsNotExist(err) {
			log.Warn("antigravity log file not found", "uuid", uuid)
			return nil
		}
	}

	file, err := os.Open(agyLogPath)
	if err != nil {
		return err
	}
	defer file.Close()

	type AgyStep struct {
		StepIndex int    `json:"step_index"`
		Source    string `json:"source"`
		Type      string `json:"type"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		Content   string `json:"content"`
		ToolCalls []struct {
			Name string                 `json:"name"`
			Args map[string]interface{} `json:"args"`
		} `json:"tool_calls"`
	}

	decoder := json.NewDecoder(file)
	var claudeTurns []map[string]interface{}
	parentUUID := ""

	for {
		var step AgyStep
		if err := decoder.Decode(&step); err == io.EOF {
			break
		} else if err != nil {
			continue
		}

		turnUUID := fmt.Sprintf("%s-%d", uuid, step.StepIndex)

		if step.Type == "USER_INPUT" {
			claudeTurns = append(claudeTurns, map[string]interface{}{
				"parentUuid":  nilIfEmpty(parentUUID),
				"isSidechain": false,
				"type":        "user",
				"message": map[string]interface{}{
					"role":    "user",
					"content": step.Content,
				},
				"uuid":      turnUUID,
				"timestamp": step.CreatedAt,
				"sessionId": uuid,
				"cwd":       workspace,
			})
			parentUUID = turnUUID
		} else if step.Type == "PLANNER_RESPONSE" {
			var content []map[string]interface{}
			if step.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": step.Content,
				})
			}
			for _, tc := range step.ToolCalls {
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"name":  tc.Name,
					"input": tc.Args,
				})
			}

			claudeTurns = append(claudeTurns, map[string]interface{}{
				"parentUuid":  nilIfEmpty(parentUUID),
				"isSidechain": false,
				"type":        "assistant",
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"uuid":      turnUUID,
				"timestamp": step.CreatedAt,
				"sessionId": uuid,
				"cwd":       workspace,
			})
			parentUUID = turnUUID
		}
	}

	// 3. Write Claude project transcript
	sanitizedPath := sanitizeProjectCwd(workspace)
	claudeProjectDir := filepath.Join(home, ".claude", "projects", sanitizedPath+"-session")
	if err := os.MkdirAll(claudeProjectDir, 0700); err != nil {
		return err
	}

	claudeLogPath := filepath.Join(claudeProjectDir, uuid+".jsonl")
	f, err := os.Create(claudeLogPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, turn := range claudeTurns {
		data, err := json.Marshal(turn)
		if err != nil {
			return err
		}
		f.Write(data)
		f.Write([]byte("\n"))
	}

	// 4. Append to Claude history.jsonl
	claudeHistoryPath := filepath.Join(home, ".claude", "history.jsonl")
	historyEntry := map[string]interface{}{
		"display":        fmt.Sprintf("Ported from Antigravity: %s", i.Title),
		"pastedContents": map[string]interface{}{},
		"timestamp":      time.Now().UnixNano() / int64(time.Millisecond),
		"project":        workspace,
		"sessionId":      uuid,
	}
	historyData, err := json.Marshal(historyEntry)
	if err == nil {
		hf, err := os.OpenFile(claudeHistoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			hf.Write(historyData)
			hf.Write([]byte("\n"))
			hf.Close()
		}
	}

	// 5. Update the instance's Claude session data so it resumes
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = uuid
	i.claudeSession.ProjectName = i.Title
	i.claudeSession.LastAttached = time.Now()
	if i.claudeSession.Metadata == nil {
		i.claudeSession.Metadata = make(map[string]string)
	}
	i.claudeSession.Metadata["working_dir"] = workspace

	log.Info("ported session history from Antigravity to Claude", "uuid", uuid, "session", i.Title)
	return nil
}

func sanitizeProjectCwd(workspace string) string {
	var sb strings.Builder
	for _, r := range workspace {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return strings.Trim(sb.String(), "-")
}
