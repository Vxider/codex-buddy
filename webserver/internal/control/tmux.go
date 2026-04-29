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

type SessionCloser interface {
	Close(session model.SessionSnapshot) error
}

type SessionOpenChecker interface {
	IsOpen(session model.SessionSnapshot) bool
}

type TmuxContinueExecutor struct {
	logger *log.Logger
}

type TmuxSessionOpenChecker struct {
	logger *log.Logger
}

func NewTmuxContinueExecutor(logger *log.Logger) *TmuxContinueExecutor {
	return &TmuxContinueExecutor{logger: logger}
}

func NewTmuxSessionOpenChecker(logger *log.Logger) *TmuxSessionOpenChecker {
	return &TmuxSessionOpenChecker{logger: logger}
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

func (e *TmuxContinueExecutor) Close(session model.SessionSnapshot) error {
	paneID := strings.TrimSpace(session.TmuxPane)
	if paneID == "" {
		return fmt.Errorf("session %s is not bound to a tmux pane", session.SessionID)
	}

	dead, err := tmuxPaneDead(paneID)
	if err != nil {
		return err
	}
	if dead {
		return fmt.Errorf("tmux pane %s is already closed", paneID)
	}

	if e.logger != nil {
		e.logger.Printf("tmux close session=%s pane=%s", session.SessionID, paneID)
	}

	cmd := exec.Command("tmux", "kill-pane", "-t", paneID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-pane failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func (c *TmuxSessionOpenChecker) IsOpen(session model.SessionSnapshot) bool {
	paneID := strings.TrimSpace(session.TmuxPane)
	if paneID == "" {
		return false
	}

	open, err := tmuxPaneOpen(paneID)
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("tmux pane open check failed session=%s pane=%s: %v", session.SessionID, paneID, err)
		}
		return true
	}
	return open
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

func tmuxPaneOpen(paneID string) (bool, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{pane_dead}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Run(); err != nil {
		if tmuxTargetMissing(stdout.String()) {
			return false, nil
		}
		return false, fmt.Errorf("tmux pane lookup failed: %w: %s", err, strings.TrimSpace(stdout.String()))
	}
	return strings.TrimSpace(stdout.String()) != "1", nil
}

func tmuxTargetMissing(output string) bool {
	text := strings.ToLower(strings.TrimSpace(output))
	if text == "" {
		return false
	}

	markers := []string{
		"can't find pane",
		"can't find window",
		"can't find session",
		"no such pane",
		"no such window",
		"no such session",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
