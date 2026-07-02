package bootstrap

import "testing"

func TestPaneTextIndicatesReady(t *testing.T) {
	sessionID := "019f178e-8c82-7e81-8a18-df44507e7470"
	text := "1 background terminal running\n\n  gpt-5.3-codex medium - Ready - 019f178e-8c82-7e81-8a18-df44507e7470"

	if !paneTextIndicatesReady(text, sessionID) {
		t.Fatalf("expected ready pane text to be detected")
	}
}

func TestPaneTextIndicatesReadyRequiresReadyLine(t *testing.T) {
	sessionID := "019f178e-8c82-7e81-8a18-df44507e7470"
	text := "Running command\n019f178e-8c82-7e81-8a18-df44507e7470"

	if paneTextIndicatesReady(text, sessionID) {
		t.Fatalf("expected non-ready pane text not to be detected")
	}
}
