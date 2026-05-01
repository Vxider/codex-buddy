package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
	"github.com/vxider/codex-buddy/webserver/internal/control"
	"github.com/vxider/codex-buddy/webserver/internal/present"
	"github.com/vxider/codex-buddy/webserver/internal/store"
	"github.com/vxider/codex-buddy/webserver/internal/transcript"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

type Server struct {
	cfg        config.Config
	store      *store.Store
	transcript *transcript.Manager
	control    control.ContinueExecutor
	openCheck  control.SessionOpenChecker
	logger     *log.Logger
	shutdown   func()
}

type publicSession struct {
	SessionID       string               `json:"session_id"`
	ShortSessionID  string               `json:"short_session_id,omitempty"`
	DisplayTitle    string               `json:"display_title,omitempty"`
	CompactTitle    string               `json:"compact_title,omitempty"`
	MicroTitle      string               `json:"micro_title,omitempty"`
	State           model.State          `json:"state"`
	StateDetail     string               `json:"state_detail,omitempty"`
	UpdatedAt       time.Time            `json:"updated_at"`
	Summary         string               `json:"summary,omitempty"`
	SummaryMarkdown string               `json:"summary_markdown,omitempty"`
	SummaryHTML     string               `json:"summary_html,omitempty"`
	CompactSummary  string               `json:"compact_summary,omitempty"`
	MicroSummary    string               `json:"micro_summary,omitempty"`
	NeedsOpen       bool                 `json:"needs_open"`
	NeedsApproval   bool                 `json:"needs_approval"`
	OpenReason      string               `json:"open_reason,omitempty"`
	OpenSummary     string               `json:"open_summary,omitempty"`
	OpenMarkdown    string               `json:"open_summary_markdown,omitempty"`
	OpenHTML        string               `json:"open_summary_html,omitempty"`
	CompactOpen     string               `json:"compact_open_summary,omitempty"`
	MicroOpen       string               `json:"micro_open_summary,omitempty"`
	TmuxSession     string               `json:"tmux_session,omitempty"`
	TmuxWindow      string               `json:"tmux_window,omitempty"`
	TmuxPane        string               `json:"tmux_pane,omitempty"`
	CanContinue     bool                 `json:"can_continue"`
	ContinueAction  *publicSessionAction `json:"continue_action,omitempty"`
	CanClose        bool                 `json:"can_close"`
	CloseAction     *publicSessionAction `json:"close_action,omitempty"`
}

type publicSessionAction struct {
	Method      string `json:"method"`
	Endpoint    string `json:"endpoint"`
	ActionToken string `json:"action_token,omitempty"`
	Label       string `json:"label,omitempty"`
}

type publicStatus struct {
	ServerTime         time.Time       `json:"server_time"`
	OverallState       model.State     `json:"overall_state"`
	OverallStateDetail string          `json:"overall_state_detail,omitempty"`
	SessionsCount      int             `json:"sessions_count"`
	Sessions           []publicSession `json:"sessions"`
}

type publicNotification struct {
	ID              string                     `json:"id"`
	SessionID       string                     `json:"session_id"`
	Kind            model.NotificationKind     `json:"kind"`
	State           model.NotificationState    `json:"state"`
	Title           string                     `json:"title"`
	Summary         string                     `json:"summary"`
	SummaryMarkdown string                     `json:"summary_markdown,omitempty"`
	SummaryHTML     string                     `json:"summary_html,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
	ActionToken     string                     `json:"action_token,omitempty"`
	Actions         []model.NotificationAction `json:"actions,omitempty"`
}

type sessionTitleSet struct {
	Display string
	Compact string
	Micro   string
}

func NewServer(cfg config.Config, sessionStore *store.Store, transcriptManager *transcript.Manager, executor control.ContinueExecutor, openCheck control.SessionOpenChecker, logger *log.Logger) *Server {
	return &Server{
		cfg:        cfg,
		store:      sessionStore,
		transcript: transcriptManager,
		control:    executor,
		openCheck:  openCheck,
		logger:     logger,
	}
}

func (s *Server) SetShutdownFunc(fn func()) {
	s.shutdown = fn
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleDebug)
	mux.HandleFunc("/debug/codex", s.handleCodexDebug)
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/notifications", s.handleNotifications)
	mux.HandleFunc("/v1/notifications/", s.handleNotificationAction)
	mux.HandleFunc("/v1/sessions", s.handleSessions)
	mux.HandleFunc("/v1/sessions/", s.handleSession)
	mux.HandleFunc("/v1/stream", s.handleStream)
	mux.HandleFunc("/v1/codex/connect", s.handleCodexControlDisabled)
	mux.HandleFunc("/v1/codex/status", s.handleCodexControlDisabled)
	mux.HandleFunc("/v1/codex/thread/start", s.handleCodexControlDisabled)
	mux.HandleFunc("/v1/codex/turn/start", s.handleCodexControlDisabled)
	mux.HandleFunc("/v1/codex/stream", s.handleCodexControlDisabled)
	mux.Handle("/v1/internal/hooks", http.HandlerFunc(s.handleInternalHooks))
	mux.Handle("/v1/internal/shutdown", http.HandlerFunc(s.handleInternalShutdown))

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().UTC(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.publicStatus(s.decorateSnapshot(s.store.Snapshot())))
}

func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(debugPageHTML))
}

func (s *Server) handleCodexDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(codexDisabledPageHTML))
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": s.publicNotifications(s.visibleNotifications()),
	})
}

func (s *Server) handleNotificationAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/notifications/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}

	id := parts[0]
	switch parts[1] {
	case "ack":
		notification, ok := s.store.AckNotification(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, s.publicNotification(notification))
	case "action":
		var req struct {
			Action      model.NotificationAction `json:"action"`
			ActionToken string                   `json:"action_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if req.Action != model.NotificationActionContinue {
			http.Error(w, "unsupported action", http.StatusBadRequest)
			return
		}
		if s.control == nil {
			http.Error(w, "continue action is not configured", http.StatusNotImplemented)
			return
		}
		notification, session, ok := s.store.ContinueTarget(id, req.ActionToken)
		if !ok || !s.isSessionVisible(session) {
			http.Error(w, "notification is no longer actionable", http.StatusConflict)
			return
		}
		if err := s.control.Continue(session, model.ContinueCommandText); err != nil {
			http.Error(w, fmt.Sprintf("continue failed: %v", err), http.StatusBadGateway)
			return
		}
		updated, ok := s.store.MarkNotificationActed(id)
		if !ok {
			http.Error(w, "notification disappeared after action", http.StatusConflict)
			return
		}
		if s.logger != nil {
			s.logger.Printf("continue action sent session=%s notification=%s", notification.SessionID, notification.ID)
		}
		writeJSON(w, http.StatusOK, s.publicNotification(updated))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	sessions := s.visibleSessions(s.store.Sessions())
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": s.publicSessions(sessions, s.notificationIndex(), sessionTitles(sessions)),
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		sessionID := parts[0]
		if sessionID == "" {
			http.NotFound(w, r)
			return
		}

		session, ok := s.store.Session(sessionID)
		if !ok || !s.isSessionVisible(session) {
			http.NotFound(w, r)
			return
		}

		writeJSON(w, http.StatusOK, s.publicSession(session, s.notificationIndex()[sessionID], sessionTitles([]model.SessionSnapshot{session})[sessionID]))
	case len(parts) == 2 && parts[1] == "continue" && r.Method == http.MethodPost:
		s.handleSessionContinue(w, r, parts[0])
	case len(parts) == 2 && parts[1] == "close" && r.Method == http.MethodPost:
		s.handleSessionClose(w, r, parts[0])
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	updates, cancel := s.store.Subscribe()
	defer cancel()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case snapshot, ok := <-updates:
			if !ok {
				return
			}
			if err := writeSSE(w, "status", s.publicStatus(s.decorateSnapshot(snapshot))); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleInternalHooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.Internal.RequireLoopback && !isLoopbackRequest(r.RemoteAddr) {
		http.Error(w, "loopback required", http.StatusForbidden)
		return
	}

	var req model.IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	snapshot := s.store.ApplyIngest(req)
	if req.Payload.SessionID != "" && req.Payload.TranscriptPath != "" {
		s.transcript.Ensure(req.Payload.SessionID, req.Payload.TranscriptPath)
	}

	writeJSON(w, http.StatusAccepted, snapshot)
}

func (s *Server) handleInternalShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.Internal.RequireLoopback && !isLoopbackRequest(r.RemoteAddr) {
		http.Error(w, "loopback required", http.StatusForbidden)
		return
	}

	if s.shutdown == nil {
		http.Error(w, "shutdown is not configured", http.StatusNotImplemented)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": "shutdown requested",
	})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go s.shutdown()
}

func (s *Server) handleCodexControlDisabled(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusGone, map[string]any{
		"ok":      false,
		"error":   "codex app-server control endpoints are disabled in passive monitor mode",
		"message": "use /status, /v1/status, and /v1/stream to observe the locally running codex cli session without creating threads or sending turns",
	})
}

func (s *Server) decorateSnapshot(snapshot model.StatusSnapshot) model.StatusSnapshot {
	snapshot.Sessions = s.visibleSessions(snapshot.Sessions)
	snapshot.SessionsCount = len(snapshot.Sessions)
	if len(snapshot.Sessions) == 0 {
		snapshot.ActiveSessionID = ""
		snapshot.OverallState = model.StateIdle
		snapshot.OverallStateDetail = string(model.StateIdle)
	} else {
		active := snapshot.Sessions[0]
		snapshot.ActiveSessionID = active.SessionID
		snapshot.OverallState = active.State
		snapshot.OverallStateDetail = active.StateDetail
	}
	if s.transcript != nil {
		snapshot.TranscriptWatchers = s.transcript.Snapshot()
	}
	return snapshot
}

func (s *Server) publicStatus(snapshot model.StatusSnapshot) publicStatus {
	notifications := s.notificationIndex()
	titles := sessionTitles(snapshot.Sessions)

	return publicStatus{
		ServerTime:         snapshot.ServerTime,
		OverallState:       snapshot.OverallState,
		OverallStateDetail: snapshot.OverallStateDetail,
		SessionsCount:      len(snapshot.Sessions),
		Sessions:           s.publicSessions(snapshot.Sessions, notifications, titles),
	}
}

func (s *Server) publicSessions(sessions []model.SessionSnapshot, notifications map[string]model.NotificationSnapshot, titles map[string]sessionTitleSet) []publicSession {
	items := make([]publicSession, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, s.publicSession(session, notifications[session.SessionID], titles[session.SessionID]))
	}
	return items
}

func (s *Server) publicSession(session model.SessionSnapshot, notification model.NotificationSnapshot, titles sessionTitleSet) publicSession {
	fullSummary := sessionSummary(session)
	summaryMarkdown, summaryHTML := renderRichText(fullSummary)
	openText := firstNonEmpty(session.LastAssistantMessageFull, session.LastAssistantMessage, session.LastUserPromptPreview)
	openReason := ""
	if session.State == model.StateAttention {
		openReason = classifyOpenReason(openText)
	}
	item := publicSession{
		SessionID:       session.SessionID,
		ShortSessionID:  shortSessionID(session.SessionID),
		DisplayTitle:    titles.Display,
		CompactTitle:    titles.Compact,
		MicroTitle:      titles.Micro,
		State:           session.State,
		StateDetail:     session.StateDetail,
		UpdatedAt:       session.UpdatedAt,
		Summary:         fullSummary,
		SummaryMarkdown: summaryMarkdown,
		SummaryHTML:     summaryHTML,
		CompactSummary:  compactSummary(fullSummary, false),
		MicroSummary:    microSummary(fullSummary, false),
		NeedsOpen:       session.State == model.StateAttention,
		NeedsApproval:   openReason == "approval",
		OpenReason:      openReason,
		TmuxSession:     strings.TrimSpace(session.TmuxSession),
		TmuxWindow:      strings.TrimSpace(session.TmuxWindow),
		TmuxPane:        strings.TrimSpace(session.TmuxPane),
	}

	if notification.ID != "" {
		item.OpenSummary = notification.Summary
		item.OpenMarkdown, item.OpenHTML = renderRichText(notification.Summary)
		item.CompactOpen = compactSummary(notification.Summary, true)
		item.MicroOpen = microSummary(notification.Summary, true)
		item.NeedsOpen = notification.Kind == model.NotificationAttention
		item.OpenReason = classifyOpenReason(notification.Summary)
		item.NeedsApproval = item.OpenReason == "approval"
		item.CanContinue = slices.Contains(notification.Actions, model.NotificationActionContinue)
		if item.CanContinue {
			item.ContinueAction = &publicSessionAction{
				Method:      http.MethodPost,
				Endpoint:    "/v1/sessions/" + session.SessionID + "/continue",
				ActionToken: notification.ActionToken,
				Label:       "Continue",
			}
		}
	}
	if closer, ok := s.control.(control.SessionCloser); ok && closer != nil && item.TmuxPane != "" {
		item.CanClose = true
		item.CloseAction = &publicSessionAction{
			Method:   http.MethodPost,
			Endpoint: "/v1/sessions/" + session.SessionID + "/close",
			Label:    "Close",
		}
	}

	return item
}

func (s *Server) publicNotifications(items []model.NotificationSnapshot) []publicNotification {
	out := make([]publicNotification, 0, len(items))
	for _, item := range items {
		out = append(out, s.publicNotification(item))
	}
	return out
}

func (s *Server) publicNotification(item model.NotificationSnapshot) publicNotification {
	summaryMarkdown, summaryHTML := renderRichText(item.Summary)
	return publicNotification{
		ID:              item.ID,
		SessionID:       item.SessionID,
		Kind:            item.Kind,
		State:           item.State,
		Title:           item.Title,
		Summary:         item.Summary,
		SummaryMarkdown: summaryMarkdown,
		SummaryHTML:     summaryHTML,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
		ActionToken:     item.ActionToken,
		Actions:         item.Actions,
	}
}

func (s *Server) handleSessionContinue(w http.ResponseWriter, r *http.Request, sessionID string) {
	if sessionID == "" {
		http.NotFound(w, r)
		return
	}
	if s.control == nil {
		http.Error(w, "continue action is not configured", http.StatusNotImplemented)
		return
	}

	var req struct {
		ActionToken string `json:"action_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	req.ActionToken = strings.TrimSpace(req.ActionToken)
	if req.ActionToken == "" {
		http.Error(w, "missing action_token", http.StatusBadRequest)
		return
	}

	notification, session, ok := s.continueTargetForSession(sessionID, req.ActionToken)
	if !ok {
		http.Error(w, "session is no longer actionable", http.StatusConflict)
		return
	}
	if err := s.control.Continue(session, model.ContinueCommandText); err != nil {
		http.Error(w, fmt.Sprintf("continue failed: %v", err), http.StatusBadGateway)
		return
	}
	if _, ok := s.store.MarkNotificationActed(notification.ID); !ok {
		http.Error(w, "notification disappeared after action", http.StatusConflict)
		return
	}
	if s.logger != nil {
		s.logger.Printf("continue action sent session=%s notification=%s", notification.SessionID, notification.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "continue sent",
		"session": s.publicSession(session, model.NotificationSnapshot{}, sessionTitles([]model.SessionSnapshot{session})[session.SessionID]),
		"status":  s.publicStatus(s.decorateSnapshot(s.store.Snapshot())),
	})
}

func (s *Server) handleSessionClose(w http.ResponseWriter, r *http.Request, sessionID string) {
	if sessionID == "" {
		http.NotFound(w, r)
		return
	}

	closer, ok := s.control.(control.SessionCloser)
	if !ok || closer == nil {
		http.Error(w, "close action is not configured", http.StatusNotImplemented)
		return
	}

	session, ok := s.store.Session(sessionID)
	if !ok || !s.isSessionVisible(session) {
		http.NotFound(w, r)
		return
	}

	if err := closer.Close(session); err != nil {
		http.Error(w, fmt.Sprintf("close failed: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "close sent",
		"session": s.publicSession(session, model.NotificationSnapshot{}, sessionTitles([]model.SessionSnapshot{session})[session.SessionID]),
		"status":  s.publicStatus(s.decorateSnapshot(s.store.Snapshot())),
	})
}

func (s *Server) notificationIndex() map[string]model.NotificationSnapshot {
	items := s.visibleNotifications()
	out := make(map[string]model.NotificationSnapshot, len(items))
	for _, item := range items {
		out[item.SessionID] = item
	}
	return out
}

func (s *Server) continueTargetForSession(sessionID, token string) (model.NotificationSnapshot, model.SessionSnapshot, bool) {
	for _, notification := range s.visibleNotifications() {
		if notification.SessionID != sessionID {
			continue
		}
		if notification.ActionToken != token {
			continue
		}
		targetNotification, session, ok := s.store.ContinueTarget(notification.ID, token)
		if !ok || !s.isSessionVisible(session) {
			return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
		}
		return targetNotification, session, true
	}
	notification, session, ok := s.store.ContinueTargetForSession(sessionID)
	if !ok || !s.isSessionVisible(session) {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}
	return notification, session, true
}

func (s *Server) visibleSessions(sessions []model.SessionSnapshot) []model.SessionSnapshot {
	if len(sessions) == 0 {
		return sessions
	}

	items := make([]model.SessionSnapshot, 0, len(sessions))
	for _, session := range sessions {
		if !s.isSessionVisible(session) {
			continue
		}
		items = append(items, session)
	}
	return items
}

func (s *Server) visibleNotifications() []model.NotificationSnapshot {
	items := s.store.Notifications()
	if len(items) == 0 {
		return items
	}

	visible := make([]model.NotificationSnapshot, 0, len(items))
	for _, item := range items {
		session, ok := s.store.Session(item.SessionID)
		if !ok || !s.isSessionVisible(session) {
			continue
		}
		visible = append(visible, item)
	}
	return visible
}

func (s *Server) isSessionVisible(session model.SessionSnapshot) bool {
	if s.openCheck == nil {
		return true
	}
	return s.openCheck.IsOpen(session)
}

func shortSessionID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 10 {
		return value
	}
	return value[:8]
}

func sessionTitles(sessions []model.SessionSnapshot) map[string]sessionTitleSet {
	baseCounts := make(map[string]int, len(sessions))
	baseBySession := make(map[string]string, len(sessions))
	for _, session := range sessions {
		base := workspaceBaseTitle(session)
		baseBySession[session.SessionID] = base
		if base != "" {
			baseCounts[base]++
		}
	}

	out := make(map[string]sessionTitleSet, len(sessions))
	for _, session := range sessions {
		base := baseBySession[session.SessionID]
		short := shortSessionID(session.SessionID)
		display := base
		compact := compactTitle(base)
		micro := microTitle(base)

		if base == "" {
			display = short
			compact = short
			micro = short
		} else if baseCounts[base] > 1 && short != "" {
			display = titleWithSuffix(base, short, 999)
			compact = titleWithSuffix(base, short, 42)
			micro = titleWithSuffix(base, short, 24)
		}

		out[session.SessionID] = sessionTitleSet{
			Display: display,
			Compact: compact,
			Micro:   micro,
		}
	}
	return out
}

func workspaceBaseTitle(session model.SessionSnapshot) string {
	if value := strings.TrimSpace(session.CWD); value != "" {
		base := strings.TrimSpace(filepath.Base(value))
		if base != "" && base != "." && base != "/" {
			return base
		}
		return value
	}
	return shortSessionID(session.SessionID)
}

func sessionSummary(session model.SessionSnapshot) string {
	if session.State == model.StateError {
		return present.ErrorSummary(session)
	}
	return firstNonEmpty(session.LastAssistantMessage, session.LastBashCommand, session.LastUserPromptPreview)
}

func classifyOpenReason(message string) string {
	if needsApprovalFromMessage(message) {
		return "approval"
	}
	if strings.TrimSpace(message) != "" {
		return "followup"
	}
	return ""
}

func needsApprovalFromMessage(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}

	approvalMarkers := []string{
		"approval required",
		"approve",
		"approval",
		"need approval",
		"need confirmation",
		"confirm ",
		"confirmation",
		"waiting for",
		"before overwriting",
		"before editing",
		"before deleting",
		"before proceeding",
		"before continuing",
		"continue after",
		"please confirm",
		"please approve",
		"请确认",
		"请批准",
		"等待你确认",
		"等你确认",
	}
	for _, marker := range approvalMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
)

func renderRichText(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if containsANSIEscape(value) {
		plain := stripANSIEscape(value)
		return "```text\n" + plain + "\n```", ansiTextToHTML(value)
	}
	return value, markdownToHTML(value)
}

func markdownToHTML(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(value), &buf); err != nil {
		return "<p>" + html.EscapeString(value) + "</p>"
	}
	return strings.TrimSpace(buf.String())
}

func containsANSIEscape(value string) bool {
	return strings.Contains(value, "\x1b[")
}

func stripANSIEscape(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != 0x1b || i+1 >= len(value) || value[i+1] != '[' {
			out.WriteByte(value[i])
			continue
		}
		i += 2
		for i < len(value) {
			ch := value[i]
			if ch >= '@' && ch <= '~' {
				break
			}
			i++
		}
	}
	return strings.TrimSpace(strings.ReplaceAll(out.String(), "\r\n", "\n"))
}

func ansiTextToHTML(value string) string {
	text := strings.ReplaceAll(value, "\r\n", "\n")
	type styleState struct {
		bold  bool
		faint bool
		color string
	}
	reset := func(state *styleState) {
		*state = styleState{}
	}
	applyCode := func(state *styleState, code int) {
		switch code {
		case 0:
			reset(state)
		case 1:
			state.bold = true
		case 2:
			state.faint = true
		case 22:
			state.bold = false
			state.faint = false
		case 30, 90:
			state.color = "#94a3b8"
		case 31, 91:
			state.color = "#f87171"
		case 32, 92:
			state.color = "#4ade80"
		case 33, 93:
			state.color = "#fbbf24"
		case 34, 94:
			state.color = "#60a5fa"
		case 35, 95:
			state.color = "#c084fc"
		case 36, 96:
			state.color = "#22d3ee"
		case 37, 97:
			state.color = "#e5e7eb"
		case 39:
			state.color = ""
		}
	}
	buildStyle := func(state styleState) string {
		styles := make([]string, 0, 3)
		if state.bold {
			styles = append(styles, "font-weight:600")
		}
		if state.faint {
			styles = append(styles, "opacity:0.7")
		}
		if state.color != "" {
			styles = append(styles, "color:"+state.color)
		}
		return strings.Join(styles, ";")
	}

	var buf strings.Builder
	buf.WriteString(`<pre class="terminal-output"><code>`)
	state := styleState{}
	openSpan := false
	for i := 0; i < len(text); i++ {
		if text[i] == 0x1b && i+1 < len(text) && text[i+1] == '[' {
			j := i + 2
			for j < len(text) {
				ch := text[j]
				if ch >= '@' && ch <= '~' {
					break
				}
				j++
			}
			if j >= len(text) {
				break
			}
			if text[j] == 'm' {
				codes := strings.Split(text[i+2:j], ";")
				if len(codes) == 1 && codes[0] == "" {
					codes = []string{"0"}
				}
				next := state
				for _, raw := range codes {
					if raw == "" {
						raw = "0"
					}
					switch raw {
					case "0":
						applyCode(&next, 0)
					case "1":
						applyCode(&next, 1)
					case "2":
						applyCode(&next, 2)
					case "22":
						applyCode(&next, 22)
					case "30":
						applyCode(&next, 30)
					case "31":
						applyCode(&next, 31)
					case "32":
						applyCode(&next, 32)
					case "33":
						applyCode(&next, 33)
					case "34":
						applyCode(&next, 34)
					case "35":
						applyCode(&next, 35)
					case "36":
						applyCode(&next, 36)
					case "37":
						applyCode(&next, 37)
					case "39":
						applyCode(&next, 39)
					case "90":
						applyCode(&next, 90)
					case "91":
						applyCode(&next, 91)
					case "92":
						applyCode(&next, 92)
					case "93":
						applyCode(&next, 93)
					case "94":
						applyCode(&next, 94)
					case "95":
						applyCode(&next, 95)
					case "96":
						applyCode(&next, 96)
					case "97":
						applyCode(&next, 97)
					}
				}
				if openSpan {
					buf.WriteString("</span>")
					openSpan = false
				}
				state = next
				if style := buildStyle(state); style != "" {
					buf.WriteString(`<span style="` + style + `">`)
					openSpan = true
				}
			}
			i = j
			continue
		}
		switch text[i] {
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '&':
			buf.WriteString("&amp;")
		default:
			buf.WriteByte(text[i])
		}
	}
	if openSpan {
		buf.WriteString("</span>")
	}
	buf.WriteString(`</code></pre>`)
	return buf.String()
}

func compactTitle(value string) string {
	value = normalizeInlineWhitespace(value)
	if value == "" {
		return ""
	}
	return clipHead(value, 42)
}

func microTitle(value string) string {
	value = normalizeInlineWhitespace(value)
	if value == "" {
		return ""
	}
	return clipHead(value, 24)
}

func titleWithSuffix(base, suffix string, limit int) string {
	base = normalizeInlineWhitespace(base)
	suffix = normalizeInlineWhitespace(suffix)
	if base == "" {
		return clipHead(suffix, limit)
	}
	if suffix == "" {
		return clipHead(base, limit)
	}
	if limit <= 0 {
		return ""
	}
	combined := base + " · " + suffix
	runes := []rune(combined)
	if len(runes) <= limit {
		return combined
	}

	suffixPart := " · " + suffix
	suffixRunes := []rune(suffixPart)
	if len(suffixRunes) >= limit {
		return clipTail(suffix, limit)
	}

	baseLimit := limit - len(suffixRunes)
	return clipHead(base, baseLimit) + suffixPart
}

func compactSummary(value string, preferTail bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if preferTail {
		if section := lastNonEmptySection(value); section != "" {
			return clipHead(normalizeInlineWhitespace(section), 72)
		}
		if line := lastNonEmptyLine(value); line != "" {
			return clipHead(normalizeInlineWhitespace(line), 72)
		}
		return clipTail(normalizeInlineWhitespace(value), 72)
	}
	return clipHead(normalizeInlineWhitespace(value), 72)
}

func microSummary(value string, preferTail bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if preferTail {
		if section := lastNonEmptySection(value); section != "" {
			return clipHead(normalizeInlineWhitespace(section), 44)
		}
		if line := lastNonEmptyLine(value); line != "" {
			return clipHead(normalizeInlineWhitespace(line), 44)
		}
		return clipTail(normalizeInlineWhitespace(value), 44)
	}
	return clipHead(normalizeInlineWhitespace(value), 44)
}

func normalizeInlineWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func clipHead(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func clipTail(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[len(runes)-limit:])
	}
	return "…" + string(runes[len(runes)-(limit-1):])
}

func lastNonEmptySection(value string) string {
	for _, item := range splitKeepOrder(value, "\n\n") {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func lastNonEmptyLine(value string) string {
	for _, item := range splitKeepOrder(value, "\n") {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func splitKeepOrder(value, separator string) []string {
	raw := strings.Split(value, separator)
	items := make([]string, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		items = append(items, raw[i])
	}
	return items
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSSE(w http.ResponseWriter, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: " + event + "\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
		return err
	}
	return nil
}

func isLoopbackRequest(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var debugPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>codex-buddy status</title>
  <style>
    body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 24px; background: #0b1020; color: #edf2f7; }
    .wrap { max-width: 760px; margin: 0 auto; }
    .card { background: #11182d; border: 1px solid #22304f; border-radius: 14px; padding: 16px; }
    h1, h2 { margin-top: 0; }
    .status { display: inline-block; padding: 6px 12px; border-radius: 999px; background: #1e293b; border: 1px solid #334155; text-transform: lowercase; }
    .status-offline { background: #111827; border-color: #374151; }
    .status-idle { background: #0f172a; border-color: #334155; }
    .status-running { background: #172554; border-color: #2563eb; }
    .status-open { background: #3f2b02; border-color: #f59e0b; }
    .status-error { background: #3b0a0a; border-color: #ef4444; }
    .muted { color: #94a3b8; font-size: 12px; }
    .summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
    .sessions { display: grid; gap: 10px; margin-top: 16px; }
    .session { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; background: #0a1124; border: 1px solid #22304f; border-radius: 10px; padding: 12px; }
    .session-main { min-width: 0; }
    .session-name { font-weight: 600; word-break: break-word; }
    .session-meta { margin-top: 4px; }
    .session-summary { margin-top: 8px; color: #dbe6ff; word-break: break-word; overflow-wrap: anywhere; }
    .session-summary > :first-child { margin-top: 0; }
    .session-summary > :last-child { margin-bottom: 0; }
    .session-summary p { margin: 0 0 10px; }
    .session-summary ul, .session-summary ol { margin: 0 0 10px 20px; padding: 0; }
    .session-summary li { margin: 4px 0; }
    .session-summary a { color: inherit; text-decoration: none; pointer-events: none; cursor: text; }
    .session-summary code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; padding: 1px 5px; border-radius: 6px; background: #162033; color: #e2e8f0; }
    .session-summary pre { margin: 8px 0 10px; padding: 12px 14px; overflow: auto; color: #dbe6ff; white-space: pre-wrap; }
    .session-summary pre code { padding: 0; border-radius: 0; background: transparent; color: inherit; }
    .session-summary .terminal-output { background: transparent; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card" style="margin-top:16px;">
      <h1>codex-buddy Status</h1>
      <div class="summary">
        <div>
          <div class="muted">aggregate status</div>
          <div style="margin-top:8px;"><span class="status status-offline" id="overall">offline</span></div>
        </div>
        <div style="text-align:right;">
          <div class="muted" id="serverTime">server_time: -</div>
        </div>
      </div>
      <h2 style="margin-top:20px;">Sessions</h2>
      <div class="sessions" id="sessions"></div>
    </div>
  </div>
  <script>
    let source;
    const states = ['offline', 'idle', 'running', 'open', 'error'];
    function escapeHTML(value) {
      return String(value || '')
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
    }
    function stripLinks(html) {
      const template = document.createElement('template');
      template.innerHTML = String(html || '');
      template.content.querySelectorAll('a').forEach((link) => {
        const text = document.createTextNode(link.textContent || '');
        link.replaceWith(text);
      });
      return template.innerHTML;
    }
    function normalizeState(value) {
      const state = String(value || '').toLowerCase();
      if (state === 'running_bash') return 'running';
      if (state === 'attention') return 'open';
      return states.includes(state) ? state : 'offline';
    }
    function renderStateChip(el, state) {
      const normalized = normalizeState(state);
      el.className = 'status status-' + normalized;
      el.textContent = normalized === 'running' ? 'RUN' : normalized;
    }
    async function loadOnce() {
      const resp = await fetch('/v1/status');
      if (!resp.ok) throw new Error('status request failed: ' + resp.status);
      return resp.json();
    }
    function render(snapshot) {
      renderStateChip(document.getElementById('overall'), snapshot.overall_state);
      document.getElementById('serverTime').textContent = 'server_time: ' + (snapshot.server_time || '-');
      const sessions = document.getElementById('sessions');
      sessions.innerHTML = '';
      if (!Array.isArray(snapshot.sessions) || snapshot.sessions.length === 0) {
        sessions.innerHTML = '<div class="session"><div class="session-main"><div class="session-name muted">No active sessions</div></div><span class="status status-offline">offline</span></div>';
        return;
      }
      snapshot.sessions.forEach((session) => {
        const title = session.display_title || session.short_session_id || session.session_id || 'unknown';
        const detail = session.open_summary || session.summary || '';
        const detailHTML = stripLinks(session.open_summary_html || session.summary_html || '');
        const meta = [];
        if (session.short_session_id) meta.push(session.short_session_id);
        if (session.updated_at) meta.push(session.updated_at);
        if (session.needs_approval) meta.push('approval pending');
        if (session.can_continue) meta.push('continue available');
        const el = document.createElement('div');
        el.className = 'session';
        el.innerHTML = [
          '<div class="session-main">',
          '  <div class="session-name">' + escapeHTML(title) + '</div>',
          '  <div class="muted session-meta">' + escapeHTML(meta.join(' · ')) + '</div>',
          detailHTML ? ('  <div class="session-summary">' + detailHTML + '</div>') : (detail ? ('  <div class="session-summary">' + escapeHTML(detail) + '</div>') : ''),
          '</div>',
          '<span class="status status-' + normalizeState(session.state) + '">' + normalizeState(session.state) + '</span>'
        ].join('');
        sessions.appendChild(el);
      });
    }
    async function connect() {
      if (source) source.close();
      render(await loadOnce());
      source = new EventSource('/v1/stream');
      source.addEventListener('status', (event) => render(JSON.parse(event.data)));
      source.onerror = () => {
        renderStateChip(document.getElementById('overall'), 'error');
        document.getElementById('serverTime').textContent = 'stream_error';
      };
    }
    connect().catch((err) => {
      renderStateChip(document.getElementById('overall'), 'error');
      document.getElementById('serverTime').textContent = String(err);
    });
  </script>
</body>
</html>`

var codexDisabledPageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>codex-buddy passive mode</title>
  <style>
    body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 24px; background: #0b1020; color: #edf2f7; }
    .wrap { max-width: 720px; margin: 0 auto; }
    .card { background: #11182d; border: 1px solid #22304f; border-radius: 14px; padding: 16px; }
    .muted { color: #94a3b8; font-size: 12px; }
    a { color: #8fb4ff; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>Passive Monitor Only</h1>
      <p>To avoid interfering with your normal <code>codex cli</code> workflow, ` + "`/debug/codex`" + ` and ` + "`/v1/codex/*`" + ` are disabled.</p>
      <p>The current build keeps passive monitoring only: it observes real local sessions through hooks and transcript watching, without creating threads or sending turns.</p>
      <p><a href="/status">Open the passive status page</a></p>
      <p class="muted">If approval or rejection flows are needed later, evaluate them separately with an app-server based design.</p>
    </div>
  </div>
</body>
</html>`
