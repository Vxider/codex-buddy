package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
	"github.com/vxider/codex-buddy/webserver/internal/store"
)

type stubContinueExecutor struct {
	called  bool
	session model.SessionSnapshot
	text    string
}

type stubSessionOpenChecker struct {
	open map[string]bool
}

func (s *stubContinueExecutor) Continue(session model.SessionSnapshot, text string) error {
	s.called = true
	s.session = session
	s.text = text
	return nil
}

func (s stubSessionOpenChecker) IsOpen(session model.SessionSnapshot) bool {
	if s.open == nil {
		return true
	}
	open, ok := s.open[session.SessionID]
	if !ok {
		return true
	}
	return open
}

func TestStatusIncludesOpenSummaryAndContinueAction(t *testing.T) {
	st := newAttentionStore(t)
	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if out.OverallState != model.StateOpen {
		t.Fatalf("expected open overall state, got %s", out.OverallState)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.DisplayTitle != "sidebar" {
		t.Fatalf("unexpected display title: %q", session.DisplayTitle)
	}
	if session.CompactTitle != "sidebar" {
		t.Fatalf("unexpected compact title: %q", session.CompactTitle)
	}
	if session.MicroTitle != "sidebar" {
		t.Fatalf("unexpected micro title: %q", session.MicroTitle)
	}
	if session.OpenSummary != "If you want, I can continue with the next sidebar refactor step" {
		t.Fatalf("unexpected open summary: %q", session.OpenSummary)
	}
	if session.OpenMarkdown != "If you want, I can continue with the next sidebar refactor step" {
		t.Fatalf("unexpected open markdown: %q", session.OpenMarkdown)
	}
	if !strings.Contains(session.OpenHTML, "<p>If you want, I can continue with the next sidebar refactor step</p>") {
		t.Fatalf("expected rendered open html, got %q", session.OpenHTML)
	}
	if session.CompactOpen != "If you want, I can continue with the next sidebar refactor step" {
		t.Fatalf("unexpected compact open summary: %q", session.CompactOpen)
	}
	if session.MicroOpen != "If you want, I can continue with the next s…" {
		t.Fatalf("unexpected micro open summary: %q", session.MicroOpen)
	}
	if !session.NeedsOpen {
		t.Fatalf("expected session to need open follow-up")
	}
	if session.NeedsApproval {
		t.Fatalf("expected follow-up state to stay separate from approval")
	}
	if session.OpenReason != "followup" {
		t.Fatalf("expected followup open reason, got %q", session.OpenReason)
	}
	if !session.CanContinue {
		t.Fatalf("expected session to be continuable")
	}
	if session.ContinueAction == nil {
		t.Fatalf("expected continue action metadata")
	}
	if session.ContinueAction.Endpoint != "/v1/sessions/sess-1/continue" {
		t.Fatalf("unexpected continue endpoint: %q", session.ContinueAction.Endpoint)
	}
	if session.ContinueAction.ActionToken == "" {
		t.Fatalf("expected action token")
	}
}

func TestOpenSummaryPreservesFullAssistantReply(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)
	fullReply := "先给你结论：当前数据库迁移不需要回滚。\n\n我已经核对了 schema 差异、线上数据分布、历史兼容逻辑和回滚窗口，现阶段风险主要在兼容层，而不是数据损坏；真正需要处理的是旧调用链、灰度入口和文件命名约束没有完全统一。\n\n如果你继续，我下一步会把兼容迁移、旧路径清理、权限边界补丁和回归验证一起收口，避免你后面再分三轮重复确认。"

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-open-full",
			CWD:       "/repo/payments",
			Prompt:    "检查迁移风险",
			TmuxPane:  "%71",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-open-full",
			CWD:                  "/repo/payments",
			TmuxPane:             "%71",
			LastAssistantMessage: fullReply,
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.OpenSummary != fullReply {
		t.Fatalf("expected full open summary, got %q", session.OpenSummary)
	}
	if !strings.Contains(session.OpenHTML, "<p>") {
		t.Fatalf("expected open html paragraphs, got %q", session.OpenHTML)
	}
	if session.NeedsApproval {
		t.Fatalf("expected non-approval follow-up to stay visible without approval flag")
	}
	if session.OpenReason != "followup" {
		t.Fatalf("expected followup open reason, got %q", session.OpenReason)
	}
	if len([]rune(fullReply)) <= 160 {
		t.Fatalf("expected test fixture to exceed preview length, got %d", len([]rune(fullReply)))
	}
	if session.CompactOpen == fullReply {
		t.Fatalf("expected compact open summary to remain condensed")
	}
	if !strings.Contains(session.CompactOpen, "如果你继续") {
		t.Fatalf("expected compact open summary to preserve tail context, got %q", session.CompactOpen)
	}
}

func TestStatusMarksRequireEscalatedPreToolUseAsApproval(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 6, 7, 14, 17, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "pre-tool-use",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-escalated",
			CWD:       "/repo/flashai",
			ToolName:  "Bash",
			TmuxPane:  "%11",
			ToolInput: map[string]any{
				"cmd":                 "node -e 'query db'",
				"sandbox_permissions": "require_escalated",
				"justification":       "需要连接本机 Postgres 确认语料包数据库记录已经改为“中国专利数据”，是否允许执行？",
			},
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if out.OverallState != model.StateOpen {
		t.Fatalf("expected open overall state, got %s", out.OverallState)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if !session.NeedsApproval {
		t.Fatalf("expected approval flag")
	}
	if session.OpenReason != "approval" {
		t.Fatalf("expected approval open reason, got %q", session.OpenReason)
	}
	if !strings.Contains(session.OpenSummary, "Postgres") {
		t.Fatalf("expected approval justification in open summary, got %q", session.OpenSummary)
	}
}

func TestStatusMarksPermissionRequestAsApproval(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 6, 7, 14, 35, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "permission-request",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-permission",
			CWD:       "/repo/flashai",
			ToolName:  "Bash",
			TmuxPane:  "%11",
			ToolInput: map[string]any{
				"command":       "node -e 'query users'",
				"justification": "需要查询本机 Postgres 用户表以获得可用于 E2E 的真实登录账号，是否允许执行？",
			},
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if out.OverallState != model.StateOpen {
		t.Fatalf("expected open overall state, got %s", out.OverallState)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if !session.NeedsApproval {
		t.Fatalf("expected approval flag")
	}
	if session.OpenReason != "approval" {
		t.Fatalf("expected approval open reason, got %q", session.OpenReason)
	}
	if !strings.Contains(session.OpenSummary, "Postgres") {
		t.Fatalf("expected permission reason in open summary, got %q", session.OpenSummary)
	}
}

func TestStoppedOpenSessionShowsFullAssistantReply(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	fullReply := "Done. I finished the implementation, updated the tests, and left the deployment notes in place.\n\nThe remaining work is operational: restart the service, open the uConsole launcher again, and confirm the taskbar icon is no longer inherited from another app."

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-stopped-full",
			CWD:       "/repo/uconsole",
			Prompt:    "finish taskbar work",
			TmuxPane:  "%72",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-stopped-full",
			CWD:                  "/repo/uconsole",
			TmuxPane:             "%72",
			LastAssistantMessage: fullReply,
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}
	session := out.Sessions[0]
	if session.State != model.StateIdle {
		t.Fatalf("expected public idle state, got %s", session.State)
	}
	if !strings.Contains(session.Summary, "remaining work is operational") {
		t.Fatalf("expected idle summary to preserve completion context, got %q", session.Summary)
	}
}

func TestEmptyStatusIsIdleWhenServerIsReachable(t *testing.T) {
	st := store.New(0, 0, log.New(io.Discard, "", 0))
	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if out.OverallState != model.StateIdle {
		t.Fatalf("expected idle overall state for reachable empty server, got %s", out.OverallState)
	}
	if out.OverallStateDetail != string(model.StateIdle) {
		t.Fatalf("expected idle state detail, got %q", out.OverallStateDetail)
	}
	if out.SessionsCount != 0 {
		t.Fatalf("expected zero sessions, got %d", out.SessionsCount)
	}
	if out.ServerState.State != model.StateIdle {
		t.Fatalf("expected raw idle server state, got %s", out.ServerState.State)
	}
	if out.ServerState.StateDetail != string(model.StateIdle) {
		t.Fatalf("expected raw idle server state detail, got %q", out.ServerState.StateDetail)
	}
	if out.ServerState.SessionsCount != 0 {
		t.Fatalf("expected zero server state sessions, got %d", out.ServerState.SessionsCount)
	}
	if out.ServerState.LastUpdatedAt != nil {
		t.Fatalf("expected no server last updated time, got %v", out.ServerState.LastUpdatedAt)
	}
}

func TestInternalShutdownRequestsServerStop(t *testing.T) {
	st := store.New(0, 0, log.New(io.Discard, "", 0))
	server := NewServer(config.Config{Internal: config.InternalConfig{RequireLoopback: true}}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))

	var (
		wg     sync.WaitGroup
		called chan struct{}
	)
	called = make(chan struct{}, 1)
	wg.Add(1)
	server.SetShutdownFunc(func() {
		defer wg.Done()
		called <- struct{}{}
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/shutdown", bytes.NewReader([]byte("{}")))
	req.RemoteAddr = "127.0.0.1:12345"
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.Code)
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("expected shutdown callback to be called")
	}
	wg.Wait()
}

func TestInternalShutdownRejectsNonLoopbackRequests(t *testing.T) {
	st := store.New(0, 0, log.New(io.Discard, "", 0))
	server := NewServer(config.Config{Internal: config.InternalConfig{RequireLoopback: true}}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))

	called := false
	server.SetShutdownFunc(func() {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/shutdown", bytes.NewReader([]byte("{}")))
	req.RemoteAddr = "192.168.1.20:12345"
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
	if called {
		t.Fatal("expected shutdown callback to remain untouched")
	}
}

func TestRenderRichTextConvertsANSIToMarkdownAndHTML(t *testing.T) {
	raw := "build failed\n\x1b[31mERROR\x1b[0m: listen EPERM"
	markdown, html := renderRichText(raw)

	if !strings.HasPrefix(markdown, "```text\n") {
		t.Fatalf("expected ansi markdown to become fenced code block, got %q", markdown)
	}
	if strings.Contains(markdown, "\x1b[") {
		t.Fatalf("expected ansi markdown to strip escape sequences, got %q", markdown)
	}
	if !strings.Contains(html, `class="terminal-output"`) {
		t.Fatalf("expected terminal html wrapper, got %q", html)
	}
	if !strings.Contains(html, "ERROR") {
		t.Fatalf("expected terminal html content, got %q", html)
	}
	if !strings.Contains(html, "color:#f87171") {
		t.Fatalf("expected ansi color style in html, got %q", html)
	}
}

func TestRenderRichTextConvertsMarkdownToHTML(t *testing.T) {
	raw := "先看 `server.go`。\n\n需要去 [status 页](/status) 再确认。"
	markdown, html := renderRichText(raw)

	if markdown != raw {
		t.Fatalf("expected markdown text to stay unchanged, got %q", markdown)
	}
	if !strings.Contains(html, "<code>server.go</code>") {
		t.Fatalf("expected inline code to render, got %q", html)
	}
	if !strings.Contains(html, `<a href="/status">status 页</a>`) {
		t.Fatalf("expected markdown link to render, got %q", html)
	}
}

func TestStatusTitleDoesNotReuseAssistantSummary(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-dup",
			CWD:       "/repo/payments",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "permission-request",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-dup",
			ToolName:  "Bash",
			TmuxPane:  "%8",
			ToolInput: map[string]any{
				"command":     "python scripts/update_billing_rules.py",
				"description": "Approval required before editing billing rules",
			},
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.DisplayTitle != "payments" {
		t.Fatalf("expected cwd-derived title, got %q", session.DisplayTitle)
	}
	if session.CompactTitle != "payments" {
		t.Fatalf("expected compact cwd-derived title, got %q", session.CompactTitle)
	}
	if session.MicroTitle != "payments" {
		t.Fatalf("expected micro cwd-derived title, got %q", session.MicroTitle)
	}
	if session.OpenSummary != "Approval required: Approval required before editing billing rules" {
		t.Fatalf("unexpected open summary: %q", session.OpenSummary)
	}
	if !session.NeedsApproval {
		t.Fatalf("expected approval message to expose approval state")
	}
	if session.OpenReason != "approval" {
		t.Fatalf("expected approval open reason, got %q", session.OpenReason)
	}
}

func TestStatusTitleDoesNotUseBashCommandFallback(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 23, 12, 10, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-bash",
			CWD:       "/repo/payments",
		},
	})
	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:       "sess-bash",
		LastBashCommand: "npm test -- payments",
		UpdatedAt:       now.Add(time.Second),
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.DisplayTitle != "payments" {
		t.Fatalf("expected cwd title instead of bash command, got %q", session.DisplayTitle)
	}
	if session.CompactTitle != "payments" {
		t.Fatalf("expected compact cwd title instead of bash command, got %q", session.CompactTitle)
	}
	if session.MicroTitle != "payments" {
		t.Fatalf("expected micro cwd title instead of bash command, got %q", session.MicroTitle)
	}
}

func TestStatusTitleSkipsCommandLikePrompt(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 23, 12, 20, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-prompt-cmd",
			CWD:       "/repo/payments",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-prompt-cmd",
			Prompt:    "npm test -- payments",
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.DisplayTitle != "payments" {
		t.Fatalf("expected cwd title instead of command-like prompt, got %q", session.DisplayTitle)
	}
}

func TestStatusTitlesDisambiguateDuplicateWorkspaces(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 23, 12, 25, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-dup-a-12345678",
			CWD:       "/repo/payments",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-dup-b-87654321",
			CWD:       "/another/payments",
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(out.Sessions))
	}

	for _, session := range out.Sessions {
		if !strings.Contains(session.DisplayTitle, "payments") {
			t.Fatalf("expected workspace name in display title, got %q", session.DisplayTitle)
		}
		if !strings.Contains(session.DisplayTitle, "·") {
			t.Fatalf("expected duplicate workspace title to include suffix, got %q", session.DisplayTitle)
		}
		if !strings.Contains(session.CompactTitle, "·") {
			t.Fatalf("expected compact duplicate workspace title to include suffix, got %q", session.CompactTitle)
		}
	}
}

func TestStatusIncludesCompactMobileFields(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 23, 12, 30, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-mobile",
			CWD:       "/repo/payments",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-mobile",
			Prompt:    "Refactor the payment approval flow to separate the fraud review copy from the final customer-facing step.",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "permission-request",
		ReceivedAt: now.Add(2 * time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-mobile",
			ToolName:  "Bash",
			TmuxPane:  "%9",
			ToolInput: map[string]any{
				"command":     "python scripts/update_billing_copy.py",
				"description": "I reviewed the flow and found the safest next step.\n\nApproval required before overwriting billing copy and continuing the refactor.",
			},
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.CompactTitle == "" {
		t.Fatalf("expected compact title")
	}
	if len([]rune(session.CompactTitle)) > 42 {
		t.Fatalf("expected compact title to be short, got %q", session.CompactTitle)
	}
	if session.DisplayTitle != "payments" {
		t.Fatalf("expected workspace display title, got %q", session.DisplayTitle)
	}
	if session.MicroTitle == "" {
		t.Fatalf("expected micro title")
	}
	if len([]rune(session.MicroTitle)) > 24 {
		t.Fatalf("expected micro title to be very short, got %q", session.MicroTitle)
	}
	if session.CompactOpen == "" {
		t.Fatalf("expected compact open summary")
	}
	if !strings.Contains(session.CompactOpen, "Approval required") {
		t.Fatalf("expected compact open summary to preserve tail context, got %q", session.CompactOpen)
	}
	if len([]rune(session.CompactOpen)) > 72 {
		t.Fatalf("expected compact open summary to be short, got %q", session.CompactOpen)
	}
	if session.MicroOpen == "" {
		t.Fatalf("expected micro open summary")
	}
	if !strings.Contains(session.MicroOpen, "Approval required") {
		t.Fatalf("expected micro open summary to preserve tail context, got %q", session.MicroOpen)
	}
	if len([]rune(session.MicroOpen)) > 44 {
		t.Fatalf("expected micro open summary to be very short, got %q", session.MicroOpen)
	}
}

func TestStatusRunningSummaryDoesNotExposeTranscriptError(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-running",
			CWD:       "/repo/payments",
			Prompt:    "refactor payments flow",
			TmuxPane:  "%51",
		},
	})
	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:            "sess-running",
		Error:                "internal tool step failed",
		LastAssistantMessage: "still working through the refactor",
		UpdatedAt:            now.Add(time.Second),
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}
	if out.Sessions[0].State != model.StateRun {
		t.Fatalf("expected run state, got %s", out.Sessions[0].State)
	}
	if out.Sessions[0].Summary != "still working through the refactor" {
		t.Fatalf("unexpected running summary: %q", out.Sessions[0].Summary)
	}
}

func TestStatusErrorSummaryPrefersReadableCommandFailure(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 25, 1, 0, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-error",
			CWD:       "/repo/payments",
			TmuxPane:  "%44",
		},
	})
	st.ApplyTranscriptUpdate(model.TranscriptUpdate{
		SessionID:       "sess-error",
		LastBashCommand: "go test ./webserver/...",
		Error:           "FAIL\tgithub.com/vxider/codex-buddy/webserver/internal/api\t0.007s",
		UpdatedAt:       now.Add(time.Second),
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(2 * time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-error",
			Error:     "FAIL\tgithub.com/vxider/codex-buddy/webserver/internal/api\t0.007s",
			TmuxPane:  "%44",
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}
	if out.Sessions[0].State != model.StateError {
		t.Fatalf("expected error state, got %s", out.Sessions[0].State)
	}
	if out.Sessions[0].Summary != "Command failed: go test ./webserver/..." {
		t.Fatalf("unexpected error summary: %q", out.Sessions[0].Summary)
	}
	if out.ServerState.State != model.StateError {
		t.Fatalf("expected raw error server state, got %s", out.ServerState.State)
	}
	if out.ServerState.StateDetail != string(model.StateError) {
		t.Fatalf("expected raw error server state detail, got %q", out.ServerState.StateDetail)
	}
	if out.ServerState.ActiveSessionID != "sess-error" {
		t.Fatalf("expected error active session, got %q", out.ServerState.ActiveSessionID)
	}
	if out.ServerState.LastUpdatedAt == nil || !out.ServerState.LastUpdatedAt.Equal(now.Add(2*time.Second)) {
		t.Fatalf("unexpected server last updated time: %v", out.ServerState.LastUpdatedAt)
	}
}

func TestSessionContinueEndpoint(t *testing.T) {
	st := newAttentionStore(t)
	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, nil, log.New(io.Discard, "", 0))

	notifications := st.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifications))
	}

	body, err := json.Marshal(map[string]string{"action_token": notifications[0].ActionToken})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess-1/continue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !exec.called {
		t.Fatalf("expected continue executor to be called")
	}
	if exec.session.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %q", exec.session.SessionID)
	}
	if exec.text != model.ContinueCommandText {
		t.Fatalf("unexpected continue text: %q", exec.text)
	}

	var out struct {
		OK     bool         `json:"ok"`
		Status publicStatus `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode continue response: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok response")
	}
	if len(out.Status.Sessions) != 1 {
		t.Fatalf("expected one session in status response")
	}
	if !out.Status.Sessions[0].CanContinue {
		t.Fatalf("expected open session to remain continuable")
	}
	if out.Status.Sessions[0].ContinueAction == nil {
		t.Fatalf("expected fallback continue action")
	}
	if out.Status.Sessions[0].ContinueAction.ActionToken != "" {
		t.Fatalf("expected fallback continue action without token")
	}
}

func TestSessionContinueEndpointAcceptsTextWithoutActionToken(t *testing.T) {
	st := newAttentionStore(t)
	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, nil, log.New(io.Discard, "", 0))

	body, err := json.Marshal(map[string]string{"text": "use the voice transcript"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess-1/continue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !exec.called {
		t.Fatalf("expected continue executor to be called")
	}
	if exec.session.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %q", exec.session.SessionID)
	}
	if exec.text != "use the voice transcript" {
		t.Fatalf("unexpected text: %q", exec.text)
	}
}

func TestSessionContinueEndpointAcceptsDefaultContinueWithoutActionToken(t *testing.T) {
	st := newAttentionStore(t)
	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, nil, log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess-1/continue", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !exec.called {
		t.Fatalf("expected continue executor to be called")
	}
	if exec.text != model.ContinueCommandText {
		t.Fatalf("unexpected text: %q", exec.text)
	}
}

func TestSessionContinueEndpointFallsBackToLatestActionableNotification(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-1",
			CWD:       "/repo/sidebar",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now.Add(1 * time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-1",
			Prompt:    "Refactor sidebar",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(2 * time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-1",
			LastAssistantMessage: "If you want, I can continue with the next sidebar refactor step",
			TmuxPane:             "%12",
		},
	})

	originalNotifications := st.Notifications()
	if len(originalNotifications) != 1 {
		t.Fatalf("expected one notification, got %d", len(originalNotifications))
	}
	staleToken := originalNotifications[0].ActionToken

	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now.Add(3 * time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-1",
			Prompt:    "Proceed with overwrite",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(4 * time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-1",
			LastAssistantMessage: "If you want, I can continue with the next sidebar cleanup pass",
			TmuxPane:             "%12",
		},
	})

	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, nil, log.New(io.Discard, "", 0))

	body, err := json.Marshal(map[string]string{"action_token": staleToken})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess-1/continue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !exec.called {
		t.Fatalf("expected continue executor to be called")
	}
	if exec.session.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %q", exec.session.SessionID)
	}
}

func TestStatusUsesHookSessionsWithoutTmuxOpenFiltering(t *testing.T) {
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-hidden",
			CWD:       "/repo/hidden",
			TmuxPane:  "%20",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-visible",
			CWD:       "/repo/visible",
			TmuxPane:  "%21",
		},
	})

	server := NewServer(
		config.Config{},
		st,
		nil,
		&stubContinueExecutor{},
		stubSessionOpenChecker{open: map[string]bool{
			"sess-hidden":  false,
			"sess-visible": true,
		}},
		log.New(io.Discard, "", 0),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if out.SessionsCount != 2 {
		t.Fatalf("expected both hook sessions, got %d", out.SessionsCount)
	}
	if len(out.Sessions) != 2 {
		t.Fatalf("expected both hook session items, got %d", len(out.Sessions))
	}
	ids := map[string]bool{}
	for _, session := range out.Sessions {
		ids[session.SessionID] = true
	}
	if !ids["sess-hidden"] || !ids["sess-visible"] {
		t.Fatalf("expected hook sessions to bypass tmux open filtering, got %#v", ids)
	}
}

func TestNotificationsUseHookSessionsWithoutTmuxOpenFiltering(t *testing.T) {
	st := newAttentionStore(t)
	server := NewServer(
		config.Config{},
		st,
		nil,
		&stubContinueExecutor{},
		stubSessionOpenChecker{open: map[string]bool{
			"sess-1": false,
		}},
		log.New(io.Discard, "", 0),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var out struct {
		Notifications []publicNotification `json:"notifications"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode notifications: %v", err)
	}
	if len(out.Notifications) != 1 {
		t.Fatalf("expected hook session notification to remain visible, got %d", len(out.Notifications))
	}
}

func TestHookSessionDetailBypassesTmuxOpenFiltering(t *testing.T) {
	st := newAttentionStore(t)
	server := NewServer(
		config.Config{},
		st,
		nil,
		&stubContinueExecutor{},
		stubSessionOpenChecker{open: map[string]bool{
			"sess-1": false,
		}},
		log.New(io.Discard, "", 0),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/sess-1", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for hook session regardless of tmux open state, got %d", resp.Code)
	}
}

func TestClassifyOpenReasonDoesNotTreatApprovalMentionsAsApproval(t *testing.T) {
	message := `现在 GUI/LED 读的是 server 的字段：

- needs_approval == true
- open_reason == "approval"

如果 server 没把状态归一好，GUI 这边就只能猜。`
	if got := classifyOpenReason(message); got != "followup" {
		t.Fatalf("expected followup for explanatory approval text, got %q", got)
	}
	if got := classifyOpenReason("Approval required before editing billing rules"); got != "approval" {
		t.Fatalf("expected explicit approval request, got %q", got)
	}
}

func newAttentionStore(t *testing.T) *store.Store {
	t.Helper()
	st := store.New(30*time.Second, 0, log.New(io.Discard, "", 0))
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	st.ApplyIngest(model.IngestRequest{
		EventName:  "session-start",
		ReceivedAt: now,
		Payload: model.HookPayload{
			SessionID: "sess-1",
			CWD:       "/repo/sidebar",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "user-prompt-submit",
		ReceivedAt: now.Add(1 * time.Second),
		Payload: model.HookPayload{
			SessionID: "sess-1",
			Prompt:    "Refactor sidebar",
		},
	})
	st.ApplyIngest(model.IngestRequest{
		EventName:  "stop",
		ReceivedAt: now.Add(2 * time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-1",
			LastAssistantMessage: "If you want, I can continue with the next sidebar refactor step",
			TmuxPane:             "%12",
		},
	})

	return st
}
