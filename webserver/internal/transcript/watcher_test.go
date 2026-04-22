package transcript

import (
	"fmt"
	"testing"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestParseLineUserMessage(t *testing.T) {
	line := []byte(fmt.Sprintf(`{"timestamp":"2026-04-02T14:05:02.844Z","type":"event_msg","payload":{"type":"user_message","message":"%s"}}`, model.ContinueCommandText))
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if update.LastUserPromptPreview != model.ContinueCommandText {
		t.Fatalf("unexpected prompt preview: %q", update.LastUserPromptPreview)
	}
}

func TestParseLineAssistantMessage(t *testing.T) {
	line := []byte(`{"timestamp":"2026-04-02T13:24:02.920Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I’m checking the failing test output..."}]}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if update.LastAssistantMessage == "" {
		t.Fatalf("expected assistant message")
	}
}

func TestParseLineShellCommand(t *testing.T) {
	line := []byte(`{"timestamp":"2026-04-02T13:24:02.920Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"npm test\",\"workdir\":\"/repo\",\"timeout_ms\":10000}"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if update.LastBashCommand != "npm test" {
		t.Fatalf("unexpected bash command: %q", update.LastBashCommand)
	}
}

func TestParseLineFailedExecCommand(t *testing.T) {
	line := []byte(`{"timestamp":"2026-04-02T13:24:02.973Z","type":"event_msg","payload":{"type":"exec_command_end","turn_id":"turn-1","command":["/usr/bin/zsh","-lc","npm test"],"aggregated_output":"failed output","status":"failed"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if update.Error != "failed output" {
		t.Fatalf("unexpected error: %q", update.Error)
	}
	if update.TurnID != "turn-1" {
		t.Fatalf("unexpected turn id: %q", update.TurnID)
	}
}

func TestHasUsefulTranscriptUpdate(t *testing.T) {
	if hasUsefulTranscriptUpdate(model.TranscriptUpdate{}) {
		t.Fatalf("empty update should not be useful")
	}
	if !hasUsefulTranscriptUpdate(model.TranscriptUpdate{SessionID: "sess-1", LastAssistantMessage: "ok"}) {
		t.Fatalf("assistant message should be useful")
	}
}
