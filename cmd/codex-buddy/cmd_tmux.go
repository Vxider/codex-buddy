package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
)

func runTmuxWindowDot(args []string) int {
	fs := flag.NewFlagSet("tmux-window-dot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath string
	var timeoutMS int
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.IntVar(&timeoutMS, "timeout-ms", 150, "HTTP timeout in milliseconds")
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

	values := url.Values{}
	values.Set("window", windowID)
	if activeWindowID := tmuxDisplayValue("", "#{window_id}"); activeWindowID != "" {
		values.Set("active_window", activeWindowID)
	}
	endpoint := cfg.InternalBaseURL() + "/v1/tmux/window-goal-dot?" + values.Encode()
	client := &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
	resp, err := client.Get(endpoint)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return 0
	}
	if len(body) == 0 {
		return 0
	}
	_, _ = fmt.Fprint(os.Stdout, string(body))
	return 0
}
