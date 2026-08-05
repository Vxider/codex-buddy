package store

import (
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vxider/agent-buddy/internal/model"
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
			LastAssistantMessage: "If you want, I can continue with the next sidebar refactor step",
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

func TestCodexInterruptionHookPromotesSessionToError(t *testing.T) {
	tests := []string{
		"HTTP 401 Unauthorized",
		"HTTP 402 Payment Required",
		"usage quota exhausted",
		"network connection timed out",
	}

	for index, message := range tests {
		st := New(0, 0, log.New(io.Discard, "", 0))
		now := time.Now().UTC()
		sessionID := fmt.Sprintf("hook-interruption-%d", index)

		snapshot := st.ApplyIngest(model.IngestRequest{
			EventName:  "user-prompt-submit",
			ReceivedAt: now,
			Payload: model.HookPayload{
				SessionID: sessionID,
				Error:     message,
			},
		})
		if snapshot.OverallState != model.StateError {
			t.Fatalf("expected hook error %q to set overall error, got %s", message, snapshot.OverallState)
		}

		session, ok := st.Session(sessionID)
		if !ok {
			t.Fatalf("expected session %q", sessionID)
		}
		if session.State != model.StateError {
			t.Fatalf("expected hook error %q to set session error, got %s", message, session.State)
		}
		if session.LastError != message {
			t.Fatalf("expected hook error metadata %q, got %q", message, session.LastError)
		}
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
	if session.CurrentAttentionDeadline.IsZero() {
		t.Fatalf("expected attention deadline")
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

func TestEmptyStoreSnapshotStaysIdle(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))

	snapshot := st.Snapshot()
	if snapshot.OverallState != model.StateIdle {
		t.Fatalf("expected empty store overall state to be idle, got %s", snapshot.OverallState)
	}
	if snapshot.OverallStateDetail != string(model.StateIdle) {
		t.Fatalf("expected empty store detail to be idle, got %q", snapshot.OverallStateDetail)
	}
	if snapshot.SessionsCount != 0 {
		t.Fatalf("expected no sessions, got %d", snapshot.SessionsCount)
	}
}

func TestCompletedStopStaysIdle(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-complete",
			Prompt:    "merge the PR",
		},
	})
	if snapshot.OverallState != model.StateRunning {
		t.Fatalf("expected running, got %s", snapshot.OverallState)
	}

	snapshot = st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-complete",
			LastAssistantMessage: "Completed. PR merged, branch deleted, and worktree is clean.",
		},
	})
	if snapshot.OverallState != model.StateIdle {
		t.Fatalf("expected idle after completed stop, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-complete")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateIdle {
		t.Fatalf("expected completed session to stay idle, got %s", session.State)
	}
}

func TestChineseFollowUpOfferStopBecomesAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-cn-attention",
			Prompt:    "排查 tailscale https 录音问题",
			TmuxPane:  "%9",
		},
	})
	if snapshot.OverallState != model.StateRunning {
		t.Fatalf("expected running, got %s", snapshot.OverallState)
	}

	snapshot = st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-cn-attention",
			TmuxPane:             "%9",
			LastAssistantMessage: "如果你愿意，我下一步可以继续帮你做一次 Tailscale HTTPS 下的录音可用性确认。",
		},
	})
	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected attention after Chinese follow-up offer, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-cn-attention")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected attention session, got %s", session.State)
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

func TestChineseImmediateStartOfferStopBecomesAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-cn-direct-start",
			Prompt:    "排查 ASR 浏览器 E2E",
			TmuxPane:  "%10",
		},
	})
	if snapshot.OverallState != model.StateRunning {
		t.Fatalf("expected running, got %s", snapshot.OverallState)
	}

	snapshot = st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-cn-direct-start",
			TmuxPane:             "%10",
			LastAssistantMessage: "如果你要，我下一步就直接开始做这条 ASR 的浏览器 E2E 调试。",
		},
	})
	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected attention after Chinese direct-start offer, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-cn-direct-start")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected attention session, got %s", session.State)
	}
}

func TestChineseContinueNextRoundOfferStopBecomesAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	snapshot := st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-cn-next-round",
			Prompt:    "收口旧 biddata 图片迁移策略",
			TmuxPane:  "%11",
		},
	})
	if snapshot.OverallState != model.StateRunning {
		t.Fatalf("expected running, got %s", snapshot.OverallState)
	}

	snapshot = st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-cn-next-round",
			TmuxPane:             "%11",
			LastAssistantMessage: "当前还留一个明确边界：历史上已经生成、但文件名里没有用户归属前缀的旧 `biddata` 图片，现在是“登录可读”但还不是“按所有者强隔离”。如果你继续，我下一轮会顺着这条线把旧资源迁移/兼容策略也收口掉。",
		},
	})
	if snapshot.OverallState != model.StateAttention {
		t.Fatalf("expected attention after Chinese next-round follow-up offer, got %s", snapshot.OverallState)
	}

	session, ok := st.Session("sess-cn-next-round")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected attention session, got %s", session.State)
	}
}

func TestAttentionNotificationSummaryPreservesFullAssistantReply(t *testing.T) {
	st := New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()
	fullReply := "这轮继续做了两处共享层收口，都是同类根因的全局修正，不是局部补丁。\n\n`AskData` 这边把 multipart 顺序改正了，未登录请求不会再提前触发上传解析，同时把共享鉴权上下文复用到了后续处理链路。\n\n`System Manage` 这边把真实路径约束也补齐了，避免通过 symlink 越出允许根目录；如果你继续，我下一轮会把剩余同类入口再统一扫一遍。"

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-full-open-notification",
			Prompt:    "继续排查共享层问题",
			TmuxPane:  "%12",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-full-open-notification",
			TmuxPane:             "%12",
			LastAssistantMessage: fullReply,
		},
	})

	notifications := st.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifications))
	}
	if notifications[0].Summary != fullReply {
		t.Fatalf("expected full notification summary, got %q", notifications[0].Summary)
	}
	if len([]rune(fullReply)) <= 160 {
		t.Fatalf("expected test fixture to exceed preview length, got %d", len([]rune(fullReply)))
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

func TestTranscriptUpdateCanPromoteIdleSessionToAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-bootstrap-idle",
			TmuxPane:  "%12",
		},
	})

	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:            "sess-bootstrap-idle",
		LastAssistantMessage: "如果你愿意，我下一步可以继续帮你做一次 Tailscale HTTPS 下的录音可用性确认。",
		UpdatedAt:            now.Add(time.Second),
	})

	session, ok := st.Session("sess-bootstrap-idle")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected transcript recovery to promote idle session to attention, got %s", session.State)
	}

	items := st.Notifications()
	if len(items) != 1 {
		t.Fatalf("expected one attention notification, got %d", len(items))
	}
	if items[0].Kind != model.NotificationAttention {
		t.Fatalf("expected attention notification, got %s", items[0].Kind)
	}
}

func TestTranscriptUpdateCanPromoteRunningSessionToAttention(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-running-followup",
			TmuxPane:  "%13",
		},
	})

	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:            "sess-running-followup",
		LastAssistantMessage: "如果你愿意，我下一步可以继续帮你把剩下的验证也跑完。",
		UpdatedAt:            now.Add(time.Second),
	})

	session, ok := st.Session("sess-running-followup")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateAttention {
		t.Fatalf("expected running session to become attention, got %s", session.State)
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

func TestTranscriptErrorDoesNotPromoteRunningSessionToError(t *testing.T) {
	st := New(0, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-running",
		},
	})

	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID: "sess-running",
		Error:     "tool failed internally",
		UpdatedAt: now.Add(time.Second),
	})

	session, ok := st.Session("sess-running")
	if !ok {
		t.Fatalf("expected session")
	}
	if session.State != model.StateRunning {
		t.Fatalf("expected running session to stay running after transcript error, got %s", session.State)
	}
	if session.LastError != "tool failed internally" {
		t.Fatalf("expected last error metadata to be retained, got %q", session.LastError)
	}
}

func TestCodexInterruptionTranscriptPromotesRunningSessionToError(t *testing.T) {
	tests := []string{
		"HTTP 401 Unauthorized",
		"HTTP 402 Payment Required",
		"usage quota exhausted",
		"network connection timed out",
	}

	for index, message := range tests {
		st := New(0, 0, log.New(io.Discard, "", 0))
		now := time.Now().UTC()
		sessionID := fmt.Sprintf("sess-interruption-%d", index)
		st.ApplyIngest(model.IngestRequest{
			EventName:  "user-prompt-submit",
			ReceivedAt: now,
			Payload: model.HookPayload{
				SessionID: sessionID,
			},
		})

		st.ApplyTranscriptUpdate(model.TranscriptUpdate{
			SessionID: sessionID,
			Error:     message,
			UpdatedAt: now.Add(time.Second),
		})

		session, ok := st.Session(sessionID)
		if !ok {
			t.Fatalf("expected session %q", sessionID)
		}
		if session.State != model.StateError {
			t.Fatalf("expected %q to promote session to error, got %s", message, session.State)
		}
		if session.StateDetail != string(model.StateError) {
			t.Fatalf("expected error state detail, got %q", session.StateDetail)
		}
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

func TestTranscriptErrorDoesNotCreateNotificationWhileRunning(t *testing.T) {
	st := New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-error",
			TmuxPane:  "%1",
		},
	})
	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:       "sess-error",
		LastBashCommand: "npm test",
		Error:           "FAIL\tapi",
		UpdatedAt:       now.Add(time.Second),
	})

	items := st.Notifications()
	if len(items) != 0 {
		t.Fatalf("expected no notification for running transcript error, got %d", len(items))
	}
}

func TestStopErrorCreatesReadableNotification(t *testing.T) {
	st := New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Now().UTC()

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-error",
			TmuxPane:  "%1",
		},
	})
	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:       "sess-error",
		LastBashCommand: "npm test",
		Error:           "FAIL\tapi",
		UpdatedAt:       now.Add(time.Second),
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(2 * time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-error",
			TmuxPane:  "%1",
			Error:     "FAIL\tapi",
		},
	})

	items := st.Notifications()
	if len(items) != 1 {
		t.Fatalf("expected one notification after stop error, got %d", len(items))
	}
	if items[0].Title != "Command failed" {
		t.Fatalf("unexpected notification title: %q", items[0].Title)
	}
	if items[0].Summary != "Command failed: npm test" {
		t.Fatalf("unexpected notification summary: %q", items[0].Summary)
	}
}
