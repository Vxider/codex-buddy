package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

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
		{"hooks", checkFileExists(paths.hooksPath)},
		{"service", checkFileExists(paths.servicePath)},
		{"codex hooks enabled", checkCodexHooksEnabled(paths.codexConfigPath)},
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

	if err := os.Remove(paths.hooksPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("remove hooks: %v\n", err)
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
	fmt.Printf("removed hooks: %s\n", paths.hooksPath)
	fmt.Printf("removed config: %s\n", resolvedConfigPath)
	if keepBinary {
		fmt.Printf("kept binary: %s\n", paths.binPath)
	} else {
		fmt.Printf("removed binary: %s\n", paths.binPath)
	}
	return 0
}
