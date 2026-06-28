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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vxider/agent-buddy/internal/config"
	"github.com/vxider/agent-buddy/internal/model"
	"github.com/vxider/agent-buddy/webserver"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to agent-buddy JSON config")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("load config: %v", err)
		return 1
	}

	logger := log.New(os.Stdout, "agent-buddy: ", log.LstdFlags|log.Lmsgprefix)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := webserver.Run(ctx, cfg, logger); err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			if status, statusErr := fetchStatus(cfg); statusErr == nil {
				logger.Printf("listen %s failed: agent-buddy is already running via %s (overall: %s, sessions: %d). Use `agent-buddy restart` or `agent-buddy stop` if you need to replace it", cfg.Listen.Address(), cfg.InternalBaseURL(), status.OverallState, status.SessionsCount)
				return 1
			}
			logger.Printf("listen %s failed: address already in use by another process or an unreachable agent-buddy. Try `agent-buddy stop` or `systemctl --user stop agent-buddy.service` if this should be the buddy daemon", cfg.Listen.Address())
			return 1
		}
		logger.Printf("webserver failed: %v", err)
		return 1
	}
	return 0
}

func runHook(args []string) int {
	return runAgentHook("codex", "hook", args)
}

func runClaudeHook(args []string) int {
	return runAgentHook("claude", "claude-hook", args)
}

func runAgentHook(agent, commandName string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: agent-buddy %s <event-name> [--config path]\n", commandName)
		return 2
	}

	eventName := args[0]
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to agent-buddy JSON config")
	_ = fs.Parse(args[1:])

	logger := log.New(os.Stderr, "agent-buddy: ", log.LstdFlags|log.Lmsgprefix)

	if isPermissionRequestEvent(eventName) {
		ringTerminalBell(logger)
	}

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
	payload.Agent = firstNonEmpty(payload.Agent, agent)
	payload.TmuxPane = firstNonEmpty(payload.TmuxPane, os.Getenv("TMUX_PANE"))
	if payload.TmuxPane != "" {
		payload.TmuxSession = firstNonEmpty(payload.TmuxSession, tmuxDisplayValue(payload.TmuxPane, "#{session_name}"))
		payload.TmuxWindow = firstNonEmpty(payload.TmuxWindow, tmuxDisplayValue(payload.TmuxPane, "#{window_id}"))
	}
	if agent == "codex" && payload.CodexPID == 0 {
		payload.CodexPID = findCodexPID()
	}
	if agent == "codex" && payload.ApprovalsReviewer == "" {
		payload.ApprovalsReviewer = detectApprovalsReviewer()
	}

	req := model.IngestRequest{
		Source:        agent + "-hook",
		Agent:         agent,
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
	httpReq.Header.Set("User-Agent", "agent-buddy/0.1 hook")

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

func isPermissionRequestEvent(eventName string) bool {
	return strings.EqualFold(strings.TrimSpace(eventName), "permission-request")
}

func ringTerminalBell(logger *log.Logger) {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		logger.Printf("warning: open tty for permission-request bell failed: %v", err)
		return
	}
	defer tty.Close()

	if _, err := tty.Write([]byte{'\a'}); err != nil {
		logger.Printf("warning: write permission-request bell failed: %v", err)
	}
}

// findCodexPID walks up the process tree from the current process to find
// the codex CLI process that spawned this hook. Returns 0 if not found.
func findCodexPID() int {
	pid := os.Getppid()
	for i := 0; i < 16 && pid > 1; i++ {
		name, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
		if err == nil {
			trimmed := strings.TrimSpace(string(name))
			if strings.HasPrefix(trimmed, "codex") {
				return pid
			}
		}
		// Walk up: read /proc/<pid>/stat field 4 (ppid)
		data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return 0
		}
		// stat format: pid (comm) state ppid ...
		// comm may contain spaces/parens, so find the last ")" and parse after it
		statStr := string(data)
		lastParen := strings.LastIndex(statStr, ")")
		if lastParen < 0 {
			return 0
		}
		fields := strings.Fields(statStr[lastParen+1:])
		if len(fields) < 2 {
			return 0
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil || ppid <= 1 || ppid == pid {
			return 0
		}
		pid = ppid
	}
	return 0
}

// detectApprovalsReviewer reads ~/.codex/config.toml to determine the
// approvals_reviewer setting. Returns "auto_review", "user", or "" if
// the setting cannot be determined.
func detectApprovalsReviewer() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "approvals_reviewer") {
			continue
		}
		// Extract value between quotes: approvals_reviewer = "auto_review"
		if idx := strings.Index(line, "\""); idx >= 0 {
			rest := line[idx+1:]
			if end := strings.Index(rest, "\""); end >= 0 {
				return rest[:end]
			}
		}
	}
	return ""
}
