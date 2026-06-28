package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCodexHooksConfigReplacesManagedBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	original := strings.Join([]string{
		`model = "gpt-5.5"`,
		"",
		agentBuddyHooksBegin,
		`[[hooks.Stop]]`,
		`hooks = [{ type = "command", command = "old agent-buddy hook stop" }]`,
		agentBuddyHooksEnd,
		"",
		`[projects."/tmp/demo"]`,
		`trust_level = "trusted"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := writeCodexHooksConfig(configPath, "/tmp/bin/agent-buddy", "/tmp/agent-buddy/config.json"); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read hooks config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "old agent-buddy hook stop") {
		t.Fatalf("expected old managed block to be replaced, got:\n%s", content)
	}
	if count := strings.Count(content, agentBuddyHooksBegin); count != 1 {
		t.Fatalf("expected one managed block begin marker, got %d in:\n%s", count, content)
	}
	if !strings.Contains(content, `[projects."/tmp/demo"]`) {
		t.Fatalf("expected unrelated config to be preserved, got:\n%s", content)
	}
	if !strings.Contains(content, `hook session-start --config`) {
		t.Fatalf("expected session-start hook to be written, got:\n%s", content)
	}
	if !strings.Contains(content, `[[hooks.PermissionRequest]]`) {
		t.Fatalf("expected permission-request hook to be written, got:\n%s", content)
	}
	if !strings.Contains(content, `hook permission-request --config`) {
		t.Fatalf("expected permission-request command to be written, got:\n%s", content)
	}
}

func TestIsPermissionRequestEvent(t *testing.T) {
	tests := []struct {
		name  string
		event string
		want  bool
	}{
		{name: "exact", event: "permission-request", want: true},
		{name: "case insensitive", event: "Permission-Request", want: true},
		{name: "trimmed", event: " permission-request\n", want: true},
		{name: "other event", event: "pre-tool-use", want: false},
		{name: "empty", event: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermissionRequestEvent(tt.event); got != tt.want {
				t.Fatalf("isPermissionRequestEvent(%q) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestClaudeHooksSnippetUsesPlainAbsoluteCommands(t *testing.T) {
	content := claudeHooksSnippet("/tmp/bin/agent-buddy", "/tmp/agent-buddy/config.json")

	if !strings.Contains(content, `"command": "/tmp/bin/agent-buddy claude-hook user-prompt-submit --config /tmp/agent-buddy/config.json"`) {
		t.Fatalf("expected plain claude hook command, got:\n%s", content)
	}
	if strings.Contains(content, `'/tmp/bin/agent-buddy'`) || strings.Contains(content, `'/tmp/agent-buddy/config.json'`) {
		t.Fatalf("expected claude hook command without shell quotes, got:\n%s", content)
	}
}

func TestContainsBuddyHookCommandMatchesClaudeHook(t *testing.T) {
	command := "/tmp/bin/agent-buddy claude-hook user-prompt-submit --config /tmp/config.json"
	if !containsBuddyHookCommand(command) {
		t.Fatalf("expected claude-hook command to match")
	}
}

func TestRemoveLegacyAgentBuddyHooksDeletesOldAgentBuddyFile(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	legacyHooks := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"'/tmp/bin/agent-buddy' hook stop --config '/tmp/config.json'"}]}]}}`
	if err := os.WriteFile(hooksPath, []byte(legacyHooks), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := removeLegacyAgentBuddyHooks(hooksPath); err != nil {
		t.Fatalf("remove legacy hooks: %v", err)
	}
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy hooks file to be removed, stat err=%v", err)
	}
}

func TestRemoveLegacyAgentBuddyHooksKeepsUnrelatedHooksFile(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	unrelatedHooks := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]}]}}`
	if err := os.WriteFile(hooksPath, []byte(unrelatedHooks), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := removeLegacyAgentBuddyHooks(hooksPath); err != nil {
		t.Fatalf("remove legacy hooks: %v", err)
	}
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("expected unrelated hooks file to remain: %v", err)
	}
	if string(data) != unrelatedHooks {
		t.Fatalf("expected unrelated hooks file to be unchanged, got:\n%s", string(data))
	}
}

func TestRemoveLegacyAgentBuddyHooksPrunesOnlyAgentBuddyEntries(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	mixedHooks := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "'/tmp/bin/agent-buddy' hook stop --config '/tmp/config.json'"
          },
          {
            "type": "command",
            "command": "notify-send done"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "'/tmp/bin/agent-buddy' claude-hook user-prompt-submit --config '/tmp/config.json'"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(hooksPath, []byte(mixedHooks), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := removeLegacyAgentBuddyHooks(hooksPath); err != nil {
		t.Fatalf("remove legacy hooks: %v", err)
	}
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("expected mixed hooks file to remain: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "agent-buddy") {
		t.Fatalf("expected agent-buddy hooks to be removed, got:\n%s", content)
	}
	if !strings.Contains(content, "notify-send done") {
		t.Fatalf("expected unrelated hook to remain, got:\n%s", content)
	}
	if strings.Contains(content, "UserPromptSubmit") {
		t.Fatalf("expected empty event to be removed, got:\n%s", content)
	}
}
