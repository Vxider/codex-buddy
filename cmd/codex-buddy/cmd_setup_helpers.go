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

const (
	codexBuddyHooksBegin = "# BEGIN codex-buddy hooks"
	codexBuddyHooksEnd   = "# END codex-buddy hooks"
)

func writeCodexHooksConfig(path, binPath, configPath string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	command := func(event string) string {
		return shellQuote(binPath) + " hook " + event + " --config " + shellQuote(configPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	content := removeManagedHooksBlock(string(data))
	hooks := strings.TrimSpace(fmt.Sprintf(`
%s
[[hooks.SessionStart]]
matcher = "startup|resume"
hooks = [{ type = "command", command = %s, statusMessage = "codex-buddy session-start" }]

[[hooks.UserPromptSubmit]]
hooks = [{ type = "command", command = %s }]

[[hooks.PreToolUse]]
matcher = "Bash"
hooks = [{ type = "command", command = %s }]

[[hooks.PostToolUse]]
matcher = "Bash"
hooks = [{ type = "command", command = %s }]

[[hooks.Stop]]
hooks = [{ type = "command", command = %s }]
%s
`, codexBuddyHooksBegin, tomlQuote(command("session-start")), tomlQuote(command("user-prompt-submit")), tomlQuote(command("pre-tool-use")), tomlQuote(command("post-tool-use")), tomlQuote(command("stop")), codexBuddyHooksEnd))

	content = strings.TrimRight(content, "\n")
	if content != "" {
		content += "\n\n"
	}
	content += hooks + "\n"
	return os.WriteFile(path, []byte(content), 0o600)
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

func removeLegacyCodexBuddyHooks(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !strings.Contains(string(data), "codex-buddy hook") {
		return nil
	}
	return os.Remove(path)
}

func removeCodexBuddyHooksConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	content := removeManagedHooksBlock(string(data))
	return os.WriteFile(path, []byte(strings.TrimRight(content, "\n")+"\n"), 0o600)
}

func managedHooksBlock(content string) string {
	start := strings.Index(content, codexBuddyHooksBegin)
	end := strings.Index(content, codexBuddyHooksEnd)
	if start < 0 || end < start {
		return ""
	}
	end += len(codexBuddyHooksEnd)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return content[start:end]
}

func removeManagedHooksBlock(content string) string {
	start := strings.Index(content, codexBuddyHooksBegin)
	end := strings.Index(content, codexBuddyHooksEnd)
	if start < 0 || end < start {
		return content
	}
	end += len(codexBuddyHooksEnd)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return strings.TrimRight(content[:start], "\n") + content[end:]
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func tomlQuote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}
