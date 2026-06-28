package present

import (
	"strings"

	"github.com/vxider/agent-buddy/internal/model"
)

func ErrorTitle(session model.SessionSnapshot) string {
	if strings.TrimSpace(session.LastBashCommand) != "" {
		return "Command failed"
	}
	return AgentDisplayName(session.Agent) + " error"
}

func ErrorSummary(session model.SessionSnapshot) string {
	if command := normalizeInlineWhitespace(session.LastBashCommand); command != "" {
		return "Command failed: " + clipHead(command, 160)
	}

	if message := readableErrorText(session.LastAssistantMessage); message != "" {
		return message
	}
	if message := readableErrorText(session.LastError); message != "" {
		return message
	}

	if prompt := normalizeInlineWhitespace(session.LastUserPromptPreview); prompt != "" {
		return "Request failed while working on: " + clipHead(prompt, 120)
	}
	return AgentDisplayName(session.Agent) + " run failed"
}

func AgentDisplayName(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "claude":
		return "Claude"
	case "codex", "":
		return "Codex"
	default:
		trimmed := strings.TrimSpace(agent)
		return strings.ToUpper(string([]rune(trimmed)[0])) + string([]rune(trimmed)[1:])
	}
}

func readableErrorText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	lines := strings.Split(value, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := normalizeInlineWhitespace(lines[i])
		if line == "" {
			continue
		}
		if seemsReadable(line) {
			return clipHead(line, 160)
		}
	}

	line := normalizeInlineWhitespace(lines[len(lines)-1])
	if line == "" {
		return ""
	}
	return clipHead(line, 160)
}

func seemsReadable(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}

	keywords := []string{
		"error",
		"failed",
		"cannot",
		"can't",
		"not found",
		"denied",
		"timeout",
		"timed out",
		"unexpected",
		"invalid",
		"no such",
		"missing",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}

	// Avoid surfacing obviously noisy lines such as code blocks or stack traces.
	if strings.HasPrefix(lower, "at ") || strings.HasPrefix(lower, "stack trace") {
		return false
	}
	if strings.ContainsAny(value, "{}[]") && !strings.Contains(lower, "error") && !strings.Contains(lower, "failed") {
		return false
	}
	return len(strings.Fields(value)) <= 24
}

func normalizeInlineWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func clipHead(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
