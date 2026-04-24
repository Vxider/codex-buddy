package bootstrap

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Session struct {
	SessionID      string
	TranscriptPath string
	CWD            string
	TmuxPane       string
	TmuxSession    string
	TmuxWindow     string
}

type pane struct {
	TmuxSession    string
	TmuxWindow     string
	TmuxPane       string
	PanePID        int
	PaneDead       bool
	CurrentCommand string
	CWD            string
}

type process struct {
	PID  int
	PPID int
	Args string
}

func RecoverOpenSessions(logger *log.Logger) ([]Session, error) {
	panes, err := listCodexPanes()
	if err != nil {
		return nil, err
	}
	if len(panes) == 0 {
		return nil, nil
	}

	processes, err := listProcesses()
	if err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(panes))
	seen := make(map[string]struct{}, len(panes))
	for _, pane := range panes {
		codexProc, ok := findCodexProcessForPane(pane, processes)
		if !ok {
			if logger != nil {
				logger.Printf("bootstrap skipped pane=%s: no codex process found below pane_pid=%d", pane.TmuxPane, pane.PanePID)
			}
			continue
		}

		transcriptPath := transcriptPathFromProcessFDs(codexProc.PID)
		sessionID := extractSessionIDFromTranscriptPath(transcriptPath)
		if sessionID == "" {
			sessionID = sessionIDFromArgs(codexProc.Args)
		}
		if sessionID == "" {
			if logger != nil {
				logger.Printf("bootstrap skipped pane=%s pid=%d: no session id found", pane.TmuxPane, codexProc.PID)
			}
			continue
		}

		if transcriptPath == "" {
			transcriptPath = findTranscriptPathBySessionID(sessionID)
		}

		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		sessions = append(sessions, Session{
			SessionID:      sessionID,
			TranscriptPath: transcriptPath,
			CWD:            pane.CWD,
			TmuxPane:       pane.TmuxPane,
			TmuxSession:    pane.TmuxSession,
			TmuxWindow:     pane.TmuxWindow,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].SessionID < sessions[j].SessionID
	})
	return sessions, nil
}

func listCodexPanes() ([]pane, error) {
	cmd := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name}\t#{window_id}\t#{pane_id}\t#{pane_pid}\t#{pane_dead}\t#{pane_current_command}\t#{pane_current_path}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list tmux panes: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	panes := make([]pane, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parsed, ok := parsePaneLine(line)
		if !ok {
			continue
		}
		if parsed.PaneDead || !strings.EqualFold(parsed.CurrentCommand, "codex") {
			continue
		}
		panes = append(panes, parsed)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return panes, nil
}

func parsePaneLine(line string) (pane, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) != 7 {
		return pane{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil || pid <= 0 {
		return pane{}, false
	}
	return pane{
		TmuxSession:    strings.TrimSpace(parts[0]),
		TmuxWindow:     strings.TrimSpace(parts[1]),
		TmuxPane:       strings.TrimSpace(parts[2]),
		PanePID:        pid,
		PaneDead:       strings.TrimSpace(parts[4]) == "1",
		CurrentCommand: strings.TrimSpace(parts[5]),
		CWD:            strings.TrimSpace(parts[6]),
	}, true
}

func listProcesses() (map[int]process, error) {
	cmd := exec.Command("ps", "-eo", "pid=,ppid=,args=")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	processes := make(map[int]process)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parsed, ok := parseProcessLine(line)
		if !ok {
			continue
		}
		processes[parsed.PID] = parsed
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return processes, nil
}

func parseProcessLine(line string) (process, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return process{}, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return process{}, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil || ppid < 0 {
		return process{}, false
	}
	return process{
		PID:  pid,
		PPID: ppid,
		Args: strings.Join(fields[2:], " "),
	}, true
}

func findCodexProcessForPane(p pane, processes map[int]process) (process, bool) {
	children := make(map[int][]process)
	for _, proc := range processes {
		children[proc.PPID] = append(children[proc.PPID], proc)
	}

	queue := append([]process(nil), children[p.PanePID]...)
	var fallback process
	var hasFallback bool
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if isCodexProcess(current.Args) {
			if sessionIDFromArgs(current.Args) != "" {
				return current, true
			}
			if !hasFallback {
				fallback = current
				hasFallback = true
			}
		}

		queue = append(queue, children[current.PID]...)
	}

	if hasFallback {
		return fallback, true
	}
	return process{}, false
}

func isCodexProcess(args string) bool {
	args = strings.TrimSpace(args)
	if args == "codex" {
		return true
	}
	return strings.HasPrefix(args, "codex ")
}

func sessionIDFromArgs(args string) string {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) >= 3 && fields[0] == "codex" && fields[1] == "resume" {
		return fields[2]
	}
	return ""
}

func transcriptPathFromProcessFDs(pid int) string {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		if extractSessionIDFromTranscriptPath(target) != "" {
			return target
		}
	}
	return ""
}

func extractSessionIDFromTranscriptPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "rollout-") || !strings.HasSuffix(base, ".jsonl") {
		return ""
	}
	base = strings.TrimSuffix(base, ".jsonl")
	const sessionIDLen = 36
	if len(base) < sessionIDLen {
		return ""
	}
	sessionID := base[len(base)-sessionIDLen:]
	if strings.Count(sessionID, "-") != 4 {
		return ""
	}
	return sessionID
}

func findTranscriptPathBySessionID(sessionID string) string {
	if sessionID == "" {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, ".codex", "sessions")
	pattern := filepath.Join(root, "*", "*", "*", "*"+sessionID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	type candidate struct {
		path    string
		modTime int64
	}
	items := make([]candidate, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		items = append(items, candidate{
			path:    match,
			modTime: info.ModTime().UnixNano(),
		})
	}
	if len(items) == 0 {
		return ""
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].modTime != items[j].modTime {
			return items[i].modTime > items[j].modTime
		}
		return items[i].path > items[j].path
	})
	return items[0].path
}

func IsNotFound(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
