package store

import (
	"io"
	"log"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestApplyIngestTransitions(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-1",
			Prompt:    "fix failing tests",
		},
	})
	if snapshot.OverallState != model.StateRunning {
		t.Fatalf("expected running, got %s", snapshot.OverallState)
	}

	snapshot = st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-1",
			LastAssistantMessage: "done",
		},
	})
	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected attention after stop, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-1")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected attention to remain active, got %s", session.State)
	}
}

func TestApplyTranscriptUpdateEnrichesSession(t *testing.T) {
	st := New(50*time.Millisecond, 50*time.Millisecond, log.New(io.Discard, "", 0))
	update := model.TranscriptUpdate{
		SessionID:             "sess-2",
		TurnID:                "turn-1",
		CWD:                   "/repo",
		Model:                 "gpt-5.4",
		LastUserPromptPreview: "build status page",
		LastAssistantMessage:  "looking into it",
		LastBashCommand:       "npm test",
		UpdatedAt:             time.Now().UTC(),
	}

	st.ApplyTranscriptUpdate(update)
	session, ok := st.Session("sess-2")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.LastBashCommand != "npm test" {
		t.Fatalf("unexpected bash command: %q", session.LastBashCommand)
	}
	if session.CWD != "/repo" {
		t.Fatalf("unexpected cwd: %q", session.CWD)
	}
}

func TestRunningDoesNotFallBackToIdleByDefault(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-3",
		},
	})

	time.Sleep(30 * time.Millisecond)
	session, ok := st.Session("sess-3")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateRunning {
		t.Fatalf("expected running session to remain running, got %s", session.State)
	}
}

func TestAttentionCanStillDecayWhenConfigured(t *testing.T) {
	st := New(20*time.Millisecond, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-4",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-4",
			LastAssistantMessage: "done",
		},
	})

	time.Sleep(30 * time.Millisecond)
	session, ok := st.Session("sess-4")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateIdle {
		t.Fatalf("expected configured attention timeout to decay to idle, got %s", session.State)
	}
}

func TestRunningCanStillFallBackWhenConfigured(t *testing.T) {
	st := New(0, 20*time.Millisecond, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-5",
		},
	})

	time.Sleep(30 * time.Millisecond)
	session, ok := st.Session("sess-5")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateIdle {
		t.Fatalf("expected configured running fallback to decay to idle, got %s", session.State)
	}
}

func TestPreviewKeepsUTF8ValidForMultibyteText(t *testing.T) {
	input := strings.Repeat("emoji🙂test", 100)
	output := preview(input)

	if !utf8.ValidString(output) {
		t.Fatalf("expected utf8-valid preview, got %q", output)
	}
	if !strings.HasSuffix(output, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis, got %q", output)
	}
	if strings.ContainsRune(output, '\uFFFD') {
		t.Fatalf("expected preview without replacement rune, got %q", output)
	}
}

func TestPreviewAssistantPrefersTailContext(t *testing.T) {
	input := strings.Repeat("setup details ", 20) + "\n\nApproval required: overwrite package-lock.json and continue."
	output := previewAssistant(input)

	if !strings.Contains(output, "Approval required") {
		t.Fatalf("expected assistant preview to keep the tail approval text, got %q", output)
	}
	if strings.Contains(output, "setup details setup details setup details setup details") {
		t.Fatalf("expected assistant preview to avoid the long leading setup text, got %q", output)
	}
}
