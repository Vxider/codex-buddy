package store

import (
	"io"
	"log"
	"strings"
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

func TestPreToolUseRequireEscalatedBecomesApprovalAttention(t *testing.T) {
	st := New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "pre-tool-use",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-approval",
			ToolName:  "Bash",
			TmuxPane:  "%11",
			ToolInput: map[string]any{
				"cmd":                 "node -e 'query db'",
				"sandbox_permissions": "require_escalated",
				"justification":       "需要连接本机 Postgres 确认语料包数据库记录已经改为“中国专利数据”，是否允许执行？",
			},
		},
	})

	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected approval pre-tool-use to become attention, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-approval")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected session attention, got %s", session.State)
	}
	if !strings.Contains(session.LastAssistantMessage, "Approval required") {
		t.Fatalf("expected approval summary, got %q", session.LastAssistantMessage)
	}
	if !strings.Contains(session.LastAssistantMessageFull, "Postgres") {
		t.Fatalf("expected justification in full approval summary, got %q", session.LastAssistantMessageFull)
	}
}
