package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/events"
)

func TestToolInputUnrenderable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input map[string]interface{}
		want  bool
	}{
		{"plain command", map[string]interface{}{"command": "git status"}, false},
		{"multi-line command with tabs", map[string]interface{}{"command": "a\n\tb\r\n"}, false},
		{"empty input hides nothing", map[string]interface{}{}, false},
		{"nil input hides nothing", nil, false},
		{"command with a NUL", map[string]interface{}{"command": "echo \x00"}, true},
		{"command with an escape", map[string]interface{}{"command": "echo \x1b[31m"}, true},
		{"invalid UTF-8", map[string]interface{}{"command": "echo \xff\xfe"}, true},
		{"only structured fields", map[string]interface{}{"edits": []interface{}{map[string]interface{}{"a": 1}}}, true},
		{"structured plus a clean string", map[string]interface{}{"file_path": "/a", "edits": []interface{}{}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toolInputUnrenderable(tt.input))
		})
	}
}

func TestBroadcastApprovalNotification_ShouldStampUnrenderable_OnlyWhenTheInputCannotBeShown(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		input map[string]interface{}
		want  bool
	}{
		{"clean", map[string]interface{}{"command": "ls"}, false},
		{"control characters", map[string]interface{}{"command": "ls \x00"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bus := events.NewEventBus(4)
			defer bus.Close()
			ctx := t.Context()
			ch, _ := bus.Subscribe(ctx)
			h := NewApprovalHandler(NewApprovalStore(""), nil, bus)

			h.broadcastApprovalNotification("sess-1", &PendingApproval{ID: "appr-1", SessionID: "sess-1", ToolName: "Bash", ToolInput: tt.input})

			ev := <-ch
			if tt.want {
				assert.Equal(t, "true", ev.NotificationMetadata["tool_input_unrenderable"])
			} else {
				assert.NotContains(t, ev.NotificationMetadata, "tool_input_unrenderable")
			}
		})
	}
}
