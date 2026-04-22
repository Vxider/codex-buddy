package control

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/vxider/codex-buddy/internal/model"
)

type ContinueExecutor interface {
	Continue(session model.SessionSnapshot, text string) error
}

type TmuxContinueExecutor struct {
	logger *log.Logger
}

func NewTmuxContinueExecutor(logger *log.Logger) *TmuxContinueExecutor {
	return &TmuxContinueExecutor{logger: logger}
}

func (e *TmuxContinueExecutor) Continue(session model.SessionSnapshot, text string) error {
	paneID := strings.TrimSpace(session.TmuxPane)
	if paneID == "" {
		return fmt.Errorf("session %s is not bound to a tmux pane", session.SessionID)
	}

	dead, err := tmuxPaneDead(paneID)
	if err != nil {
		return err
	}
	if dead {
		return fmt.Errorf("tmux pane %s is dead", paneID)
	}

	if e.logger != nil {
		e.logger.Printf("tmux continue session=%s pane=%s text=%q", session.SessionID, paneID, text)
	}

	cmd := exec.Command("tmux", "send-keys", "-t", paneID, text, "Enter")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func tmuxPaneDead(paneID string) (bool, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{pane_dead}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("tmux pane lookup failed: %w: %s", err, strings.TrimSpace(stdout.String()))
	}
	return strings.TrimSpace(stdout.String()) == "1", nil
}
