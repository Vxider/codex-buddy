package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
)

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("FAIL load config: %v\n", err)
		return 1
	}

	paths, err := resolveUserPaths()
	if err != nil {
		fmt.Printf("FAIL resolve user home: %v\n", err)
		return 1
	}

	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("FAIL resolve config path: %v\n", err)
		return 1
	}

	checks := []struct {
		name string
		err  error
	}{
		{"config", checkFileExists(resolvedConfigPath)},
		{"codex config", checkFileExists(paths.codexConfigPath)},
		{"service", checkFileExists(paths.servicePath)},
		{"codex hooks", checkCodexHooksEnabled(paths.codexConfigPath)},
		{"legacy hooks", checkNoLegacyCodexBuddyHooks(paths.legacyHooksPath)},
		{"daemon http", checkStatusEndpoint(cfg)},
	}

	failed := false
	for _, check := range checks {
		if check.err != nil {
			failed = true
			fmt.Printf("FAIL %-20s %v\n", check.name, check.err)
			continue
		}
		fmt.Printf("OK   %s\n", check.name)
	}

	if err := runCommand("systemctl", "--user", "is-active", "codex-buddy.service"); err != nil {
		failed = true
		fmt.Printf("FAIL service active        %v\n", err)
	} else {
		fmt.Printf("OK   service active\n")
	}

	if failed {
		return 1
	}
	return 0
}

func runLogs(args []string) int {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	var lines int
	fs.IntVar(&lines, "lines", 50, "Number of recent journal lines to print")
	_ = fs.Parse(args)

	if lines <= 0 {
		lines = 50
	}

	cmd := exec.Command(
		"journalctl",
		"--user",
		"--unit", "codex-buddy.service",
		"--no-pager",
		"-n", fmt.Sprintf("%d", lines),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Printf("read logs failed: %v\n", err)
		return 1
	}
	return 0
}

func runStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}

	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("resolve config path: %v\n", err)
		return 1
	}

	if status, err := fetchStatus(cfg); err == nil {
		fmt.Printf("already running via %s (overall: %s, sessions: %d)\n", cfg.InternalBaseURL(), status.OverallState, status.SessionsCount)
		return 0
	}

	if hasServiceFile, err := hasSystemdServiceFile(); err == nil && hasServiceFile {
		if err := startSystemdService(); err != nil {
			fmt.Printf("start failed: systemd start: %v\n", err)
			return 1
		}
		fmt.Printf("started systemd user service: codex-buddy.service\n")
		return 0
	}

	if err := startManagedServer(cfg, resolvedConfigPath); err != nil {
		fmt.Printf("start failed: %v\n", err)
		return 1
	}

	fmt.Printf("started codex-buddy in background via %s\n", cfg.InternalBaseURL())
	return 0
}

func runRestart(args []string) int {
	fs := flag.NewFlagSet("restart", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}

	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("resolve config path: %v\n", err)
		return 1
	}

	if hasServiceFile, err := hasSystemdServiceFile(); err == nil && hasServiceFile {
		if err := restartSystemdService(); err != nil {
			fmt.Printf("restart failed: systemd restart: %v\n", err)
			return 1
		}
		fmt.Printf("restarted systemd user service: codex-buddy.service\n")
		return 0
	}

	if _, err := fetchStatus(cfg); err == nil {
		if err := requestShutdown(cfg); err != nil {
			fmt.Printf("restart failed: shutdown request: %v\n", err)
			return 1
		}
		if err := waitForServerDown(cfg, 5*time.Second); err != nil {
			fmt.Printf("restart failed: %v\n", err)
			return 1
		}
	}

	if err := startManagedServer(cfg, resolvedConfigPath); err != nil {
		fmt.Printf("restart failed: %v\n", err)
		return 1
	}

	fmt.Printf("restarted codex-buddy in background via %s\n", cfg.InternalBaseURL())
	return 0
}

func runStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}

	if active, err := isSystemdServiceActive(); err == nil && active {
		if err := stopSystemdService(); err == nil {
			fmt.Printf("stopped systemd user service: codex-buddy.service\n")
			return 0
		}
		fmt.Printf("stop failed: systemd stop: %v\n", err)
		return 1
	}

	if err := requestShutdown(cfg); err == nil {
		fmt.Printf("stop requested via %s\n", cfg.ShutdownURL())
		return 0
	} else {
		fmt.Printf("stop failed: shutdown request: %v\n", err)
		return 1
	}
}

func runUninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	var configPath string
	var keepBinary bool
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.BoolVar(&keepBinary, "keep-binary", false, "Keep ~/.local/bin/codex-buddy after uninstall")
	_ = fs.Parse(args)

	paths, err := resolveUserPaths()
	if err != nil {
		fmt.Printf("resolve user home: %v\n", err)
		return 1
	}

	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("resolve config path: %v\n", err)
		return 1
	}

	_ = runCommand("systemctl", "--user", "disable", "--now", "codex-buddy.service")
	_ = os.Remove(paths.servicePath)
	_ = runCommand("systemctl", "--user", "daemon-reload")

	if err := removeCodexBuddyHooksConfig(paths.codexConfigPath); err != nil {
		fmt.Printf("remove hooks: %v\n", err)
		return 1
	}
	if err := removeLegacyCodexBuddyHooks(paths.legacyHooksPath); err != nil {
		fmt.Printf("remove legacy hooks: %v\n", err)
		return 1
	}
	if err := os.Remove(resolvedConfigPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("remove config: %v\n", err)
		return 1
	}
	if !keepBinary {
		if err := os.Remove(paths.binPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove binary: %v\n", err)
			return 1
		}
	}

	fmt.Printf("removed service: %s\n", paths.servicePath)
	fmt.Printf("removed hooks from: %s\n", paths.codexConfigPath)
	fmt.Printf("removed config: %s\n", resolvedConfigPath)
	if keepBinary {
		fmt.Printf("kept binary: %s\n", paths.binPath)
	} else {
		fmt.Printf("removed binary: %s\n", paths.binPath)
	}
	return 0
}

func stopSystemdService() error {
	return runSystemdServiceCommand("stop")
}

func startSystemdService() error {
	return runSystemdServiceCommand("start")
}

func restartSystemdService() error {
	return runSystemdServiceCommand("restart")
}

func runSystemdServiceCommand(action string) error {
	cmd := exec.Command("systemctl", "--user", action, "codex-buddy.service")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(string(bytes.TrimSpace(output)))
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func isSystemdServiceActive() (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", "codex-buddy.service")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func hasSystemdServiceFile() (bool, error) {
	paths, err := resolveUserPaths()
	if err != nil {
		return false, err
	}
	if err := checkFileExists(paths.servicePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func startManagedServer(cfg config.Config, resolvedConfigPath string) error {
	if len(cfg.HookClient.AutostartCommand) > 0 {
		return startDetachedCommand(cfg.HookClient.AutostartCommand[0], cfg.HookClient.AutostartCommand[1:]...)
	}
	return startDetachedServe(resolvedConfigPath)
}

func startDetachedServe(resolvedConfigPath string) error {
	selfPath, err := os.Executable()
	if err != nil {
		return err
	}
	return startDetachedCommand(selfPath, "serve", "--config", resolvedConfigPath)
}

func startDetachedCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func waitForServerDown(cfg config.Config, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := fetchStatus(cfg); err != nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("server did not stop within %s", timeout)
}
