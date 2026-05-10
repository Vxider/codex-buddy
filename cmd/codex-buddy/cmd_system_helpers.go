package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/vxider/codex-buddy/internal/config"
)

func checkCodexHooksEnabled(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, codexBuddyHooksBegin) || !strings.Contains(content, "codex-buddy hook") {
		return errors.New("codex-buddy hooks are not configured in config.toml")
	}
	return nil
}

func checkNoLegacyCodexBuddyHooks(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if strings.Contains(string(data), "codex-buddy hook") {
		return errors.New("legacy hooks.json still contains codex-buddy hooks")
	}
	return nil
}

func checkFileExists(path string) error {
	_, err := os.Stat(path)
	return err
}

func checkStatusEndpoint(cfg config.Config) error {
	_, err := fetchStatus(cfg)
	return err
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
