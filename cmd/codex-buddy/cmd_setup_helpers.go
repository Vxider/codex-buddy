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
	binDir := filepath.Dir(binPath)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
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

	tmp, err := os.CreateTemp(binDir, ".codex-buddy.install.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
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

	var config legacyHooksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		if strings.Contains(string(data), "codex-buddy") && strings.Contains(string(data), " hook ") {
			return os.Remove(path)
		}
		return nil
	}

	changed := false
	for event, matchers := range config.Hooks {
		keptMatchers := matchers[:0]
		for _, matcher := range matchers {
			keptHooks := matcher.Hooks[:0]
			for _, hook := range matcher.Hooks {
				if isLegacyCodexBuddyHookCommand(hook.Command) {
					changed = true
					continue
				}
				keptHooks = append(keptHooks, hook)
			}
			matcher.Hooks = keptHooks
			if len(matcher.Hooks) > 0 {
				keptMatchers = append(keptMatchers, matcher)
			}
		}
		if len(keptMatchers) == 0 {
			delete(config.Hooks, event)
			continue
		}
		config.Hooks[event] = keptMatchers
	}

	if !changed {
		return nil
	}
	if len(config.Hooks) == 0 {
		return os.Remove(path)
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	return os.WriteFile(path, updated, 0o600)
}

type legacyHooksConfig struct {
	Hooks map[string][]legacyHookMatcher `json:"hooks,omitempty"`
}

type legacyHookMatcher struct {
	Matcher string              `json:"matcher,omitempty"`
	Hooks   []legacyHookCommand `json:"hooks,omitempty"`
}

type legacyHookCommand struct {
	Type          string `json:"type,omitempty"`
	Command       string `json:"command,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

func isLegacyCodexBuddyHookCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	return strings.Contains(command, "codex-buddy") && strings.Contains(command, " hook ")
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
