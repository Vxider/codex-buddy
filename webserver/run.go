package webserver

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/vxider/agent-buddy/engine"
	"github.com/vxider/agent-buddy/engine/httpapi"
	"github.com/vxider/agent-buddy/internal/config"
)

func Run(ctx context.Context, cfg config.Config, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(log.Writer(), "agent-buddy: ", log.LstdFlags|log.Lmsgprefix)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancelCtx := context.WithCancel(ctx)
	defer cancelCtx()

	runtime := engine.NewRuntime(cfg, logger)
	runtime.Start(ctx)
	server := httpapi.NewServer(
		cfg,
		runtime.Store(),
		runtime.TranscriptManager(),
		runtime.ContinueExecutor(),
		runtime.SessionOpenChecker(),
		logger,
	)
	server.SetShutdownFunc(func() {
		if logger != nil {
			logger.Printf("shutdown requested via internal API")
		}
		cancelCtx()
	})

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
