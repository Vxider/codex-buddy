//go:build uconsole_gui

package uconsole

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/vxider/codex-buddy/engine"
	"github.com/vxider/codex-buddy/engine/httpapi"
	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
)

type statusClient interface {
	LoadStatus(ctx context.Context) (StatusResponse, error)
	LoadNotifications(ctx context.Context) ([]NotificationResponse, error)
	AckNotification(ctx context.Context, id string) error
	ContinueNotification(ctx context.Context, item NotificationResponse) error
	ContinueSession(ctx context.Context, session SessionResponse) error
	SendSessionText(ctx context.Context, session SessionResponse, text string) error
	CloseSession(ctx context.Context, session SessionResponse) error
}

type LocalClient struct {
	handler http.Handler
	baseURL string
}

func NewLocalClient(cfg config.Config, runtime *engine.Runtime) *LocalClient {
	server := httpapi.NewServer(
		cfg,
		runtime.Store(),
		runtime.TranscriptManager(),
		runtime.ContinueExecutor(),
		runtime.SessionOpenChecker(),
		nil,
	)
	return &LocalClient{
		handler: server.Handler(),
		baseURL: "http://local",
	}
}

func (c *LocalClient) LoadStatus(ctx context.Context) (StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/status", nil)
	if err != nil {
		return StatusResponse{}, err
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return StatusResponse{}, fmt.Errorf("status request failed: %s", rec.Result().Status)
	}
	var status StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		return StatusResponse{}, err
	}
	return status, nil
}

func (c *LocalClient) LoadNotifications(ctx context.Context) ([]NotificationResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/notifications", nil)
	if err != nil {
		return nil, err
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("notifications request failed: %s", rec.Result().Status)
	}
	var out NotificationsResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Notifications, nil
}

func (c *LocalClient) AckNotification(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/notifications/"+url.PathEscape(id)+"/ack", nil)
	if err != nil {
		return err
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		return fmt.Errorf("ack failed: %s: %s", rec.Result().Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *LocalClient) ContinueNotification(ctx context.Context, item NotificationResponse) error {
	payload, err := json.Marshal(map[string]string{
		"action":       string(model.NotificationActionContinue),
		"action_token": item.ActionToken,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/notifications/"+url.PathEscape(item.ID)+"/action", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		return fmt.Errorf("continue failed: %s: %s", rec.Result().Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *LocalClient) ContinueSession(ctx context.Context, session SessionResponse) error {
	return c.executeSessionAction(ctx, session.ContinueAction, "session continue", "")
}

func (c *LocalClient) SendSessionText(ctx context.Context, session SessionResponse, text string) error {
	action := session.ContinueAction
	if action == nil && strings.TrimSpace(session.SessionID) != "" {
		action = &SessionActionPayload{
			Method:   http.MethodPost,
			Endpoint: "/v1/sessions/" + session.SessionID + "/continue",
		}
	}
	return c.executeSessionAction(ctx, action, "session text", text)
}

func (c *LocalClient) CloseSession(ctx context.Context, session SessionResponse) error {
	return c.executeSessionAction(ctx, session.CloseAction, "session close", "")
}

func (c *LocalClient) executeSessionAction(ctx context.Context, action *SessionActionPayload, actionName string, text string) error {
	if action == nil {
		return fmt.Errorf("%s is unavailable", actionName)
	}
	endpoint := strings.TrimSpace(action.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("%s endpoint is missing", actionName)
	}
	method := strings.ToUpper(strings.TrimSpace(action.Method))
	if method == "" {
		method = http.MethodPost
	}
	targetURL, err := url.Parse(c.baseURL + "/")
	if err != nil {
		return err
	}
	ref, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	resolved := targetURL.ResolveReference(ref).String()

	payload, err := json.Marshal(map[string]string{
		"action_token": strings.TrimSpace(action.ActionToken),
		"text":         strings.TrimSpace(text),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, resolved, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		return fmt.Errorf("%s failed: %s: %s", actionName, rec.Result().Status, strings.TrimSpace(string(body)))
	}
	return nil
}
