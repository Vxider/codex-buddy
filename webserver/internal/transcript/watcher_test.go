package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vxider/agent-buddy/internal/model"
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

func TestParseLineCodexErrorEvent(t *testing.T) {
	line := []byte(`{"timestamp":"2026-04-02T13:24:02.973Z","type":"event_msg","payload":{"type":"error","message":"HTTP 401 Unauthorized"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if update.Error != "HTTP 401 Unauthorized" {
		t.Fatalf("unexpected Codex error: %q", update.Error)
	}
}

func TestParseLineTaskCompleteNestedError(t *testing.T) {
	line := []byte(`{"timestamp":"2026-08-07T08:24:12.214Z","type":"event_msg","payload":{"type":"task_complete","error":{"message":"unexpected status 503 Service Unavailable: auth_unavailable"}}}`)
	update, ok := parseLine("sess-1", line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if update.Error != "unexpected status 503 Service Unavailable: auth_unavailable" {
		t.Fatalf("unexpected task completion error: %q", update.Error)
	}
}

func TestParseLineTaskCompleteUsageLimitError(t *testing.T) {
	line := []byte(`{"timestamp":"2026-07-24T03:15:16.529Z","type":"event_msg","payload":{"type":"task_complete","last_agent_message":null,"error":{"message":"You've hit your usage limit. To get more access now, send a request to your admin or try again at Aug 23rd, 2026 3:35 AM.","codex_error_info":"usage_limit_exceeded"}}}`)
	update, ok := parseLine("sess-usage-limit", line)
	if !ok || !update.TaskCompleted {
		t.Fatalf("expected usage-limit task completion, got ok=%v update=%#v", ok, update)
	}
	if update.Error != "You've hit your usage limit. To get more access now, send a request to your admin or try again at Aug 23rd, 2026 3:35 AM." {
		t.Fatalf("unexpected usage-limit error: %q", update.Error)
	}
	if !model.IsCodexInterruptionText(update.Error) {
		t.Fatalf("expected usage-limit error to be classified as Codex interruption: %q", update.Error)
	}
}

func TestParseLineSuccessfulTaskCompleteDoesNotCreateError(t *testing.T) {
	line := []byte(`{"timestamp":"2026-08-07T08:24:12.214Z","type":"event_msg","payload":{"type":"task_complete"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok || !update.TaskCompleted || update.Error != "" {
		t.Fatalf("expected successful task completion marker, got ok=%v update=%#v", ok, update)
	}
}

func TestParseLineTaskStarted(t *testing.T) {
	line := []byte(`{"timestamp":"2026-08-12T14:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-retry"}}`)
	update, ok := parseLine("sess-1", line)
	if !ok || !update.TaskStarted || update.TurnID != "turn-retry" {
		t.Fatalf("expected task start marker, got ok=%v update=%#v", ok, update)
	}
}

func TestRecoverSessionTaskStartedClearsEarlierInterruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-08-12T14-00-00-sess-1.jsonl")
	content := "" +
		`{"timestamp":"2026-08-12T14:00:00Z","type":"event_msg","payload":{"type":"task_complete","error":{"message":"HTTP 503 Service Unavailable"}}}` + "\n" +
		`{"timestamp":"2026-08-12T14:01:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-retry"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	update, err := RecoverSession(path, "sess-1")
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if !update.TaskStarted || update.TaskCompleted || update.Error != "" {
		t.Fatalf("expected active retry to replace earlier interruption, got %#v", update)
	}
}

func TestRecoverSessionHandlesLargeTranscriptLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-08-07T08-24-12-sess-1.jsonl")
	content := "{" +
		`"timestamp":"2026-08-07T08:24:00Z","type":"unknown","padding":"` +
		strings.Repeat("x", 2*1024*1024) +
		`"}` + "\n" +
		`{"timestamp":"2026-08-07T08:24:12.214Z","type":"event_msg","payload":{"type":"task_complete","error":{"message":"HTTP 503 Service Unavailable"}}}` + "\n" +
		`{"timestamp":"2026-08-07T08:24:13Z","type":"turn_context","payload":{"turn_id":"turn-2"}}` + "\n" +
		`{"timestamp":"2026-08-07T08:24:14Z","type":"event_msg","payload":{"type":"user_message","message":"resume after failure"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	update, err := RecoverSession(path, "sess-1")
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if update.Error != "HTTP 503 Service Unavailable" {
		t.Fatalf("unexpected recovered error: %q", update.Error)
	}
}

func TestParseLineUserAbortDoesNotCreateError(t *testing.T) {
	line := []byte(`{"timestamp":"2026-04-02T13:24:02.973Z","type":"event_msg","payload":{"type":"turn_aborted","reason":"user"}}`)
	update, ok := parseLine("sess-1", line)
	if ok || update.Error != "" {
		t.Fatalf("expected user abort to be ignored, got ok=%v update=%#v", ok, update)
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

func TestRecoverSessionRestoresLatestTranscriptContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-04-24T21-56-57-sess-1.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-24T21:56:57Z\",\"type\":\"turn_context\",\"payload\":{\"turn_id\":\"turn-1\",\"cwd\":\"/repo/app\",\"model\":\"gpt-5.4\"}}\n" +
		fmt.Sprintf("{\"timestamp\":\"2026-04-24T21:57:00Z\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"%s\"}}\n", model.ContinueCommandText) +
		"{\"timestamp\":\"2026-04-24T21:57:02Z\",\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Need confirmation before overwriting files\"}]}}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	update, err := RecoverSession(path, "sess-1")
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if update.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %q", update.SessionID)
	}
	if update.CWD != "/repo/app" {
		t.Fatalf("unexpected cwd: %q", update.CWD)
	}
	if update.Model != "gpt-5.4" {
		t.Fatalf("unexpected model: %q", update.Model)
	}
	if update.LastUserPromptPreview != model.ContinueCommandText {
		t.Fatalf("unexpected prompt preview: %q", update.LastUserPromptPreview)
	}
	if update.LastAssistantMessage != "Need confirmation before overwriting files" {
		t.Fatalf("unexpected assistant message: %q", update.LastAssistantMessage)
	}
	if update.UpdatedAt.Before(time.Date(2026, 4, 24, 21, 57, 2, 0, time.UTC)) {
		t.Fatalf("expected updated time to reflect latest entry, got %s", update.UpdatedAt)
	}
}

func TestRecoverSessionClearsEarlierGoal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-06-08T13-00-00-sess-1.jsonl")
	content := "" +
		`{"timestamp":"2026-06-08T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"create_goal","arguments":"{\"objective\":\"ship the fix\"}"}}` + "\n" +
		`{"timestamp":"2026-06-08T13:05:00Z","type":"response_item","payload":{"type":"function_call","name":"update_goal","arguments":{"status":"complete"}}}` + "\n" +
		`{"timestamp":"2026-06-08T13:10:00Z","type":"event_msg","payload":{"type":"user_message","message":"/goal clear"}}` + "\n"
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

func TestMergeUpdateClearsHistoricalErrorAfterLaterProgress(t *testing.T) {
	dst := model.TranscriptUpdate{
		SessionID: "sess-1",
		Error:     "old error",
	}

	mergeUpdate(&dst, model.TranscriptUpdate{
		SessionID:            "sess-1",
		LastAssistantMessage: "later assistant output",
	})

	if dst.Error != "" {
		t.Fatalf("expected later non-error progress to clear stale error, got %q", dst.Error)
	}
}
