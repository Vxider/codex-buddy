package main

import (
	"os"
	"path/filepath"
)

type userPaths struct {
	home            string
	binPath         string
	legacyHooksPath string
	codexConfigPath string
	servicePath     string
}

func resolveUserPaths() (userPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return userPaths{}, err
	}

	return userPaths{
		home:            home,
		binPath:         filepath.Join(home, ".local", "bin", "agent-buddy"),
		legacyHooksPath: filepath.Join(home, ".codex", "hooks.json"),
		codexConfigPath: filepath.Join(home, ".codex", "config.toml"),
		servicePath:     filepath.Join(home, ".config", "systemd", "user", "agent-buddy.service"),
	}, nil
}
