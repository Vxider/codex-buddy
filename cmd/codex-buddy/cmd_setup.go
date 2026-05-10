package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vxider/codex-buddy/internal/config"
)

func runSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	var configPath string
	var host string
	var port int
	var skipSystemd bool
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.StringVar(&host, "host", "0.0.0.0", "Listen host for the buddy web server")
	fs.IntVar(&port, "port", 8787, "Listen port for the buddy web server")
	fs.BoolVar(&skipSystemd, "skip-systemd", false, "Do not install or enable the systemd user service")
	_ = fs.Parse(args)

	logger := log.New(os.Stdout, "codex-buddy: ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Printf("load config: %v", err)
		return 1
	}

	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		logger.Printf("resolve config path: %v", err)
		return 1
	}

	paths, err := resolveUserPaths()
	if err != nil {
		logger.Printf("resolve user home: %v", err)
		return 1
	}

	cfg.Listen.Host = host
	cfg.Listen.Port = port
	cfg.HookClient.AutostartEnabled = !skipSystemd
	cfg.HookClient.AutostartCommand = []string{"systemctl", "--user", "start", "codex-buddy.service"}

	if err := installArtifacts(cfg, resolvedConfigPath, paths, skipSystemd); err != nil {
		logger.Printf("setup failed: %v", err)
		return 1
	}

	logger.Printf("setup completed")
	logger.Printf("binary: %s", paths.binPath)
	logger.Printf("config: %s", resolvedConfigPath)
	logger.Printf("codex hooks: %s", paths.codexConfigPath)
	if !skipSystemd {
		logger.Printf("service: %s", paths.servicePath)
	}
	logger.Printf("status page: http://127.0.0.1:%d/status", cfg.Listen.Port)
	return 0
}

func installArtifacts(cfg config.Config, resolvedConfigPath string, paths userPaths, skipSystemd bool) error {
	if err := installSelf(paths.binPath); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	if err := writeConfigJSON(resolvedConfigPath, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := writeCodexHooksConfig(paths.codexConfigPath, paths.binPath, resolvedConfigPath); err != nil {
		return fmt.Errorf("write hooks: %w", err)
	}
	if err := removeLegacyCodexBuddyHooks(paths.legacyHooksPath); err != nil {
		return fmt.Errorf("remove legacy hooks: %w", err)
	}
	if skipSystemd {
		return nil
	}
	if err := writeServiceFile(paths.servicePath, resolvedConfigPath); err != nil {
		return fmt.Errorf("write systemd service: %w", err)
	}
	if err := runCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemd daemon-reload failed: %w", err)
	}
	if err := runCommand("systemctl", "--user", "enable", "--now", "codex-buddy.service"); err != nil {
		return fmt.Errorf("systemd enable/start failed: %w", err)
	}
	return nil
}
