package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
)

type tmuxDotWindowState struct {
	WasRunning bool      `json:"was_running"`
	DownUnread bool      `json:"down_unread"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func runTmuxWindowDot(args []string) int {
	fs := flag.NewFlagSet("tmux-window-dot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath string
	var timeoutMS int
	var activeWindowID string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.IntVar(&timeoutMS, "timeout-ms", 150, "HTTP timeout in milliseconds")
	fs.StringVar(&activeWindowID, "active-window", "", "Current active tmux window id")
	if err := fs.Parse(args); err != nil {
		return 0
	}

	windowID := strings.TrimSpace(fs.Arg(0))
	if windowID == "" {
		return 0
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return 0
	}
	if timeoutMS <= 0 {
		timeoutMS = 150
	}

	activeWindowID = strings.TrimSpace(activeWindowID)
	if activeWindowID == "" {
		activeWindowID = tmuxDisplayValue("", "#{window_id}")
	}

	status, err := fetchStatusWithTimeout(cfg, time.Duration(timeoutMS)*time.Millisecond)
	if err != nil {
		return 0
	}

	now := time.Now().UTC()
	windowRunning, windowGoal := summarizeTmuxWindows(status)
	state := updatePersistedTmuxDotState(windowRunning, activeWindowID, now)

	if windowGoal[windowID] {
		printPulsingDot("#af00ff")
		return 0
	}
	if state[windowID].DownUnread {
		printPulsingDot("#ffff00")
		return 0
	}
	return 0
}

func updatePersistedTmuxDotState(windowRunning map[string]bool, activeWindowID string, now time.Time) map[string]tmuxDotWindowState {
	unlock, ok := lockTmuxDotState()
	if !ok {
		state, err := loadTmuxDotState()
		if err != nil {
			return make(map[string]tmuxDotWindowState)
		}
		return state
	}
	defer unlock()

	state, err := loadTmuxDotState()
	if err != nil {
		state = make(map[string]tmuxDotWindowState)
	}
	updateTmuxDotState(state, windowRunning, activeWindowID, now)
	_ = saveTmuxDotState(state)
	return state
}

func updateTmuxDotState(state map[string]tmuxDotWindowState, windowRunning map[string]bool, activeWindowID string, now time.Time) {
	if state == nil {
		return
	}
	for id, item := range state {
		running := windowRunning[id]
		if item.WasRunning && !running && id != activeWindowID {
			item.DownUnread = true
			item.UpdatedAt = now
		}
		item.WasRunning = running
		if id == activeWindowID {
			item.DownUnread = false
			item.UpdatedAt = now
		}
		state[id] = item
	}
	for id, running := range windowRunning {
		item := state[id]
		if item.WasRunning && !running && id != activeWindowID {
			item.DownUnread = true
		}
		item.WasRunning = running
		item.UpdatedAt = now
		if id == activeWindowID {
			item.DownUnread = false
		}
		state[id] = item
	}
}

func fetchStatusWithTimeout(cfg config.Config, timeout time.Duration) (apiStatus, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, cfg.InternalBaseURL()+"/v1/status", nil)
	if err != nil {
		return apiStatus{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return apiStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiStatus{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var status apiStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&status); err != nil {
		return apiStatus{}, err
	}
	return status, nil
}

func summarizeTmuxWindows(status apiStatus) (map[string]bool, map[string]bool) {
	running := make(map[string]bool)
	goal := make(map[string]bool)
	for _, session := range status.Sessions {
		windowID := strings.TrimSpace(session.TmuxWindow)
		if windowID == "" {
			continue
		}
		if tmuxSessionRunning(session.State) {
			running[windowID] = true
			if session.GoalState == model.GoalStateInProgress || codexGoalActive(session.SessionID) {
				goal[windowID] = true
			}
		}
	}
	return running, goal
}

func tmuxSessionRunning(state model.State) bool {
	switch state {
	case model.StateRun, model.StateRunning, model.StateRunningBash:
		return true
	default:
		return false
	}
}

func printPulsingDot(color string) {
	if time.Now().Unix()%2 == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "#[fg=%s]●", color)
	} else {
		_, _ = fmt.Fprint(os.Stdout, " ")
	}
}

func lockTmuxDotState() (func(), bool) {
	path, err := tmuxDotStateLockPath()
	if err != nil {
		return nil, false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, false
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, true
}

func loadTmuxDotState() (map[string]tmuxDotWindowState, error) {
	path, err := tmuxDotStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]tmuxDotWindowState), nil
		}
		return nil, err
	}
	var state map[string]tmuxDotWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = make(map[string]tmuxDotWindowState)
	}
	return state, nil
}

func saveTmuxDotState(state map[string]tmuxDotWindowState) error {
	path, err := tmuxDotStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmux-window-dots-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func tmuxDotStatePath() (string, error) {
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		return filepath.Join(cacheDir, "codex-buddy", "tmux-window-dots.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "codex-buddy", "tmux-window-dots.json"), nil
}

func tmuxDotStateLockPath() (string, error) {
	path, err := tmuxDotStatePath()
	if err != nil {
		return "", err
	}
	return path + ".lock", nil
}

func codexGoalActive(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || !safeSQLiteLiteralID(sessionID) {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	dbPath := filepath.Join(home, ".codex", "goals_1.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return false
	}
	cmd := exec.Command(
		"sqlite3",
		"-noheader",
		dbPath,
		"select 1 from thread_goals where thread_id = '"+sessionID+"' and status = 'active' limit 1;",
	)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "1"
}

func safeSQLiteLiteralID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
