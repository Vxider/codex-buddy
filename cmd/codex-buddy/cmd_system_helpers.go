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
	if !strings.Contains(string(data), "codex_hooks = true") {
		return errors.New("codex_hooks is not enabled")
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
