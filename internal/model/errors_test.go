package model

import "testing"

func TestIsCodexInterruptionTextRecognizesServiceAndTransportFailures(t *testing.T) {
	tests := []string{
		"HTTP 401 Unauthorized",
		"response status: 402 Payment Required",
		"HTTP 403 Forbidden",
		"HTTP 429 Too Many Requests",
		"HTTP 503 Service Unavailable",
		"quota exhausted",
		"network connection timed out",
		"网络连接中断，额度已用光",
	}
	for _, text := range tests {
		if !IsCodexInterruptionText(text) {
			t.Errorf("expected %q to be recognized as a Codex interruption", text)
		}
	}
}

func TestIsCodexInterruptionTextDoesNotMatchOrdinaryToolFailure(t *testing.T) {
	for _, text := range []string{"tool failed internally", "command exited with status 1", "FAIL api", "Approval required before editing billing copy"} {
		if IsCodexInterruptionText(text) {
			t.Errorf("did not expect ordinary tool failure %q to be recognized", text)
		}
	}
}

func TestIsCodexInterruptionTextDoesNotMatchLargerStatusCode(t *testing.T) {
	for _, text := range []string{
		"status 1401 from a local command",
		"status 1503 from a local command",
	} {
		if IsCodexInterruptionText(text) {
			t.Fatalf("did not expect an embedded status code to match: %q", text)
		}
	}
}
