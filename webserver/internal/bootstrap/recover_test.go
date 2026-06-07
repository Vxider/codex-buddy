package bootstrap

import "testing"

func TestParsePaneLine(t *testing.T) {
	line := "0\t@77\t%211\t3969652\t0\tcodex\t/home/test/workspace/example"
	out, ok := parsePaneLine(line)
	if !ok {
		t.Fatalf("expected pane line to parse")
	}
	if out.TmuxPane != "%211" {
		t.Fatalf("unexpected pane id: %q", out.TmuxPane)
	}
	if out.PanePID != 3969652 {
		t.Fatalf("unexpected pane pid: %d", out.PanePID)
	}
	if out.CurrentCommand != "codex" {
		t.Fatalf("unexpected command: %q", out.CurrentCommand)
	}
}

func TestFindCodexProcessForPanePrefersResumeDescendant(t *testing.T) {
	pane := pane{PanePID: 43179}
	processes := map[int]process{
		43179:  {PID: 43179, PPID: 10567, Args: "-zsh"},
		500001: {PID: 500001, PPID: 43179, Args: "codex"},
		500002: {PID: 500002, PPID: 43179, Args: "codex resume 019db9c6-a235-7411-aef8-3dc5ecc34a2e"},
	}

	out, ok := findCodexProcessForPane(pane, processes)
	if !ok {
		t.Fatalf("expected to find codex process")
	}
	if got := sessionIDFromArgs(out.Args); got != "019db9c6-a235-7411-aef8-3dc5ecc34a2e" {
		t.Fatalf("unexpected recovered session id: %q", got)
	}
}

func TestExtractSessionIDFromTranscriptPath(t *testing.T) {
	path := "/home/test/.codex/sessions/2026/04/24/rollout-2026-04-24T21-56-57-019dbfc7-8489-7b40-925f-a37935a06117.jsonl"
	if got := extractSessionIDFromTranscriptPath(path); got != "019dbfc7-8489-7b40-925f-a37935a06117" {
		t.Fatalf("unexpected session id: %q", got)
	}
}
