package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type apiSession struct {
	SessionID string      `json:"session_id"`
	State     model.State `json:"state"`
}

type apiStatus struct {
	ServerTime      time.Time    `json:"server_time"`
	OverallState    model.State  `json:"overall_state"`
	ActiveSessionID string       `json:"active_session_id,omitempty"`
	SessionsCount   int          `json:"sessions_count"`
	Sessions        []apiSession `json:"sessions"`
}
