package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vxider/codex-buddy/engine"
	"github.com/vxider/codex-buddy/engine/httpapi"
	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
)

const localServerID = "local"

type statusResponse struct {
	ServerTime         time.Time         `json:"server_time"`
	OverallState       model.State       `json:"overall_state"`
	OverallStateDetail string            `json:"overall_state_detail,omitempty"`
	SessionsCount      int               `json:"sessions_count"`
	Sessions           []sessionResponse `json:"sessions"`
}

type sessionResponse struct {
	SessionID           string                 `json:"session_id"`
	ShortSessionID      string                 `json:"short_session_id,omitempty"`
	DisplayTitle        string                 `json:"display_title,omitempty"`
	State               model.State            `json:"state"`
	StateDetail         string                 `json:"state_detail,omitempty"`
	UpdatedAt           time.Time              `json:"updated_at,omitempty"`
	Summary             string                 `json:"summary,omitempty"`
	SummaryMarkdown     string                 `json:"summary_markdown,omitempty"`
	SummaryHTML         string                 `json:"summary_html,omitempty"`
	OpenSummary         string                 `json:"open_summary,omitempty"`
	OpenSummaryMarkdown string                 `json:"open_summary_markdown,omitempty"`
	OpenSummaryHTML     string                 `json:"open_summary_html,omitempty"`
	CanContinue         bool                   `json:"can_continue,omitempty"`
	ContinueAction      *continueActionPayload `json:"continue_action,omitempty"`
	ServerID            string                 `json:"-"`
	ServerName          string                 `json:"-"`
}

type continueActionPayload struct {
	Method      string `json:"method"`
	Endpoint    string `json:"endpoint"`
	ActionToken string `json:"action_token,omitempty"`
	Label       string `json:"label,omitempty"`
}

type notificationsResponse struct {
	Notifications []notificationResponse `json:"notifications"`
}

type notificationResponse struct {
	ID          string                     `json:"id"`
	SessionID   string                     `json:"session_id"`
	Kind        model.NotificationKind     `json:"kind"`
	State       model.NotificationState    `json:"state"`
	Title       string                     `json:"title"`
	Summary     string                     `json:"summary"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
	ActionToken string                     `json:"action_token,omitempty"`
	Actions     []model.NotificationAction `json:"actions,omitempty"`
	ServerID    string                     `json:"-"`
	ServerName  string                     `json:"-"`
}

type sourceClient interface {
	LoadStatus(ctx context.Context) (statusResponse, error)
	LoadNotifications(ctx context.Context) ([]notificationResponse, error)
	AckNotification(ctx context.Context, id string) error
	ContinueNotification(ctx context.Context, item notificationResponse) error
	ContinueSession(ctx context.Context, session sessionResponse) error
}

type remoteClient struct {
	baseURL string
	http    *http.Client
}

type localClient struct {
	handler http.Handler
	baseURL string
}

type serverTarget struct {
	ID      string
	Name    string
	BaseURL string
	Local   bool
	Client  sourceClient
}

type serverSnapshot struct {
	Target        serverTarget
	Status        statusResponse
	Notifications []notificationResponse
	Connected     bool
	Err           error
}

type App struct {
	cfg        config.Config
	configPath string
	logger     *log.Logger
	runtime    *engine.Runtime
	server     *engine.EmbeddedServer
	reader     *bufio.Reader
}

func Run(ctx context.Context, cfg config.Config, configPath string, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(os.Stdout, "codex-buddy-cli: ", log.LstdFlags|log.Lmsgprefix)
	}
	app := &App{
		cfg:        cfg,
		configPath: configPath,
		logger:     logger,
		runtime:    engine.NewRuntime(cfg, logger),
		reader:     bufio.NewReader(os.Stdin),
	}
	app.runtime.Start(ctx)
	app.server = engine.NewEmbeddedServer(app.runtime, cfg, logger)
	if cfg.LocalServer.Enabled {
		if err := app.server.Start(ctx, cfg); err != nil {
			logger.Printf("local server start failed: %v", err)
		}
	}

	for {
		rendered, err := app.render(ctx)
		if err != nil {
			return err
		}
		fmt.Print(rendered)
		fmt.Print("\ncmd> ")
		line, err := app.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "r") || strings.EqualFold(line, "refresh") {
			continue
		}
		if err := app.handleCommand(ctx, line); err != nil {
			fmt.Printf("\nerror: %v\n", err)
			time.Sleep(1200 * time.Millisecond)
		}
	}
}

func (a *App) render(ctx context.Context) (string, error) {
	snapshots := a.loadAll(ctx)
	var b strings.Builder
	b.WriteString("\033[H\033[2J")
	b.WriteString("codex-buddy cli\n")
	b.WriteString("================\n")
	b.WriteString(fmt.Sprintf("local server: %s\n", a.serverSummary()))
	b.WriteString(fmt.Sprintf("listen: http://%s\n", a.cfg.Listen.Address()))
	b.WriteString(fmt.Sprintf("remote servers: %d\n\n", len(a.cfg.RemoteServers)))

	overall := aggregateState(snapshots)
	b.WriteString(fmt.Sprintf("overall: %s\n", overall))
	b.WriteString("servers:\n")
	for _, snapshot := range snapshots {
		line := fmt.Sprintf("- %s", snapshot.Target.Name)
		if snapshot.Connected {
			line += fmt.Sprintf(" [%s]", snapshot.Status.OverallState)
		} else {
			line += " [offline]"
		}
		if snapshot.Err != nil {
			line += " " + brief(snapshot.Err.Error(), 96)
		}
		b.WriteString(line + "\n")
	}

	sessions := aggregateSessions(snapshots)
	b.WriteString("\nsessions:\n")
	if len(sessions) == 0 {
		b.WriteString("- none\n")
	}
	for _, session := range sessions {
		title := firstNonEmpty(session.DisplayTitle, session.ShortSessionID, session.SessionID)
		summary := firstNonEmpty(session.OpenSummary, session.Summary)
		line := fmt.Sprintf("- %s [%s] (%s) %s", title, session.State, session.ServerName, session.UpdatedAt.Local().Format("15:04:05"))
		if summary != "" {
			line += " " + brief(summary, 120)
		}
		if session.CanContinue {
			line += " {continue}"
		}
		b.WriteString(line + "\n")
	}

	notifs := aggregateNotifications(snapshots)
	b.WriteString("\nnotifications:\n")
	if len(notifs) == 0 {
		b.WriteString("- none\n")
	}
	for _, item := range notifs {
		b.WriteString(fmt.Sprintf("- %s [%s] (%s) %s\n", item.ID, item.Kind, item.ServerName, brief(item.Summary, 120)))
	}

	b.WriteString("\ncommands:\n")
	b.WriteString("  r|refresh                refresh screen\n")
	b.WriteString("  v|server                 toggle local server\n")
	b.WriteString("  listen <host> <port>     update listen host/port\n")
	b.WriteString("  ack <notif-prefix>       acknowledge notification\n")
	b.WriteString("  c <session-prefix>       continue session\n")
	b.WriteString("  add <url> [name]         add remote server\n")
	b.WriteString("  del <id|name>            delete remote server\n")
	b.WriteString("  q|quit                   quit\n")
	return b.String(), nil
}

func (a *App) handleCommand(ctx context.Context, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	switch strings.ToLower(fields[0]) {
	case "q", "quit", "exit":
		os.Exit(0)
	case "v", "server":
		return a.toggleServer(ctx)
	case "listen":
		if len(fields) != 3 {
			return fmt.Errorf("usage: listen <host> <port>")
		}
		port, err := strconv.Atoi(fields[2])
		if err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("enter a valid port")
		}
		next := a.cfg
		next.Listen.Host = fields[1]
		next.Listen.Port = port
		if a.server.Running() {
			if err := a.server.Start(ctx, next); err != nil {
				return err
			}
		}
		return a.saveConfig(next)
	case "ack":
		if len(fields) != 2 {
			return fmt.Errorf("usage: ack <notif-prefix>")
		}
		item, client, err := a.findNotification(ctx, fields[1])
		if err != nil {
			return err
		}
		return client.AckNotification(ctx, item.ID)
	case "c", "continue":
		if len(fields) != 2 {
			return fmt.Errorf("usage: c <session-prefix>")
		}
		session, client, err := a.findSession(ctx, fields[1])
		if err != nil {
			return err
		}
		return client.ContinueSession(ctx, session)
	case "add":
		if len(fields) < 2 {
			return fmt.Errorf("usage: add <url> [name]")
		}
		baseURL, err := normalizeBaseURL(fields[1])
		if err != nil {
			return err
		}
		name := ""
		if len(fields) > 2 {
			name = strings.Join(fields[2:], " ")
		}
		next := a.cfg
		next.RemoteServers = append(next.RemoteServers, config.RemoteServerConfig{
			ID:      newServerID(),
			Name:    normalizeServerName(name, baseURL),
			BaseURL: baseURL,
		})
		next.RemoteServers = dedupeRemoteServers(next.RemoteServers)
		return a.saveConfig(next)
	case "del":
		if len(fields) != 2 {
			return fmt.Errorf("usage: del <id|name>")
		}
		target := strings.ToLower(fields[1])
		next := a.cfg
		filtered := make([]config.RemoteServerConfig, 0, len(next.RemoteServers))
		for _, server := range next.RemoteServers {
			if strings.HasPrefix(strings.ToLower(server.ID), target) || strings.EqualFold(server.Name, fields[1]) {
				continue
			}
			filtered = append(filtered, server)
		}
		next.RemoteServers = filtered
		return a.saveConfig(next)
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
	}
	return nil
}

func (a *App) toggleServer(ctx context.Context) error {
	next := a.cfg
	if a.server.Running() {
		if err := a.server.Stop(ctx); err != nil {
			return err
		}
		next.LocalServer.Enabled = false
	} else {
		if err := a.server.Start(ctx, next); err != nil {
			return err
		}
		next.LocalServer.Enabled = true
	}
	return a.saveConfig(next)
}

func (a *App) saveConfig(next config.Config) error {
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next
	return nil
}

func (a *App) serverSummary() string {
	if a.server != nil && a.server.Running() {
		return "on (" + a.server.Address() + ")"
	}
	return "off"
}

func (a *App) loadAll(ctx context.Context) []serverSnapshot {
	targets := a.targets()
	snapshots := make([]serverSnapshot, 0, len(targets))
	for _, target := range targets {
		status, err := target.Client.LoadStatus(ctx)
		snapshot := serverSnapshot{
			Target: target,
			Err:    err,
		}
		if err == nil {
			snapshot.Connected = true
			snapshot.Status = status
			notifications, notifErr := target.Client.LoadNotifications(ctx)
			snapshot.Notifications = notifications
			if notifErr != nil {
				snapshot.Err = notifErr
			}
		}
		annotateSnapshot(&snapshot)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (a *App) targets() []serverTarget {
	targets := []serverTarget{{
		ID:      localServerID,
		Name:    "Local",
		BaseURL: strings.TrimRight(a.cfg.InternalBaseURL(), "/"),
		Local:   true,
		Client:  newLocalCLIClient(a.cfg, a.runtime),
	}}

	servers := a.cfg.RemoteServers
	if len(servers) == 0 && strings.TrimRight(a.cfg.UConsole.ServerURL, "/") != strings.TrimRight(a.cfg.InternalBaseURL(), "/") {
		servers = append(servers, config.RemoteServerConfig{
			ID:      newServerID(),
			Name:    normalizeServerName("", a.cfg.UConsole.ServerURL),
			BaseURL: a.cfg.UConsole.ServerURL,
		})
	}

	for _, server := range dedupeRemoteServers(servers) {
		targets = append(targets, serverTarget{
			ID:      server.ID,
			Name:    firstNonEmpty(server.Name, server.BaseURL),
			BaseURL: server.BaseURL,
			Client:  newRemoteCLIClient(server.BaseURL),
		})
	}
	return targets
}

func (a *App) findNotification(ctx context.Context, prefix string) (notificationResponse, sourceClient, error) {
	for _, snapshot := range a.loadAll(ctx) {
		for _, item := range snapshot.Notifications {
			if strings.HasPrefix(strings.ToLower(item.ID), strings.ToLower(prefix)) {
				return item, snapshot.Target.Client, nil
			}
		}
	}
	return notificationResponse{}, nil, fmt.Errorf("notification not found")
}

func (a *App) findSession(ctx context.Context, prefix string) (sessionResponse, sourceClient, error) {
	for _, snapshot := range a.loadAll(ctx) {
		for _, session := range snapshot.Status.Sessions {
			if strings.HasPrefix(strings.ToLower(session.SessionID), strings.ToLower(prefix)) ||
				strings.HasPrefix(strings.ToLower(session.ShortSessionID), strings.ToLower(prefix)) {
				return session, snapshot.Target.Client, nil
			}
		}
	}
	return sessionResponse{}, nil, fmt.Errorf("session not found")
}

func annotateSnapshot(snapshot *serverSnapshot) {
	if snapshot == nil {
		return
	}
	for i := range snapshot.Status.Sessions {
		snapshot.Status.Sessions[i].ServerID = snapshot.Target.ID
		snapshot.Status.Sessions[i].ServerName = snapshot.Target.Name
	}
	for i := range snapshot.Notifications {
		snapshot.Notifications[i].ServerID = snapshot.Target.ID
		snapshot.Notifications[i].ServerName = snapshot.Target.Name
	}
}

func aggregateState(snapshots []serverSnapshot) model.State {
	best := model.StateOffline
	bestRank := -1
	for _, snapshot := range snapshots {
		if !snapshot.Connected {
			continue
		}
		rank := stateRank(snapshot.Status.OverallState)
		if rank > bestRank {
			bestRank = rank
			best = snapshot.Status.OverallState
		}
	}
	return best
}

func aggregateSessions(snapshots []serverSnapshot) []sessionResponse {
	out := make([]sessionResponse, 0, 16)
	for _, snapshot := range snapshots {
		out = append(out, snapshot.Status.Sessions...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if stateRank(out[i].State) != stateRank(out[j].State) {
			return stateRank(out[i].State) > stateRank(out[j].State)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func aggregateNotifications(snapshots []serverSnapshot) []notificationResponse {
	out := make([]notificationResponse, 0, 16)
	for _, snapshot := range snapshots {
		out = append(out, snapshot.Notifications...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func stateRank(state model.State) int {
	switch state {
	case model.StateError:
		return 5
	case model.StateAttention:
		return 4
	case model.StateRunningBash:
		return 3
	case model.StateRunning:
		return 2
	case model.StateIdle:
		return 1
	default:
		return 0
	}
}

func newRemoteCLIClient(baseURL string) *remoteClient {
	return &remoteClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *remoteClient) LoadStatus(ctx context.Context) (statusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/status", nil)
	if err != nil {
		return statusResponse{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return statusResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusResponse{}, fmt.Errorf("status request failed: %s", resp.Status)
	}
	var status statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return statusResponse{}, err
	}
	return status, nil
}

func (c *remoteClient) LoadNotifications(ctx context.Context) ([]notificationResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/notifications", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notifications request failed: %s", resp.Status)
	}
	var out notificationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Notifications, nil
}

func (c *remoteClient) AckNotification(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/notifications/"+url.PathEscape(id)+"/ack", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ack failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *remoteClient) ContinueNotification(ctx context.Context, item notificationResponse) error {
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
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("continue failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *remoteClient) ContinueSession(ctx context.Context, session sessionResponse) error {
	if session.ContinueAction == nil {
		return fmt.Errorf("session continue is unavailable")
	}
	payload, err := json.Marshal(map[string]string{
		"action_token": session.ContinueAction.ActionToken,
	})
	if err != nil {
		return err
	}
	base, err := url.Parse(c.baseURL + "/")
	if err != nil {
		return err
	}
	ref, err := url.Parse(session.ContinueAction.Endpoint)
	if err != nil {
		return err
	}
	method := session.ContinueAction.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, base.ResolveReference(ref).String(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("session continue failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func newLocalCLIClient(cfg config.Config, runtime *engine.Runtime) *localClient {
	server := httpapi.NewServer(
		cfg,
		runtime.Store(),
		runtime.TranscriptManager(),
		runtime.ContinueExecutor(),
		runtime.SessionOpenChecker(),
		nil,
	)
	return &localClient{handler: server.Handler(), baseURL: "http://local"}
}

func (c *localClient) LoadStatus(ctx context.Context) (statusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/status", nil)
	if err != nil {
		return statusResponse{}, err
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return statusResponse{}, fmt.Errorf("status request failed: %s", rec.Result().Status)
	}
	var status statusResponse
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		return statusResponse{}, err
	}
	return status, nil
}

func (c *localClient) LoadNotifications(ctx context.Context) ([]notificationResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/notifications", nil)
	if err != nil {
		return nil, err
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("notifications request failed: %s", rec.Result().Status)
	}
	var out notificationsResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Notifications, nil
}

func (c *localClient) AckNotification(ctx context.Context, id string) error {
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

func (c *localClient) ContinueNotification(ctx context.Context, item notificationResponse) error {
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

func (c *localClient) ContinueSession(ctx context.Context, session sessionResponse) error {
	if session.ContinueAction == nil {
		return fmt.Errorf("session continue is unavailable")
	}
	payload, err := json.Marshal(map[string]string{
		"action_token": session.ContinueAction.ActionToken,
	})
	if err != nil {
		return err
	}
	base, _ := url.Parse(c.baseURL + "/")
	ref, err := url.Parse(session.ContinueAction.Endpoint)
	if err != nil {
		return err
	}
	method := session.ContinueAction.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, base.ResolveReference(ref).String(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		return fmt.Errorf("session continue failed: %s: %s", rec.Result().Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func normalizeBaseURL(value string) (string, error) {
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
	return strings.TrimRight(parts.String(), "/"), nil
}

func normalizeServerName(value, baseURL string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return baseURL
}

func dedupeRemoteServers(servers []config.RemoteServerConfig) []config.RemoteServerConfig {
	out := make([]config.RemoteServerConfig, 0, len(servers))
	seen := make(map[string]int, len(servers))
	for _, server := range servers {
		baseURL, err := normalizeBaseURL(server.BaseURL)
		if err != nil {
			continue
		}
		item := config.RemoteServerConfig{
			ID:      firstNonEmpty(server.ID, newServerID()),
			Name:    normalizeServerName(server.Name, baseURL),
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

func newServerID() string {
	return fmt.Sprintf("srv-%d", time.Now().UnixNano())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func brief(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
