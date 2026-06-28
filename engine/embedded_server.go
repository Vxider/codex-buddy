package engine

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/vxider/agent-buddy/engine/httpapi"
	"github.com/vxider/agent-buddy/internal/config"
)

type EmbeddedServer struct {
	runtime *Runtime
	logger  *log.Logger

	mu       sync.Mutex
	cfg      config.Config
	server   *http.Server
	listener net.Listener
}

func NewEmbeddedServer(runtime *Runtime, cfg config.Config, logger *log.Logger) *EmbeddedServer {
	return &EmbeddedServer{
		runtime: runtime,
		cfg:     cfg,
		logger:  logger,
	}
}

func (s *EmbeddedServer) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener != nil
}

func (s *EmbeddedServer) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Listen.Address()
}

func (s *EmbeddedServer) Start(ctx context.Context, cfg config.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		if s.cfg.Listen.Address() == cfg.Listen.Address() {
			s.cfg = cfg
			return nil
		}
		if err := s.stopLocked(context.Background()); err != nil {
			return err
		}
	}

	handler := httpapi.NewServer(
		cfg,
		s.runtime.Store(),
		s.runtime.TranscriptManager(),
		s.runtime.ContinueExecutor(),
		s.runtime.SessionOpenChecker(),
		s.logger,
	)

	listener, err := net.Listen("tcp", cfg.Listen.Address())
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.Listen.Address(),
		Handler:           handler.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	s.cfg = cfg
	s.server = server
	s.listener = listener

	go func(server *http.Server, listener net.Listener) {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && s.logger != nil {
			s.logger.Printf("embedded server failed: %v", err)
		}
	}(server, listener)

	if s.logger != nil {
		s.logger.Printf("embedded server listening on %s", listener.Addr().String())
	}
	return nil
}

func (s *EmbeddedServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked(ctx)
}

func (s *EmbeddedServer) stopLocked(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	server := s.server
	s.server = nil
	s.listener = nil
	return server.Shutdown(ctx)
}
