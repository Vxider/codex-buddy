//go:build uconsole_gui

package uconsole

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

const serversPreferenceKey = "uconsoleServers"

type BuddyServer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
}

func (s BuddyServer) DisplayName() string {
	if name := strings.TrimSpace(s.Name); name != "" {
		return name
	}
	if parsed, err := url.Parse(strings.TrimSpace(s.BaseURL)); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return strings.TrimSpace(s.BaseURL)
}

func loadServers(prefs fyne.Preferences, fallbackURL string) []BuddyServer {
	if prefs == nil {
		return seedServers(fallbackURL)
	}

	raw := prefs.StringWithFallback(serversPreferenceKey, "")
	if strings.TrimSpace(raw) == "" {
		servers := seedServers(fallbackURL)
		saveServers(prefs, servers)
		return servers
	}

	var servers []BuddyServer
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		servers = seedServers(fallbackURL)
		saveServers(prefs, servers)
		return servers
	}

	normalized := make([]BuddyServer, 0, len(servers))
	for _, server := range servers {
		baseURL, err := normalizedBaseURL(server.BaseURL)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(server.ID)
		if id == "" {
			id = newServerID()
		}
		normalized = append(normalized, BuddyServer{
			ID:      id,
			Name:    normalizedServerName(server.Name, baseURL),
			BaseURL: baseURL,
		})
	}

	if len(normalized) == 0 {
		normalized = seedServers(fallbackURL)
	}
	saveServers(prefs, normalized)
	return normalized
}

func saveServers(prefs fyne.Preferences, servers []BuddyServer) {
	if prefs == nil {
		return
	}
	data, err := json.Marshal(servers)
	if err != nil {
		return
	}
	prefs.SetString(serversPreferenceKey, string(data))
}

func seedServers(fallbackURL string) []BuddyServer {
	baseURL, err := normalizedBaseURL(fallbackURL)
	if err != nil {
		return nil
	}
	return []BuddyServer{{
		ID:      newServerID(),
		Name:    normalizedServerName("", baseURL),
		BaseURL: baseURL,
	}}
}

func normalizedBaseURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("enter a valid http:// or https:// server URL")
	}

	parts, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("enter a valid http:// or https:// server URL")
	}
	scheme := strings.ToLower(parts.Scheme)
	if (scheme != "http" && scheme != "https") || parts.Host == "" {
		return "", fmt.Errorf("enter a valid http:// or https:// server URL")
	}

	normalized := strings.TrimRight(parts.String(), "/")
	if normalized == "" {
		return "", fmt.Errorf("enter a valid http:// or https:// server URL")
	}
	return normalized, nil
}

func normalizedServerName(value, baseURL string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return baseURL
}

func newServerID() string {
	return fmt.Sprintf("srv-%d", time.Now().UnixNano())
}
