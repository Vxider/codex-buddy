package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
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
	if out.ActiveSessionDisplayTitle != "Refactor sidebar" {
		t.Fatalf("unexpected active display title: %q", out.ActiveSessionDisplayTitle)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(out.Sessions))
	}

	session := out.Sessions[0]
	if session.DisplayTitle != "Refactor sidebar" {
		t.Fatalf("unexpected display title: %q", session.DisplayTitle)
	}
	if session.AttentionSummary != "Need confirmation before overwriting files" {
		t.Fatalf("unexpected attention summary: %q", session.AttentionSummary)
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
