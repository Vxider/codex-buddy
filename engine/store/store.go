package store

import (
	"encoding/json"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vxider/codex-buddy/engine/present"
	"github.com/vxider/codex-buddy/internal/model"
)

type DiscoverySession struct {
	SessionID      string
	TranscriptPath string
	CWD            string
	TmuxPane       string
	TmuxSession    string
	TmuxWindow     string
}

type Store struct {
	mu            sync.RWMutex
	sessions      map[string]*sessionState
	notifications map[string]*model.NotificationSnapshot
	recentEvents  []model.RecentEvent
	attentionHold time.Duration
	idleFallback  time.Duration
	subscribers   map[chan model.StatusSnapshot]struct{}
	logger        *log.Logger
}

type sessionState struct {
	model.SessionSnapshot
	notificationEpoch    int
	activeNotificationID string
}

func New(attentionHold, idleFallback time.Duration, logger *log.Logger) *Store {
	return &Store{
		sessions:      make(map[string]*sessionState),
		notifications: make(map[string]*model.NotificationSnapshot),
		recentEvents:  make([]model.RecentEvent, 0, 64),
		attentionHold: attentionHold,
		idleFallback:  idleFallback,
		subscribers:   make(map[chan model.StatusSnapshot]struct{}),
		logger:        logger,
	}
}

func (s *Store) ApplyIngest(req model.IngestRequest) model.StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := req.Payload.SessionID
	if sessionID == "" {
		sessionID = "unknown"
	}

	session, ok := s.sessions[sessionID]
	if !ok {
		session = &sessionState{
			SessionSnapshot: model.SessionSnapshot{
				SessionID: sessionID,
				State:     model.StateIdle,
				UpdatedAt: req.ReceivedAt,
			},
		}
		s.sessions[sessionID] = session
	}

	previous := s.deriveSession(session.SessionSnapshot)

	session.TurnID = coalesce(req.Payload.TurnID, session.TurnID)
	session.CWD = coalesce(req.Payload.CWD, session.CWD)
	session.Model = coalesce(req.Payload.Model, session.Model)
	session.TmuxPane = coalesce(req.Payload.TmuxPane, session.TmuxPane)
	session.TmuxSession = coalesce(req.Payload.TmuxSession, session.TmuxSession)
	session.TmuxWindow = coalesce(req.Payload.TmuxWindow, session.TmuxWindow)
	session.TranscriptPath = coalesce(req.Payload.TranscriptPath, session.TranscriptPath)
	session.UpdatedAt = maxTime(req.ReceivedAt, session.UpdatedAt)

	if req.Payload.Prompt != "" {
		session.LastUserPromptPreview = preview(req.Payload.Prompt)
	}
	if req.Payload.LastAssistantMessage != "" {
		session.LastAssistantMessageFull = strings.TrimSpace(req.Payload.LastAssistantMessage)
		session.LastAssistantMessage = previewAssistant(req.Payload.LastAssistantMessage)
	}
	if req.Payload.Error != "" {
		session.LastError = preview(req.Payload.Error)
	}

	event := canonicalEvent(req.HookEventName, req.EventName)
	switch event {
	case "sessionstart":
		session.State = model.StateIdle
		session.StateDetail = string(model.StateIdle)
		session.LastError = ""
		session.CurrentAttentionDeadline = time.Time{}
	case "userpromptsubmit":
		session.State = model.StateRunning
		session.StateDetail = string(model.StateRunning)
		session.LastError = ""
		session.CurrentAttentionDeadline = time.Time{}
	case "pretooluse":
		if strings.EqualFold(req.Payload.ToolName, "Bash") || strings.EqualFold(req.EventName, "pre-tool-use") {
			session.State = model.StateRunningBash
			session.StateDetail = string(model.StateRunningBash)
			if command := previewAny(req.Payload.ToolInput); command != "" {
				session.LastBashCommand = command
			}
		}
	case "posttooluse":
		if strings.EqualFold(req.Payload.ToolName, "Bash") || session.State == model.StateRunningBash {
			session.State = model.StateRunning
			session.StateDetail = string(model.StateRunning)
		}
	case "stop":
		session.CurrentAttentionDeadline = s.attentionDeadline()
		if req.Payload.Error != "" {
			session.State = model.StateError
			session.StateDetail = string(model.StateError)
		} else {
			session.State = stopState(session)
			session.StateDetail = string(session.State)
		}
	default:
		s.logger.Printf("ignoring unknown event=%q session=%q", event, sessionID)
	}

	s.appendRecentEventLocked(model.RecentEvent{
		Time:      req.ReceivedAt,
		Source:    "hook",
		SessionID: sessionID,
		EventName: event,
		Summary:   summarizeHookEvent(req, session),
	})

	s.reconcileNotificationLocked(session, previous, s.deriveSession(session.SessionSnapshot), req.ReceivedAt)

	snapshot := s.snapshotLocked()
	s.broadcastLocked(snapshot)
	return snapshot
}

func (s *Store) ApplyTranscriptUpdate(update model.TranscriptUpdate) model.StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := update.SessionID
	if sessionID == "" {
		sessionID = "unknown"
	}

	session, ok := s.sessions[sessionID]
	if !ok {
		session = &sessionState{
			SessionSnapshot: model.SessionSnapshot{
				SessionID: sessionID,
				State:     model.StateIdle,
				UpdatedAt: update.UpdatedAt,
			},
		}
		s.sessions[sessionID] = session
	}

	previous := s.deriveSession(session.SessionSnapshot)

	session.TurnID = coalesce(update.TurnID, session.TurnID)
	session.CWD = coalesce(update.CWD, session.CWD)
	session.Model = coalesce(update.Model, session.Model)
	session.UpdatedAt = maxTime(update.UpdatedAt, session.UpdatedAt)

	if update.LastUserPromptPreview != "" {
		session.LastUserPromptPreview = preview(update.LastUserPromptPreview)
	}
	if update.LastAssistantMessage != "" {
		session.LastAssistantMessageFull = strings.TrimSpace(update.LastAssistantMessage)
		session.LastAssistantMessage = previewAssistant(update.LastAssistantMessage)
	}
	if update.LastBashCommand != "" {
		session.LastBashCommand = preview(update.LastBashCommand)
	}
	if update.Error != "" {
		session.LastError = preview(update.Error)
	}
	if update.GoalState != "" {
		session.GoalState = update.GoalState
	}
	if update.GoalSummary != "" {
		session.GoalSummary = preview(update.GoalSummary)
	}
	if !update.GoalUpdatedAt.IsZero() {
		session.GoalUpdatedAt = update.GoalUpdatedAt
	}
	if session.State == model.StateIdle && needsAttentionFromMessage(session.LastAssistantMessage) {
		session.State = model.StateAttention
		session.StateDetail = string(model.StateAttention)
		session.CurrentAttentionDeadline = s.attentionDeadline()
	}

	s.appendRecentEventLocked(model.RecentEvent{
		Time:      update.UpdatedAt,
		Source:    "transcript",
		SessionID: sessionID,
		EventName: "transcript_update",
		Summary:   summarizeTranscriptUpdate(update),
	})

	s.reconcileNotificationLocked(session, previous, s.deriveSession(session.SessionSnapshot), update.UpdatedAt)

	snapshot := s.snapshotLocked()
	s.broadcastLocked(snapshot)
	return snapshot
}

func (s *Store) Snapshot() model.StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *Store) ApplyDiscovery(sessions []DiscoverySession, now time.Time) ([]string, model.StatusSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if now.IsZero() {
		now = time.Now().UTC()
	}

	seen := make(map[string]DiscoverySession, len(sessions))
	for _, discovered := range sessions {
		sessionID := strings.TrimSpace(discovered.SessionID)
		if sessionID == "" {
			continue
		}
		seen[sessionID] = discovered

		current, ok := s.sessions[sessionID]
		if !ok {
			current = &sessionState{
				SessionSnapshot: model.SessionSnapshot{
					SessionID: sessionID,
					State:     model.StateIdle,
					UpdatedAt: now,
				},
			}
			s.sessions[sessionID] = current
		}

		previous := s.deriveSession(current.SessionSnapshot)
		current.CWD = coalesce(discovered.CWD, current.CWD)
		current.TranscriptPath = coalesce(discovered.TranscriptPath, current.TranscriptPath)
		current.TmuxPane = coalesce(discovered.TmuxPane, current.TmuxPane)
		current.TmuxSession = coalesce(discovered.TmuxSession, current.TmuxSession)
		current.TmuxWindow = coalesce(discovered.TmuxWindow, current.TmuxWindow)
		current.UpdatedAt = maxTime(now, current.UpdatedAt)

		s.reconcileNotificationLocked(current, previous, s.deriveSession(current.SessionSnapshot), now)
	}

	removed := make([]string, 0)
	for sessionID, session := range s.sessions {
		if _, ok := seen[sessionID]; ok {
			continue
		}
		if session.TmuxPane == "" {
			continue
		}
		if session.activeNotificationID != "" {
			if existing, ok := s.notifications[session.activeNotificationID]; ok && existing.State != model.NotificationActed {
				existing.State = model.NotificationExpired
				existing.UpdatedAt = now
			}
		}
		delete(s.sessions, sessionID)
		removed = append(removed, sessionID)
	}

	snapshot := s.snapshotLocked()
	s.broadcastLocked(snapshot)
	return removed, snapshot
}

func (s *Store) Session(sessionID string) (model.SessionSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return model.SessionSnapshot{}, false
	}

	snap := s.deriveSession(session.SessionSnapshot)
	return snap, true
}

func (s *Store) Sessions() []model.SessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedSessionsLocked()
}

func (s *Store) Notifications() []model.NotificationSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]model.NotificationSnapshot, 0, len(s.notifications))
	for _, notification := range s.notifications {
		if !s.isNotificationVisibleLocked(notification) {
			continue
		}
		items = append(items, *notification)
	}

	slices.SortFunc(items, func(a, b model.NotificationSnapshot) int {
		if priorityForNotification(a.Kind) != priorityForNotification(b.Kind) {
			return priorityForNotification(b.Kind) - priorityForNotification(a.Kind)
		}
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})

	return items
}

func (s *Store) AckNotification(id string) (model.NotificationSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, ok := s.notifications[id]
	if !ok || !s.isNotificationVisibleLocked(notification) {
		return model.NotificationSnapshot{}, false
	}
	notification.State = model.NotificationAcked
	notification.UpdatedAt = time.Now().UTC()
	return *notification, true
}

func (s *Store) ContinueTarget(id, token string) (model.NotificationSnapshot, model.SessionSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notification, ok := s.notifications[id]
	if !ok || !s.isNotificationVisibleLocked(notification) {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}
	if notification.ActionToken == "" || notification.ActionToken != token {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}
	if !slices.Contains(notification.Actions, model.NotificationActionContinue) {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}

	session, ok := s.sessions[notification.SessionID]
	if !ok {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}

	derived := s.deriveSession(session.SessionSnapshot)
	if derived.State != model.StateAttention || derived.TmuxPane == "" {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}

	return *notification, derived, true
}

func (s *Store) ContinueTargetForSession(sessionID string) (model.NotificationSnapshot, model.SessionSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *model.NotificationSnapshot
	for _, notification := range s.notifications {
		if notification.SessionID != sessionID {
			continue
		}
		if !s.isNotificationVisibleLocked(notification) {
			continue
		}
		if !slices.Contains(notification.Actions, model.NotificationActionContinue) {
			continue
		}
		if latest == nil || notification.UpdatedAt.After(latest.UpdatedAt) {
			latest = notification
		}
	}
	if latest == nil {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}

	session, ok := s.sessions[latest.SessionID]
	if !ok {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}

	derived := s.deriveSession(session.SessionSnapshot)
	if derived.State != model.StateAttention || derived.TmuxPane == "" {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, false
	}

	return *latest, derived, true
}

func (s *Store) MarkNotificationActed(id string) (model.NotificationSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, ok := s.notifications[id]
	if !ok {
		return model.NotificationSnapshot{}, false
	}
	notification.State = model.NotificationActed
	notification.UpdatedAt = time.Now().UTC()
	return *notification, true
}

func (s *Store) Subscribe() (<-chan model.StatusSnapshot, func()) {
	ch := make(chan model.StatusSnapshot, 8)

	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	ch <- snapshot

	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
	}

	return ch, cancel
}

func (s *Store) snapshotLocked() model.StatusSnapshot {
	sessions := s.sortedSessionsLocked()
	snapshot := model.StatusSnapshot{
		ServerTime:         time.Now().UTC(),
		OverallState:       model.StateIdle,
		OverallStateDetail: string(model.StateIdle),
		SessionsCount:      len(sessions),
		Sessions:           sessions,
		RecentEvents:       append([]model.RecentEvent(nil), s.recentEvents...),
	}

	if len(sessions) == 0 {
		return snapshot
	}

	active := sessions[0]
	snapshot.ActiveSessionID = active.SessionID
	snapshot.OverallState = active.State
	snapshot.OverallStateDetail = active.StateDetail
	snapshot.GoalState = active.GoalState
	snapshot.GoalSummary = active.GoalSummary
	snapshot.GoalUpdatedAt = active.GoalUpdatedAt
	return snapshot
}

func (s *Store) sortedSessionsLocked() []model.SessionSnapshot {
	sessions := make([]model.SessionSnapshot, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, s.deriveSession(session.SessionSnapshot))
	}

	slices.SortFunc(sessions, func(a, b model.SessionSnapshot) int {
		if priority(a.State) != priority(b.State) {
			return priority(b.State) - priority(a.State)
		}
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return 1
		}
		return strings.Compare(a.SessionID, b.SessionID)
	})

	return sessions
}

func (s *Store) deriveSession(session model.SessionSnapshot) model.SessionSnapshot {
	if session.State == model.StateAttention && !session.CurrentAttentionDeadline.IsZero() && time.Now().After(session.CurrentAttentionDeadline) {
		session.State = model.StateIdle
		session.StateDetail = string(model.StateIdle)
		session.CurrentAttentionDeadline = time.Time{}
	}
	if s.idleFallback > 0 && (session.State == model.StateRunning || session.State == model.StateRunningBash) && time.Since(session.UpdatedAt) > s.idleFallback {
		session.State = model.StateIdle
		session.StateDetail = string(model.StateIdle)
	}
	if session.State == model.StateRunningBash {
		session.State = model.StateRunning
		if session.StateDetail == "" {
			session.StateDetail = string(model.StateRunningBash)
		}
	}
	return session
}

func (s *Store) attentionDeadline() time.Time {
	if s.attentionHold <= 0 {
		return time.Time{}
	}
	return time.Now().Add(s.attentionHold)
}

func stopState(session *sessionState) model.State {
	if session == nil {
		return model.StateIdle
	}
	if needsAttentionFromMessage(session.LastAssistantMessage) {
		return model.StateAttention
	}
	return model.StateIdle
}

func needsAttentionFromMessage(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}

	attentionMarkers := []string{
		"approval required",
		"need approval",
		"needs approval",
		"need confirmation",
		"needs confirmation",
		"confirmation",
		"before overwriting",
		"before editing",
		"before deleting",
		"before proceeding",
		"before continuing",
		"would you like me to",
		"if you'd like me to",
		"if you'd like, i can",
		"if you want, i can",
		"let me know if you'd like",
		"let me know if you want",
		"please confirm",
		"please approve",
		"permissionrequest",
		"permission request",
		"如果你愿意",
		"如果你希望",
		"如果你想",
		"如果你要",
		"如果需要",
		"如果你继续",
		"要的话我可以",
		"要不要我",
		"是否要我",
		"请确认",
		"请批准",
		"等待你确认",
		"等你确认",
		"我下一步可以继续",
		"我下一步就直接开始",
		"我下一步就开始",
		"我下一轮会",
		"下一轮会",
		"我可以继续帮你",
		"继续帮你做",
		"直接开始做",
	}
	for _, marker := range attentionMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	if strings.HasSuffix(text, "?") || strings.HasSuffix(text, "？") {
		return true
	}

	return false
}

func (s *Store) reconcileNotificationLocked(session *sessionState, previous, current model.SessionSnapshot, now time.Time) {
	prevKind, _ := notificationKindForState(previous.State)
	currentKind, currentEligible := notificationKindForState(current.State)

	if session.activeNotificationID != "" && (!currentEligible || currentKind != prevKind) {
		if existing, ok := s.notifications[session.activeNotificationID]; ok && existing.State != model.NotificationActed {
			existing.State = model.NotificationExpired
			existing.UpdatedAt = now.UTC()
		}
		session.activeNotificationID = ""
	}

	if !currentEligible {
		return
	}

	summary := notificationSummary(current)
	if summary == "" {
		return
	}

	if session.activeNotificationID == "" {
		session.notificationEpoch++
		id := notificationID(current.SessionID, session.notificationEpoch)
		notification := &model.NotificationSnapshot{
			ID:          id,
			SessionID:   current.SessionID,
			Kind:        currentKind,
			State:       model.NotificationPending,
			Title:       notificationTitle(current),
			Summary:     summary,
			CreatedAt:   now.UTC(),
			UpdatedAt:   now.UTC(),
			ActionToken: notificationActionToken(current.SessionID, session.notificationEpoch),
			Actions:     notificationActions(current),
		}
		s.notifications[id] = notification
		session.activeNotificationID = id
		s.pruneNotificationsLocked()
		return
	}

	notification, ok := s.notifications[session.activeNotificationID]
	if !ok {
		session.activeNotificationID = ""
		s.reconcileNotificationLocked(session, previous, current, now)
		return
	}

	notification.Kind = currentKind
	notification.Title = notificationTitle(current)
	notification.Summary = summary
	notification.UpdatedAt = now.UTC()
	notification.Actions = notificationActions(current)
}

func (s *Store) pruneNotificationsLocked() {
	if len(s.notifications) <= 100 {
		return
	}

	ids := make([]string, 0, len(s.notifications))
	for id := range s.notifications {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		left := s.notifications[a]
		right := s.notifications[b]
		if left.UpdatedAt.Before(right.UpdatedAt) {
			return -1
		}
		if left.UpdatedAt.After(right.UpdatedAt) {
			return 1
		}
		return strings.Compare(a, b)
	})

	for len(ids) > 100 {
		id := ids[0]
		delete(s.notifications, id)
		ids = ids[1:]
	}
}

func (s *Store) isNotificationVisibleLocked(notification *model.NotificationSnapshot) bool {
	if notification == nil {
		return false
	}
	if notification.State != model.NotificationPending && notification.State != model.NotificationAcked {
		return false
	}
	session, ok := s.sessions[notification.SessionID]
	if !ok {
		return false
	}
	current := s.deriveSession(session.SessionSnapshot)
	kind, ok := notificationKindForState(current.State)
	if !ok {
		return false
	}
	return kind == notification.Kind
}

func (s *Store) broadcastLocked(snapshot model.StatusSnapshot) {
	for ch := range s.subscribers {
		select {
		case ch <- snapshot:
		default:
		}
	}
}

func (s *Store) appendRecentEventLocked(event model.RecentEvent) {
	if strings.TrimSpace(event.Summary) == "" {
		return
	}
	s.recentEvents = append(s.recentEvents, event)
	if len(s.recentEvents) > 50 {
		s.recentEvents = s.recentEvents[len(s.recentEvents)-50:]
	}
}

func priority(state model.State) int {
	switch state {
	case model.StateError:
		return 50
	case model.StateAttention:
		return 40
	case model.StateRunningBash:
		return 30
	case model.StateRunning:
		return 20
	case model.StateIdle:
		return 10
	default:
		return 0
	}
}

func priorityForNotification(kind model.NotificationKind) int {
	switch kind {
	case model.NotificationError:
		return 20
	case model.NotificationAttention:
		return 10
	default:
		return 0
	}
}

func notificationKindForState(state model.State) (model.NotificationKind, bool) {
	switch state {
	case model.StateAttention:
		return model.NotificationAttention, true
	case model.StateError:
		return model.NotificationError, true
	default:
		return "", false
	}
}

func notificationID(sessionID string, epoch int) string {
	return sessionID + ":" + time.Now().UTC().Format("20060102T150405.000000000") + ":" + strconv.Itoa(epoch)
}

func notificationActionToken(sessionID string, epoch int) string {
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 36) + ":" + sessionID + ":" + strconv.Itoa(epoch)
}

func notificationTitle(session model.SessionSnapshot) string {
	switch session.State {
	case model.StateError:
		return present.ErrorTitle(session)
	default:
		return "Codex finished"
	}
}

func notificationSummary(session model.SessionSnapshot) string {
	switch session.State {
	case model.StateError:
		return present.ErrorSummary(session)
	case model.StateAttention:
		return firstNonEmptyRaw(session.LastAssistantMessageFull, session.LastAssistantMessage, session.LastUserPromptPreview)
	default:
		return ""
	}
}

func notificationActions(session model.SessionSnapshot) []model.NotificationAction {
	actions := []model.NotificationAction{model.NotificationActionAck}
	if session.State == model.StateAttention && session.TmuxPane != "" {
		actions = append(actions, model.NotificationActionContinue)
	}
	return actions
}

func canonicalEvent(values ...string) string {
	for _, value := range values {
		if value == "" {
			continue
		}
		replacer := strings.NewReplacer("-", "", "_", "", " ", "")
		return strings.ToLower(replacer.Replace(value))
	}
	return ""
}

func preview(value string) string {
	value = strings.TrimSpace(value)
	const limit = 160
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func previewTail(value string) string {
	value = strings.TrimSpace(value)
	const limit = 160
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return "..." + string(runes[len(runes)-limit:])
}

func previewAssistant(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	paragraphs := splitNonEmptySections(value, "\n\n")
	if len(paragraphs) > 0 {
		last := paragraphs[len(paragraphs)-1]
		if len([]rune(last)) <= 160 {
			return last
		}
	}

	lines := splitNonEmptyLines(value)
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		if len([]rune(last)) <= 160 {
			return last
		}
	}

	return previewTail(value)
}

func previewAny(value any) string {
	switch typed := value.(type) {
	case string:
		return preview(typed)
	case nil:
		return ""
	default:
		blob, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return preview(string(blob))
	}
}

func coalesce(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return preview(value)
		}
	}
	return ""
}

func firstNonEmptyRaw(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func splitNonEmptySections(value, separator string) []string {
	raw := strings.Split(value, separator)
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func splitNonEmptyLines(value string) []string {
	raw := strings.Split(value, "\n")
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func summarizeHookEvent(req model.IngestRequest, session *sessionState) string {
	switch canonicalEvent(req.HookEventName, req.EventName) {
	case "sessionstart":
		if session.CWD != "" {
			return "session started in " + preview(session.CWD)
		}
		return "session started"
	case "userpromptsubmit":
		if req.Payload.Prompt != "" {
			return "prompt: " + preview(req.Payload.Prompt)
		}
		if session.LastUserPromptPreview != "" {
			return "prompt: " + preview(session.LastUserPromptPreview)
		}
		return "user prompt submitted"
	case "pretooluse":
		if command := previewAny(req.Payload.ToolInput); command != "" {
			return "tool start: " + command
		}
		return "tool started"
	case "posttooluse":
		if session.LastBashCommand != "" {
			return "tool finished: " + preview(session.LastBashCommand)
		}
		return "tool finished"
	case "stop":
		if req.Payload.Error != "" {
			return "turn stopped with error: " + preview(req.Payload.Error)
		}
		if session.LastAssistantMessage != "" {
			return "turn stopped, assistant: " + session.LastAssistantMessage
		}
		return "turn stopped"
	default:
		return canonicalEvent(req.HookEventName, req.EventName)
	}
}

func summarizeTranscriptUpdate(update model.TranscriptUpdate) string {
	switch {
	case strings.TrimSpace(update.LastAssistantMessage) != "":
		return "assistant: " + previewAssistant(update.LastAssistantMessage)
	case strings.TrimSpace(update.LastUserPromptPreview) != "":
		return "user: " + preview(update.LastUserPromptPreview)
	case strings.TrimSpace(update.LastBashCommand) != "":
		return "bash: " + preview(update.LastBashCommand)
	case strings.TrimSpace(update.Error) != "":
		return "error: " + preview(update.Error)
	case strings.TrimSpace(update.CWD) != "" || strings.TrimSpace(update.Model) != "":
		parts := make([]string, 0, 2)
		if update.CWD != "" {
			parts = append(parts, "cwd="+preview(update.CWD))
		}
		if update.Model != "" {
			parts = append(parts, "model="+preview(update.Model))
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
