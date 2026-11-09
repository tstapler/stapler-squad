package artifacts

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// pendingCall is a Bash tool_use waiting for its tool_result.
type pendingCall struct {
	seenAt     time.Time
	cmd        *CommandArtifact // nil when the command matches no known pattern
	createPR   bool
	head, base string
}

// addLine extracts artifacts from one complete transcript line; malformed lines
// are skipped. Findings come from tool_result text (never assistant prose, which
// is full of doc links) and from Bash tool_use inputs, which are paired with
// their result by tool_use id so a record can say what the command produced.
func (f *scanFindings) addLine(line []byte) {
	var entry artifactEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}
	if entry.Type != "user" && entry.Type != "assistant" {
		return
	}
	var msg artifactMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}
	for _, c := range msg.Content {
		switch {
		case entry.Type == "user" && c.Type == "tool_result":
			f.addToolResult(c, entry.Timestamp)
		case entry.Type == "assistant" && c.Type == "tool_use" && c.Name == "Bash":
			f.addBashUse(c, entry.Timestamp)
		case entry.Type == "assistant" && c.Type == "tool_use" && fileWriteTools[c.Name]:
			f.addFileWrite(c, entry.Timestamp)
		}
	}
}

func (f *scanFindings) addBashUse(c artifactContentBlock, ts time.Time) {
	var inp bashInput
	if err := json.Unmarshal(c.Input, &inp); err != nil || inp.Command == "" {
		return
	}
	cmd := ExtractFromBashCommand(inp.Command)
	f.addCommandRecords(inp.Command, ts)
	call := pendingCall{seenAt: ts, cmd: cmd}
	if rest, ok := ghSubcommand(shellFields(inp.Command), "pr", "create"); ok {
		call.createPR = true
		call.head = flagValue(rest, "--head", "-H")
		call.base = flagValue(rest, "--base", "-B")
	}
	if cmd != nil {
		f.commands = append(f.commands, *cmd) // v1 list: kept so an older binary reads the blob
	}
	if c.ID != "" && (cmd != nil || call.createPR) {
		if f.pending == nil {
			f.pending = map[string]pendingCall{}
		}
		f.pending[c.ID] = call
	}
}

func (f *scanFindings) addToolResult(c artifactContentBlock, ts time.Time) {
	text := extractToolResultText(c.Content)
	prs, shas, urls := ExtractFromToolResult(text)
	f.prURLs = append(f.prURLs, prs...)
	f.commitSHAs = append(f.commitSHAs, shas...)
	f.externalURLs = append(f.externalURLs, urls...)

	call, paired := f.pending[c.ToolUseID]
	if paired {
		delete(f.pending, c.ToolUseID)
	}
	f.addPRRecords(prs, call, paired, ts)
	f.addResultRecords(text, ts)
	if paired && call.cmd != nil {
		f.records = append(f.records, commandRecord(call, text))
	}
}

// addPRRecords records each PR URL of a result. Only the first URL printed by a
// paired `gh pr create` is authored; every other sighting is a mention.
func (f *scanFindings) addPRRecords(prURLs []string, call pendingCall, paired bool, ts time.Time) {
	for i, u := range prURLs {
		rec, ok := parsePRURL(u)
		if !ok {
			continue
		}
		rec.Source, rec.FirstSeenAt = PRSourceMentioned, ts
		if paired && call.createPR && i == 0 {
			rec.Source, rec.Branch, rec.Base = PRSourceCreated, call.head, call.base
		}
		f.records = append(f.records, rec)
	}
}

func commandRecord(call pendingCall, result string) CommandRecord {
	return CommandRecord{
		Type: call.cmd.Type, Command: call.cmd.Command, Detail: call.cmd.Detail,
		Result: truncateBytes(Redact(strings.TrimSpace(result)), MaxResultBytes), FirstSeenAt: call.seenAt,
	}
}

// finish flushes calls whose result never arrived in this pass as result-less
// command records. Call it once after the last line.
func (f *scanFindings) finish() {
	unanswered := make([]pendingCall, 0, len(f.pending))
	for _, call := range f.pending {
		if call.cmd != nil {
			unanswered = append(unanswered, call)
		}
	}
	sort.Slice(unanswered, func(i, j int) bool { return unanswered[i].seenAt.Before(unanswered[j].seenAt) })
	for _, call := range unanswered {
		f.records = append(f.records, commandRecord(call, ""))
	}
	f.pending = nil
	f.redactFindings()
}
