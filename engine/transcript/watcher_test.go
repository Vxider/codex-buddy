package transcript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vxider/agent-buddy/internal/model"
)

func TestParseLineSlashGoalClear(t *testing.T) {
	line := []byte(`{"timestamp":"2026-06-08T13:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"/goal clear"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if !update.GoalUpdated {
		t.Fatalf("expected goal update")
	}
	if update.GoalState != "" {
		t.Fatalf("expected empty goal state, got %q", update.GoalState)
	}
	if update.GoalSummary != "" {
		t.Fatalf("expected empty goal summary, got %q", update.GoalSummary)
	}
	if !update.GoalUpdatedAt.IsZero() {
		t.Fatalf("expected empty goal updated time after clear, got %s", update.GoalUpdatedAt)
	}
}

func TestRecoverSessionClearsEarlierGoal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-06-08T13-00-00-sess-1.jsonl")
	content := "" +
		`{"timestamp":"2026-06-08T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"create_goal","arguments":"{\"objective\":\"ship the fix\"}"}}` + "\n" +
		`{"timestamp":"2026-06-08T13:05:00Z","type":"event_msg","payload":{"type":"user_message","message":"/goal clear"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	update, err := RecoverSession(path, "sess-1")
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if !update.GoalUpdated {
		t.Fatalf("expected recovered clear update")
	}
	if update.GoalState != "" {
		t.Fatalf("expected recovered goal state to be empty, got %q", update.GoalState)
	}
	if update.GoalSummary != "" {
		t.Fatalf("expected recovered goal summary to be empty, got %q", update.GoalSummary)
	}
}

func TestParseLineGoalToolCallMarksGoalUpdated(t *testing.T) {
	line := []byte(`{"timestamp":"2026-06-08T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"update_goal","arguments":"{\"status\":\"complete\"}"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if !update.GoalUpdated {
		t.Fatalf("expected goal update")
	}
	if update.GoalState != model.GoalStateAchieved {
		t.Fatalf("unexpected goal state: %q", update.GoalState)
	}
}

func TestParseLineGoalToolCallAcceptsObjectArguments(t *testing.T) {
	line := []byte(`{"timestamp":"2026-06-08T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"create_goal","arguments":{"objective":"ship the next fix"}}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if !update.GoalUpdated {
		t.Fatalf("expected goal update")
	}
	if update.GoalState != model.GoalStateInProgress {
		t.Fatalf("unexpected goal state: %q", update.GoalState)
	}
	if update.GoalSummary != "ship the next fix" {
		t.Fatalf("unexpected goal summary: %q", update.GoalSummary)
	}
}

func TestRecoverSessionCreateGoalAfterCompleteReturnsInProgress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-06-08T13-00-00-sess-1.jsonl")
	content := "" +
		`{"timestamp":"2026-06-08T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"create_goal","arguments":{"objective":"ship the first fix"}}}` + "\n" +
		`{"timestamp":"2026-06-08T13:05:00Z","type":"response_item","payload":{"type":"function_call","name":"update_goal","arguments":{"status":"complete"}}}` + "\n" +
		`{"timestamp":"2026-06-08T13:10:00Z","type":"response_item","payload":{"type":"function_call","name":"create_goal","arguments":{"objective":"ship the next fix"}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	update, err := RecoverSession(path, "sess-1")
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if !update.GoalUpdated {
		t.Fatalf("expected recovered create update")
	}
	if update.GoalState != model.GoalStateInProgress {
		t.Fatalf("expected recovered goal state in progress, got %q", update.GoalState)
	}
	if update.GoalSummary != "ship the next fix" {
		t.Fatalf("unexpected recovered goal summary: %q", update.GoalSummary)
	}
}

func TestRecoverSessionUserPromptAfterCompleteClearsGoal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-06-08T13-00-00-sess-1.jsonl")
	content := "" +
		`{"timestamp":"2026-06-08T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"create_goal","arguments":{"objective":"ship the fix"}}}` + "\n" +
		`{"timestamp":"2026-06-08T13:05:00Z","type":"response_item","payload":{"type":"function_call","name":"update_goal","arguments":{"status":"complete"}}}` + "\n" +
		`{"timestamp":"2026-06-08T13:10:00Z","type":"event_msg","payload":{"type":"user_message","message":"next question"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	update, err := RecoverSession(path, "sess-1")
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if !update.GoalUpdated {
		t.Fatalf("expected recovered goal update")
	}
	if update.GoalState != "" {
		t.Fatalf("expected recovered goal state to be empty, got %q", update.GoalState)
	}
	if update.GoalSummary != "" {
		t.Fatalf("expected recovered goal summary to be empty, got %q", update.GoalSummary)
	}
}
