package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vxider/codex-buddy/engine/bootstrap"
	"github.com/vxider/codex-buddy/engine/control"
	"github.com/vxider/codex-buddy/engine/store"
	"github.com/vxider/codex-buddy/engine/transcript"
	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
)

type Runtime struct {
	cfg          config.Config
	logger       *log.Logger
	store        *store.Store
	transcript   *transcript.Manager
	control      control.ContinueExecutor
	pollInterval time.Duration

	mu             sync.Mutex
	recoveredPaths map[string]string
	startOnce      sync.Once
}

func NewRuntime(cfg config.Config, logger *log.Logger) *Runtime {
	if logger == nil {
		logger = log.New(log.Writer(), "codex-buddy: ", log.LstdFlags|log.Lmsgprefix)
	}

	sessionStore := store.New(
		time.Duration(cfg.State.AttentionHoldMS)*time.Millisecond,
		time.Duration(cfg.State.IdleFallbackMS)*time.Millisecond,
		logger,
	)

	r := &Runtime{
		cfg:            cfg,
		logger:         logger,
		store:          sessionStore,
		control:        control.NewTmuxContinueExecutor(logger),
		pollInterval:   time.Duration(cfg.Transcript.PollIntervalMS) * time.Millisecond,
		recoveredPaths: make(map[string]string),
	}
	if r.pollInterval <= 0 {
		r.pollInterval = time.Second
	}

	r.transcript = transcript.NewManager(
		cfg.Transcript.Enabled,
		cfg.Transcript.TailFromEnd,
		r.pollInterval,
		logger,
		func(update model.TranscriptUpdate) {
			r.store.ApplyTranscriptUpdate(update)
		},
	)
	return r
}

func (r *Runtime) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.startOnce.Do(func() {
		if r.logger != nil {
			r.logger.Printf("runtime using Codex hook state with tmux session discovery")
		}
		r.store.StartReaper(30 * time.Second)
		r.discoverOpenSessions()
		go r.discoveryLoop(ctx)
	})
}

func (r *Runtime) discoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.discoverOpenSessions()
		}
	}
}

func (r *Runtime) discoverOpenSessions() {
	sessions, err := bootstrap.RecoverOpenSessions(r.logger)
	if err != nil {
		if r.logger != nil {
			r.logger.Printf("tmux session discovery failed: %v", err)
		}
		return
	}
	if r.logger != nil {
		r.logger.Printf("tmux session discovery found %d session(s)", len(sessions))
	}

	discovered := make([]store.DiscoverySession, 0, len(sessions))
	for _, session := range sessions {
		discovered = append(discovered, store.DiscoverySession{
			SessionID:      session.SessionID,
			TranscriptPath: session.TranscriptPath,
			CWD:            session.CWD,
			TmuxPane:       session.TmuxPane,
			TmuxSession:    session.TmuxSession,
			TmuxWindow:     session.TmuxWindow,
		})
	}

	removed, _ := r.store.ApplyDiscovery(discovered, time.Now().UTC())
	for _, sessionID := range removed {
		r.transcript.Remove(sessionID)
	}
	for _, session := range sessions {
		r.ensureRecovered(session.SessionID, session.TranscriptPath)
	}
}

func (r *Runtime) Store() *store.Store {
	return r.store
}

func (r *Runtime) TranscriptManager() *transcript.Manager {
	return r.transcript
}

func (r *Runtime) ContinueExecutor() control.ContinueExecutor {
	return r.control
}

func (r *Runtime) SessionOpenChecker() control.SessionOpenChecker {
	return control.NewTmuxSessionOpenChecker(r.logger)
}

func (r *Runtime) Snapshot() model.StatusSnapshot {
	return r.store.Snapshot()
}

func (r *Runtime) Notifications() []model.NotificationSnapshot {
	return r.store.Notifications()
}

func (r *Runtime) Subscribe() (<-chan model.StatusSnapshot, func()) {
	return r.store.Subscribe()
}

func (r *Runtime) AckNotification(id string) (model.NotificationSnapshot, bool) {
	return r.store.AckNotification(id)
}

func (r *Runtime) ContinueNotification(id, token string) (model.NotificationSnapshot, error) {
	if r.control == nil {
		return model.NotificationSnapshot{}, fmt.Errorf("continue action is not configured")
	}

	notification, session, ok := r.store.ContinueTarget(id, token)
	if !ok {
		return model.NotificationSnapshot{}, fmt.Errorf("notification is no longer actionable")
	}
	if err := r.control.Continue(session, model.ContinueCommandText); err != nil {
		return model.NotificationSnapshot{}, err
	}
	updated, ok := r.store.MarkNotificationActed(id)
	if !ok {
		return model.NotificationSnapshot{}, fmt.Errorf("notification disappeared after action")
	}
	if r.logger != nil {
		r.logger.Printf("continue action sent session=%s notification=%s", notification.SessionID, notification.ID)
	}
	return updated, nil
}

func (r *Runtime) ContinueSession(sessionID string) (model.NotificationSnapshot, model.SessionSnapshot, error) {
	if r.control == nil {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, fmt.Errorf("continue action is not configured")
	}

	notification, session, ok := r.store.ContinueTargetForSession(sessionID)
	if !ok {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, fmt.Errorf("session continue is unavailable")
	}
	if err := r.control.Continue(session, model.ContinueCommandText); err != nil {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, err
	}
	updated, ok := r.store.MarkNotificationActed(notification.ID)
	if !ok {
		return model.NotificationSnapshot{}, model.SessionSnapshot{}, fmt.Errorf("notification disappeared after action")
	}
	return updated, session, nil
}

func (r *Runtime) ApplyIngest(req model.IngestRequest) model.StatusSnapshot {
	snapshot := r.store.ApplyIngest(req)
	if req.Payload.TranscriptPath != "" && req.Payload.SessionID != "" {
		r.ensureRecovered(req.Payload.SessionID, req.Payload.TranscriptPath)
	}
	return snapshot
}

func (r *Runtime) ensureRecovered(sessionID, transcriptPath string) {
	if sessionID == "" || transcriptPath == "" {
		return
	}

	r.mu.Lock()
	previous := r.recoveredPaths[sessionID]
	if previous == transcriptPath {
		r.mu.Unlock()
		r.transcript.Ensure(sessionID, transcriptPath)
		return
	}
	r.recoveredPaths[sessionID] = transcriptPath
	r.mu.Unlock()

	update, err := transcript.RecoverSession(transcriptPath, sessionID)
	if err != nil {
		if r.logger != nil {
			r.logger.Printf("transcript recovery failed session=%s path=%s err=%v", sessionID, transcriptPath, err)
		}
	} else if update.SessionID != "" && !update.UpdatedAt.IsZero() {
		r.store.ApplyTranscriptUpdate(update)
	}
	r.transcript.Ensure(sessionID, transcriptPath)
}
