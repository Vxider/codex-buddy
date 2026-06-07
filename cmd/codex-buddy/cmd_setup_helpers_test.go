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
		codexBuddyHooksBegin,
		`[[hooks.Stop]]`,
		`hooks = [{ type = "command", command = "old codex-buddy hook stop" }]`,
		codexBuddyHooksEnd,
		"",
		`[projects."/tmp/demo"]`,
		`trust_level = "trusted"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := writeCodexHooksConfig(configPath, "/tmp/bin/codex-buddy", "/tmp/codex-buddy/config.json"); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read hooks config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "old codex-buddy hook stop") {
		t.Fatalf("expected old managed block to be replaced, got:\n%s", content)
	}
	if count := strings.Count(content, codexBuddyHooksBegin); count != 1 {
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

func TestRemoveLegacyCodexBuddyHooksDeletesOldCodexBuddyFile(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	legacyHooks := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"'/tmp/bin/codex-buddy' hook stop --config '/tmp/config.json'"}]}]}}`
	if err := os.WriteFile(hooksPath, []byte(legacyHooks), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := removeLegacyCodexBuddyHooks(hooksPath); err != nil {
		t.Fatalf("remove legacy hooks: %v", err)
	}
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy hooks file to be removed, stat err=%v", err)
	}
}

func TestRemoveLegacyCodexBuddyHooksKeepsUnrelatedHooksFile(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	unrelatedHooks := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]}]}}`
	if err := os.WriteFile(hooksPath, []byte(unrelatedHooks), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := removeLegacyCodexBuddyHooks(hooksPath); err != nil {
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

func TestRemoveLegacyCodexBuddyHooksPrunesOnlyCodexBuddyEntries(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	mixedHooks := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "'/tmp/bin/codex-buddy' hook stop --config '/tmp/config.json'"
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
            "command": "'/tmp/bin/codex-buddy' hook user-prompt-submit --config '/tmp/config.json'"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(hooksPath, []byte(mixedHooks), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := removeLegacyCodexBuddyHooks(hooksPath); err != nil {
		t.Fatalf("remove legacy hooks: %v", err)
	}
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("expected mixed hooks file to remain: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "codex-buddy") {
		t.Fatalf("expected codex-buddy hooks to be removed, got:\n%s", content)
	}
	if !strings.Contains(content, "notify-send done") {
		t.Fatalf("expected unrelated hook to remain, got:\n%s", content)
	}
	if strings.Contains(content, "UserPromptSubmit") {
		t.Fatalf("expected empty event to be removed, got:\n%s", content)
	}
}
