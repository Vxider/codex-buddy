package main

import (
	"os"
	"path/filepath"
)

type userPaths struct {
	home            string
	binPath         string
	hooksPath       string
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
		binPath:         filepath.Join(home, ".local", "bin", "codex-buddy"),
		hooksPath:       filepath.Join(home, ".codex", "hooks.json"),
		codexConfigPath: filepath.Join(home, ".codex", "config.toml"),
		servicePath:     filepath.Join(home, ".config", "systemd", "user", "codex-buddy.service"),
	}, nil
}
