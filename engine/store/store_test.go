package store

import (
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/vxider/agent-buddy/internal/model"
)

func TestDiscoveryDoesNotRemoveClaudeHookSessions(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 6, 28, 13, 30, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		Source:     "claude-hook",
		Agent:      "claude",
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			Agent:      "claude",
			SessionID:  "claude-session",
			TmuxPane:   "%99",
			TmuxWindow: "@99",
			Prompt:     "build it",
		},
	})

	removed, _ := st.ApplyDiscovery(nil, now.Add(time.Second))
	if len(removed) != 0 {
		t.Fatalf("expected no discovered removals, got %v", removed)
	}

	session, ok := st.Session("claude-session")
	if !ok {
		t.Fatalf("expected claude hook session to remain after codex discovery")
	}
	if session.Agent != "claude" {
		t.Fatalf("expected claude agent, got %q", session.Agent)
	}
	if session.TmuxWindow != "@99" {
		t.Fatalf("expected tmux window to remain, got %q", session.TmuxWindow)
	}
}

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

func TestTranscriptGoalClearRemovesAchievedGoalState(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:     "sess-goal",
		UpdatedAt:     now,
		GoalUpdated:   true,
		GoalState:     model.GoalStateAchieved,
		GoalSummary:   "ship the fix",
		GoalUpdatedAt: now,
	})

	snapshot := st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:     "sess-goal",
		UpdatedAt:     now.Add(time.Second),
		GoalUpdated:   true,
		GoalState:     "",
		GoalSummary:   "",
		GoalUpdatedAt: now.Add(time.Second),
	})
	if snapshot.GoalState != "" {
		t.Fatalf("expected status goal state to be empty after clear, got %q", snapshot.GoalState)
	}
	if snapshot.GoalSummary != "" {
		t.Fatalf("expected status goal summary to be empty after clear, got %q", snapshot.GoalSummary)
	}
	if !snapshot.GoalUpdatedAt.IsZero() {
		t.Fatalf("expected status goal updated time to be empty after clear, got %s", snapshot.GoalUpdatedAt)
	}

	session, ok := st.Session("sess-goal")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.GoalState != "" {
		t.Fatalf("expected session goal state to be empty after clear, got %q", session.GoalState)
	}
	if session.GoalSummary != "" {
		t.Fatalf("expected session goal summary to be empty after clear, got %q", session.GoalSummary)
	}
	if !session.GoalUpdatedAt.IsZero() {
		t.Fatalf("expected session goal updated time to be empty after clear, got %s", session.GoalUpdatedAt)
	}
}

func TestTranscriptUserPromptAfterAchievedClearsStaleGoalState(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:     "sess-goal",
		UpdatedAt:     now,
		GoalUpdated:   true,
		GoalState:     model.GoalStateAchieved,
		GoalSummary:   "ship the fix",
		GoalUpdatedAt: now,
	})

	snapshot := st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:             "sess-goal",
		UpdatedAt:             now.Add(time.Second),
		LastUserPromptPreview: "next question",
	})
	if snapshot.GoalState != "" {
		t.Fatalf("expected status goal state to be empty after later user prompt, got %q", snapshot.GoalState)
	}
	session, ok := st.Session("sess-goal")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.GoalState != "" {
		t.Fatalf("expected session goal state to be empty after later user prompt, got %q", session.GoalState)
	}
	if session.GoalSummary != "" {
		t.Fatalf("expected session goal summary to be empty after later user prompt, got %q", session.GoalSummary)
	}
	if !session.GoalUpdatedAt.IsZero() {
		t.Fatalf("expected session goal updated time to be empty after later user prompt, got %s", session.GoalUpdatedAt)
	}
}

func TestStopFollowUpOfferBecomesAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID:            "sess-followup",
			TmuxPane:             "%2",
			LastAssistantMessage: "如果你愿意，我下一步可以继续帮你把剩下的 E2E 失败用例收口。",
		},
	})

	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected follow-up stop to become attention, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-followup")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected attention session, got %s", session.State)
	}
}

func TestTranscriptFollowUpPromotesRunningSessionToAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-running-followup",
		},
	})

	snapshot := st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:            "sess-running-followup",
		LastAssistantMessage: "如果你愿意，我下一步可以继续帮你把剩下的验证也跑完。",
		UpdatedAt:            now.Add(time.Second),
	})
	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected attention after transcript follow-up, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-running-followup")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected running session to become attention, got %s", session.State)
	}
}

func TestStopQuestionWithoutFollowUpOfferStaysIdle(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID:            "sess-question",
			TmuxPane:             "%3",
			LastAssistantMessage: "测试完成。你还有别的问题吗？",
		},
	})

	if snapshot.OverallState != model.StateIdle {
		t.Fatalf("expected generic question stop to stay idle, got %s", snapshot.OverallState)
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

func TestPermissionRequestBecomesApprovalAttention(t *testing.T) {
	st := New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "permission-request",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-permission",
			ToolName:  "Bash",
			TmuxPane:  "%11",
			ToolInput: map[string]any{
				"command":       "node -e 'query db'",
				"description":   "Run a local Postgres lookup for E2E credentials.",
				"justification": "需要查询本机 Postgres 用户表以获得可用于 E2E 的真实登录账号，是否允许执行？",
			},
		},
	})

	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected permission request to become attention, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-permission")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected session attention, got %s", session.State)
	}
	if !strings.Contains(session.LastAssistantMessageFull, "local Postgres lookup") {
		t.Fatalf("expected permission description in approval summary, got %q", session.LastAssistantMessageFull)
	}
	if strings.Contains(session.LastAssistantMessageFull, "真实登录账号") {
		t.Fatalf("expected official description to win over fallback justification, got %q", session.LastAssistantMessageFull)
	}
}
