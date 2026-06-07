package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vxider/codex-buddy/engine/store"
	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
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

func TestSessionContinueEndpointUsesCustomTextWhenProvided(t *testing.T) {
	st := newAttentionStore(t)
	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, nil, log.New(io.Discard, "", 0))

	notifications := st.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifications))
	}

	body, err := json.Marshal(map[string]string{
		"action_token": notifications[0].ActionToken,
		"text":         "fix the flaky test and rerun",
	})
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
	if exec.text != "fix the flaky test and rerun" {
		t.Fatalf("unexpected continue text: %q", exec.text)
	}
}

func TestSessionContinueEndpointFallsBackToDefaultContinueText(t *testing.T) {
	st := newAttentionStore(t)
	exec := &stubContinueExecutor{}
	server := NewServer(config.Config{}, st, nil, exec, nil, log.New(io.Discard, "", 0))

	notifications := st.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifications))
	}

	body, err := json.Marshal(map[string]string{
		"action_token": notifications[0].ActionToken,
	})
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
	if exec.text != model.ContinueCommandText {
		t.Fatalf("unexpected continue text: %q", exec.text)
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
