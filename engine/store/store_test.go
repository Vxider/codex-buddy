package store

import (
	"io"
	"log"
	"testing"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestTranscriptCompletionClearsAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID:            "sess-push",
			TmuxPane:             "%1",
			LastAssistantMessage: "如果你愿意，我下一步可以继续帮你确认 push 状态。",
		},
	})

	session, ok := st.Session("sess-push")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected attention session, got %s", session.State)
	}

	snapshot := st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:            "sess-push",
		UpdatedAt:            now.Add(time.Second),
		LastAssistantMessage: "已 push 到 `origin/main`。\n\n远端更新：`bb20943..536724b main -> main`。",
	})
	if snapshot.OverallState != model.StateIdle {
		t.Fatalf("expected idle after completion transcript, got %s", snapshot.OverallState)
	}

	session, ok = st.Session("sess-push")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateIdle {
		t.Fatalf("expected completion transcript to clear attention, got %s", session.State)
	}
}
