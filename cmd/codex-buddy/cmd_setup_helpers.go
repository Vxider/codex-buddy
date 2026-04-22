package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vxider/codex-buddy/internal/config"
)

func installSelf(binPath string) error {
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return err
	}

	selfPath, err := os.Executable()
	if err != nil {
		return err
	}

	src, err := os.Open(selfPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(binPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func writeConfigJSON(path string, cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func writeHooksJSON(path, binPath, configPath string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	command := func(event string) string {
		return shellQuote(binPath) + " hook " + event + " --config " + shellQuote(configPath)
	}

	payload := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "startup|resume",
					"hooks": []any{
						map[string]any{
							"type":          "command",
							"command":       command("session-start"),
							"statusMessage": "codex-buddy session-start",
						},
					},
				},
			},
			"UserPromptSubmit": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command("user-prompt-submit"),
						},
					},
				},
			},
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command("pre-tool-use"),
						},
					},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command("post-tool-use"),
						},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command("stop"),
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeServiceFile(path, configPath string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=codex-buddy daemon
After=default.target

[Service]
ExecStart=%%h/.local/bin/codex-buddy serve --config %s
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
`, configPath)) + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func ensureCodexHooksEnabled(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte("[features]\ncodex_hooks = true\n"), 0o600)
		}
		return err
	}

	content := string(data)
	switch {
	case strings.Contains(content, "codex_hooks = true"):
		return nil
	case strings.Contains(content, "codex_hooks = false"):
		content = strings.Replace(content, "codex_hooks = false", "codex_hooks = true", 1)
	case strings.Contains(content, "[features]"):
		content = strings.Replace(content, "[features]", "[features]\ncodex_hooks = true", 1)
	default:
		content = strings.TrimRight(content, "\n") + "\n\n[features]\ncodex_hooks = true\n"
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
