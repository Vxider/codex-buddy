package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vxider/codex-buddy/internal/config"
)

type continueOKFixture struct {
	OK      bool          `json:"ok"`
	Message string        `json:"message"`
	Session publicSession `json:"session"`
	Status  publicStatus  `json:"status"`
}

func TestContractFixturesDecode(t *testing.T) {
	root := filepath.Join("..", "..", "api", "contract", "fixtures")

	for _, name := range []string{
		"status.idle.json",
		"status.running.json",
		"status.open.json",
	} {
		t.Run(name, func(t *testing.T) {
			var status publicStatus
			decodeContractFixture(t, filepath.Join(root, name), &status)
			if status.OverallState == "" {
				t.Fatalf("fixture %s has empty overall_state", name)
			}
			if status.SessionsCount != len(status.Sessions) {
				t.Fatalf("fixture %s sessions_count=%d len(sessions)=%d", name, status.SessionsCount, len(status.Sessions))
			}
		})
	}

	t.Run("continue.ok.json", func(t *testing.T) {
		var response continueOKFixture
		decodeContractFixture(t, filepath.Join(root, "continue.ok.json"), &response)
		if !response.OK {
			t.Fatalf("expected ok response")
		}
		if response.Session.SessionID == "" {
			t.Fatalf("expected session payload")
		}
	})
}

func TestOpenStatusContractFixtureMatchesServerShape(t *testing.T) {
	var fixture publicStatus
	decodeContractFixture(t, filepath.Join("..", "..", "api", "contract", "fixtures", "status.open.json"), &fixture)

	st := newAttentionStore(t)
	server := NewServer(config.Config{}, st, nil, &stubContinueExecutor{}, nil, log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var actual publicStatus
	if err := json.NewDecoder(resp.Body).Decode(&actual); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	if len(actual.Sessions) != 1 {
		t.Fatalf("expected one actual session, got %d", len(actual.Sessions))
	}
	if len(fixture.Sessions) != 1 {
		t.Fatalf("expected one fixture session, got %d", len(fixture.Sessions))
	}

	actualSession := actual.Sessions[0]
	fixtureSession := fixture.Sessions[0]
	assertContractSessionShape(t, fixtureSession, actualSession)
}

func assertContractSessionShape(t *testing.T, fixture publicSession, actual publicSession) {
	t.Helper()

	checks := map[string][2]any{
		"session_id":            {fixture.SessionID, actual.SessionID},
		"short_session_id":      {fixture.ShortSessionID, actual.ShortSessionID},
		"display_title":         {fixture.DisplayTitle, actual.DisplayTitle},
		"compact_title":         {fixture.CompactTitle, actual.CompactTitle},
		"micro_title":           {fixture.MicroTitle, actual.MicroTitle},
		"state":                 {fixture.State, actual.State},
		"state_detail":          {fixture.StateDetail, actual.StateDetail},
		"summary":               {fixture.Summary, actual.Summary},
		"summary_markdown":      {fixture.SummaryMarkdown, actual.SummaryMarkdown},
		"summary_html":          {fixture.SummaryHTML, actual.SummaryHTML},
		"compact_summary":       {fixture.CompactSummary, actual.CompactSummary},
		"micro_summary":         {fixture.MicroSummary, actual.MicroSummary},
		"needs_open":            {fixture.NeedsOpen, actual.NeedsOpen},
		"needs_approval":        {fixture.NeedsApproval, actual.NeedsApproval},
		"open_reason":           {fixture.OpenReason, actual.OpenReason},
		"open_summary":          {fixture.OpenSummary, actual.OpenSummary},
		"open_summary_markdown": {fixture.OpenMarkdown, actual.OpenMarkdown},
		"open_summary_html":     {fixture.OpenHTML, actual.OpenHTML},
		"compact_open_summary":  {fixture.CompactOpen, actual.CompactOpen},
		"micro_open_summary":    {fixture.MicroOpen, actual.MicroOpen},
		"tmux_pane":             {fixture.TmuxPane, actual.TmuxPane},
		"can_continue":          {fixture.CanContinue, actual.CanContinue},
	}
	for name, values := range checks {
		if values[0] != values[1] {
			t.Fatalf("%s fixture=%#v actual=%#v", name, values[0], values[1])
		}
	}
	if fixture.ContinueAction == nil || actual.ContinueAction == nil {
		t.Fatalf("expected continue actions fixture=%#v actual=%#v", fixture.ContinueAction, actual.ContinueAction)
	}
	if fixture.ContinueAction.Method != actual.ContinueAction.Method {
		t.Fatalf("continue method fixture=%q actual=%q", fixture.ContinueAction.Method, actual.ContinueAction.Method)
	}
	if fixture.ContinueAction.Endpoint != actual.ContinueAction.Endpoint {
		t.Fatalf("continue endpoint fixture=%q actual=%q", fixture.ContinueAction.Endpoint, actual.ContinueAction.Endpoint)
	}
	if fixture.ContinueAction.Label != actual.ContinueAction.Label {
		t.Fatalf("continue label fixture=%q actual=%q", fixture.ContinueAction.Label, actual.ContinueAction.Label)
	}
	if actual.ContinueAction.ActionToken == "" {
		t.Fatalf("expected actual action token")
	}
}

func decodeContractFixture(t *testing.T, path string, out any) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode fixture %s: %v", path, err)
	}
}
