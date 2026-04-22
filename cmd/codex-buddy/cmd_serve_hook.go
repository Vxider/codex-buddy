package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
	"github.com/vxider/codex-buddy/webserver"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("load config: %v", err)
		return 1
	}

	logger := log.New(os.Stdout, "codex-buddy: ", log.LstdFlags|log.Lmsgprefix)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := webserver.Run(ctx, cfg, logger); err != nil {
		logger.Printf("webserver failed: %v", err)
		return 1
	}
	return 0
}

func runHook(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: codex-buddy hook <event-name> [--config path]")
		return 2
	}

	eventName := args[0]
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args[1:])

	logger := log.New(os.Stderr, "codex-buddy: ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Printf("load config: %v", err)
		return 1
	}

	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		logger.Printf("read stdin: %v", err)
		return 1
	}
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}

	var payload model.HookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		logger.Printf("warning: parse hook payload failed, forwarding partial event: %v", err)
	}
	payload.TmuxPane = firstNonEmpty(payload.TmuxPane, os.Getenv("TMUX_PANE"))
	if payload.TmuxPane != "" {
		payload.TmuxSession = firstNonEmpty(payload.TmuxSession, tmuxDisplayValue(payload.TmuxPane, "#{session_name}"))
		payload.TmuxWindow = firstNonEmpty(payload.TmuxWindow, tmuxDisplayValue(payload.TmuxPane, "#{window_id}"))
	}

	req := model.IngestRequest{
		Source:        "codex-hook",
		EventName:     eventName,
		HookEventName: firstNonEmpty(payload.HookEventName, eventName),
		ReceivedAt:    time.Now().UTC(),
		Payload:       payload,
	}

	if err := postHook(cfg, req); err == nil {
		return 0
	} else {
		logger.Printf("initial post failed: %v", err)
	}

	if cfg.HookClient.AutostartEnabled {
		if err := autostart(cfg, logger); err != nil {
			logger.Printf("autostart failed: %v", err)
		} else {
			time.Sleep(500 * time.Millisecond)
			if err := postHook(cfg, req); err == nil {
				return 0
			} else {
				logger.Printf("retry post failed: %v", err)
			}
		}
	}

	return 1
}

func postHook(cfg config.Config, req model.IngestRequest) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	timeout := time.Duration(cfg.HookClient.RequestTimeoutMS) * time.Millisecond
	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequest(http.MethodPost, cfg.IngestURL(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "codex-buddy/0.1 hook")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

func autostart(cfg config.Config, logger *log.Logger) error {
	if len(cfg.HookClient.AutostartCommand) == 0 {
		return errors.New("no autostart_command configured")
	}

	cmd := exec.Command(cfg.HookClient.AutostartCommand[0], cfg.HookClient.AutostartCommand[1:]...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}

	logger.Printf("triggered daemon autostart")
	return nil
}

func tmuxDisplayValue(paneID, format string) string {
	if paneID == "" || format == "" {
		return ""
	}

	cmd := exec.Command("tmux", "display-message", "-p", "-t", paneID, format)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
