//go:build uconsole_gui

package uconsole

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/vxider/codex-buddy/internal/config"
)

const (
	serversPreferenceKey = "uconsoleServers"
	localServerID        = "local"
)

type BuddyServer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	BuiltIn bool   `json:"-"`
}

func (s BuddyServer) DisplayName() string {
	if s.IsLocal() {
		return "Local"
	}
	if name := strings.TrimSpace(s.Name); name != "" {
		return name
	}
	if parsed, err := url.Parse(strings.TrimSpace(s.BaseURL)); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return strings.TrimSpace(s.BaseURL)
}

func (s BuddyServer) IsLocal() bool {
	return s.BuiltIn && s.ID == localServerID
}

func configuredServersFromConfig(cfg config.Config, prefs fyne.Preferences) []BuddyServer {
	localBaseURL := strings.TrimRight(cfg.InternalBaseURL(), "/")
	normalized := normalizeRemoteServers(cfg.RemoteServers)
	if len(normalized) > 0 {
		return normalized
	}

	migrated := migrateLegacyServers(prefs, cfg.UConsole.ServerURL, localBaseURL)
	if len(migrated) > 0 {
		return migrated
	}
	return seedServers(cfg.UConsole.ServerURL, localBaseURL)
}

func localServer() BuddyServer {
	return BuddyServer{
		ID:      localServerID,
		Name:    "Local",
		BaseURL: strings.TrimRight(config.Default().InternalBaseURL(), "/"),
		BuiltIn: true,
	}
}

func normalizeRemoteServers(items []config.RemoteServerConfig) []BuddyServer {
	out := make([]BuddyServer, 0, len(items))
	seen := make(map[string]int, len(items))
	for _, server := range items {
		baseURL, err := normalizedBaseURL(server.BaseURL)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(server.ID)
		if id == "" || id == localServerID {
			id = newServerID()
		}
		item := BuddyServer{
			ID:      id,
			Name:    normalizedServerName(server.Name, baseURL),
			BaseURL: baseURL,
		}
		if index, ok := seen[item.BaseURL]; ok {
			out[index] = item
			continue
		}
		seen[item.BaseURL] = len(out)
		out = append(out, item)
	}
	return out
}

func migrateLegacyServers(prefs fyne.Preferences, fallbackURL string, localBaseURL string) []BuddyServer {
	if prefs == nil {
		return nil
	}

	raw := prefs.StringWithFallback(serversPreferenceKey, "")
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var servers []BuddyServer
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		return seedServers(fallbackURL, localBaseURL)
	}

	out := make([]BuddyServer, 0, len(servers))
	for _, server := range servers {
		baseURL, err := normalizedBaseURL(server.BaseURL)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(server.ID)
		if id == "" || id == localServerID {
			id = newServerID()
		}
		out = append(out, BuddyServer{
			ID:      id,
			Name:    normalizedServerName(server.Name, baseURL),
			BaseURL: baseURL,
		})
	}
	if len(out) == 0 {
		return seedServers(fallbackURL, localBaseURL)
	}
	return dedupeServers(out)
}

func persistLegacyServersClear(prefs fyne.Preferences) {
	if prefs == nil {
		return
	}
	prefs.SetString(serversPreferenceKey, "")
}

func seedServers(fallbackURL string, localBaseURL string) []BuddyServer {
	baseURL, err := normalizedBaseURL(fallbackURL)
	if err != nil {
		return nil
	}
	if baseURL == strings.TrimRight(localBaseURL, "/") {
		return nil
	}
	return []BuddyServer{{
		ID:      newServerID(),
		Name:    normalizedServerName("", baseURL),
		BaseURL: baseURL,
	}}
}

func dedupeServers(servers []BuddyServer) []BuddyServer {
	out := make([]BuddyServer, 0, len(servers))
	seen := make(map[string]int, len(servers))
	for _, server := range servers {
		if server.IsLocal() {
			continue
		}
		baseURL, err := normalizedBaseURL(server.BaseURL)
		if err != nil {
			continue
		}
		item := BuddyServer{
			ID:      firstNonEmptyText(strings.TrimSpace(server.ID), newServerID()),
			Name:    normalizedServerName(server.Name, baseURL),
			BaseURL: baseURL,
		}
		if index, ok := seen[item.BaseURL]; ok {
			out[index] = item
			continue
		}
		seen[item.BaseURL] = len(out)
		out = append(out, item)
	}
	return out
}

func remoteServerConfigs(servers []BuddyServer) []config.RemoteServerConfig {
	out := make([]config.RemoteServerConfig, 0, len(servers))
	for _, server := range dedupeServers(servers) {
		out = append(out, config.RemoteServerConfig{
			ID:      server.ID,
			Name:    server.Name,
			BaseURL: server.BaseURL,
		})
	}
	return out
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
