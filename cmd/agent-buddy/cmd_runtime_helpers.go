package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vxider/agent-buddy/internal/config"
	"github.com/vxider/agent-buddy/internal/model"
)

func fetchStatus(cfg config.Config) (apiStatus, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, cfg.InternalBaseURL()+"/v1/status", nil)
	if err != nil {
		return apiStatus{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return apiStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiStatus{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var status apiStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return apiStatus{}, err
	}
	return status, nil
}

func requestShutdown(cfg config.Config) error {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodPost, cfg.ShutdownURL(), bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		bodyText := strings.TrimSpace(string(body))
		if bodyText == "" {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, bodyText)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type apiSession struct {
	SessionID         string          `json:"session_id"`
	State             model.State     `json:"state"`
	CodexInterruption bool            `json:"codex_interruption,omitempty"`
	TmuxWindow        string          `json:"tmux_window,omitempty"`
	GoalState         model.GoalState `json:"goal_state,omitempty"`
}

type apiStatus struct {
	ServerTime    time.Time    `json:"server_time"`
	OverallState  model.State  `json:"overall_state"`
	SessionsCount int          `json:"sessions_count"`
	Sessions      []apiSession `json:"sessions"`
}
