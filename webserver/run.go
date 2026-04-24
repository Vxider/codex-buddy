package webserver

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
	"github.com/vxider/codex-buddy/webserver/internal/api"
	"github.com/vxider/codex-buddy/webserver/internal/bootstrap"
	"github.com/vxider/codex-buddy/webserver/internal/control"
	"github.com/vxider/codex-buddy/webserver/internal/store"
	"github.com/vxider/codex-buddy/webserver/internal/transcript"
)

func Run(ctx context.Context, cfg config.Config, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(log.Writer(), "codex-buddy: ", log.LstdFlags|log.Lmsgprefix)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sessionStore := store.New(
		time.Duration(cfg.State.AttentionHoldMS)*time.Millisecond,
		time.Duration(cfg.State.IdleFallbackMS)*time.Millisecond,
		logger,
	)
	transcriptManager := transcript.NewManager(
		cfg.Transcript.Enabled,
		cfg.Transcript.TailFromEnd,
		time.Duration(cfg.Transcript.PollIntervalMS)*time.Millisecond,
		logger,
		func(update model.TranscriptUpdate) {
			sessionStore.ApplyTranscriptUpdate(update)
		},
	)
	bootstrapOpenSessions(sessionStore, transcriptManager, logger)
	server := api.NewServer(
		cfg,
		sessionStore,
		transcriptManager,
		control.NewTmuxContinueExecutor(logger),
		control.NewTmuxSessionOpenChecker(logger),
		logger,
	)

	httpServer := &http.Server{
		Addr:              cfg.Listen.Address(),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.Listen.Address())
	if err != nil {
		return err
	}

	logger.Printf("listening on %s", listener.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Printf("shutdown requested")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
		if closeErr := httpServer.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			return closeErr
		}
		logger.Printf("server closed after shutdown timeout")
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(1 * time.Second):
		logger.Printf("server exit confirmation timed out; assuming closed")
		return nil
	}
}

func bootstrapOpenSessions(sessionStore *store.Store, transcriptManager *transcript.Manager, logger *log.Logger) {
	sessions, err := bootstrap.RecoverOpenSessions(logger)
	if err != nil {
		if logger != nil {
			logger.Printf("bootstrap session recovery failed: %v", err)
		}
		return
	}

	now := time.Now().UTC()
	for _, session := range sessions {
		sessionStore.ApplyIngest(model.IngestRequest{
			Source:        "bootstrap",
			EventName:     "session-start",
			HookEventName: "session-start",
			ReceivedAt:    now,
			Payload: model.HookPayload{
				SessionID:      session.SessionID,
				CWD:            session.CWD,
				TmuxPane:       session.TmuxPane,
				TmuxSession:    session.TmuxSession,
				TmuxWindow:     session.TmuxWindow,
				TranscriptPath: session.TranscriptPath,
			},
		})

		if session.TranscriptPath == "" {
			if logger != nil {
				logger.Printf("bootstrap recovered session=%s pane=%s without transcript path", session.SessionID, session.TmuxPane)
			}
			continue
		}

		update, err := transcript.RecoverSession(session.TranscriptPath, session.SessionID)
		if err != nil {
			if logger != nil {
				logger.Printf("bootstrap transcript recovery failed session=%s path=%s err=%v", session.SessionID, session.TranscriptPath, err)
			}
		} else if update.SessionID != "" && !update.UpdatedAt.IsZero() {
			sessionStore.ApplyTranscriptUpdate(update)
		}

		transcriptManager.Ensure(session.SessionID, session.TranscriptPath)
	}

	if logger != nil && len(sessions) > 0 {
		logger.Printf("bootstrap recovered %d open codex sessions", len(sessions))
	}
}
