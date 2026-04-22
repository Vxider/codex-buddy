package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (s *stubContinueExecutor) Continue(session model.SessionSnapshot, text string) error {
	s.called = true
	s.session = session
	s.text = text
	return nil
}

func TestStatusIncludesAttentionSummaryAndContinueAction(t *testing.T) {
	st := newAttentionStore(t)
	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, log.New(io.Discard, "", 0))

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
	if out.OverallState != model.StateAttention {
		t.Fatalf("expected attention overall state, got %s", out.OverallState)
	}
	if out.ActiveSessionDisplayTitle != "sidebar" {
		t.Fatalf("unexpected active display title: %q", out.ActiveSessionDisplayTitle)
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
	if session.AttentionSummary != "Need confirmation before overwriting files" {
		t.Fatalf("unexpected attention summary: %q", session.AttentionSummary)
	}
	if session.CompactAttention != "Need confirmation before overwriting files" {
		t.Fatalf("unexpected compact attention summary: %q", session.CompactAttention)
	}
	if session.MicroAttention != "Need confirmation before overwriting files" {
		t.Fatalf("unexpected micro attention summary: %q", session.MicroAttention)
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
		EventName:  "stop",
		ReceivedAt: now.Add(time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-dup",
			LastAssistantMessage: "Approval required before editing billing rules",
			TmuxPane:             "%8",
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, log.New(io.Discard, "", 0))
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
	if session.AttentionSummary != "Approval required before editing billing rules" {
		t.Fatalf("unexpected attention summary: %q", session.AttentionSummary)
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

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, log.New(io.Discard, "", 0))
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

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, log.New(io.Discard, "", 0))
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

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, log.New(io.Discard, "", 0))
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
		EventName:  "stop",
		ReceivedAt: now.Add(2 * time.Second),
		Payload: model.HookPayload{
			SessionID:            "sess-mobile",
			LastAssistantMessage: "I reviewed the flow and found the safest next step.\n\nApproval required before overwriting billing copy and continuing the refactor.",
			TmuxPane:             "%9",
		},
	})

	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, log.New(io.Discard, "", 0))
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
	if out.ActiveSessionCompactTitle == "" {
		t.Fatalf("expected active compact title")
	}
	if out.ActiveSessionMicroTitle == "" {
		t.Fatalf("expected active micro title")
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
	if session.CompactAttention == "" {
		t.Fatalf("expected compact attention summary")
	}
	if !strings.Contains(session.CompactAttention, "Approval required") {
		t.Fatalf("expected compact attention summary to preserve tail context, got %q", session.CompactAttention)
	}
	if len([]rune(session.CompactAttention)) > 72 {
		t.Fatalf("expected compact attention summary to be short, got %q", session.CompactAttention)
	}
	if session.MicroAttention == "" {
		t.Fatalf("expected micro attention summary")
	}
	if !strings.Contains(session.MicroAttention, "Approval required") {
		t.Fatalf("expected micro attention summary to preserve tail context, got %q", session.MicroAttention)
	}
	if len([]rune(session.MicroAttention)) > 44 {
		t.Fatalf("expected micro attention summary to be very short, got %q", session.MicroAttention)
	}
}

func TestSessionContinueEndpoint(t *testing.T) {
	st := newAttentionStore(t)
	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, log.New(io.Discard, "", 0))

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
	if out.Status.Sessions[0].CanContinue {
		t.Fatalf("expected continue action to disappear after acting")
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
			LastAssistantMessage: "Need confirmation before overwriting files",
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
			LastAssistantMessage: "Ready to continue after confirmation",
			TmuxPane:             "%12",
		},
	})

	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, log.New(io.Discard, "", 0))

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
			LastAssistantMessage: "Need confirmation before overwriting files",
			TmuxPane:             "%12",
		},
	})

	return st
}
