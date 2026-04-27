package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

type UpdateHandler func(model.TranscriptUpdate)

type Manager struct {
	mu           sync.Mutex
	enabled      bool
	tailFromEnd  bool
	pollInterval time.Duration
	watchers     map[string]*watcher
	logger       *log.Logger
	onUpdate     UpdateHandler
}

type watcher struct {
	sessionID string
	path      string
	offset    int64
	partial   []byte
	cancel    chan struct{}
}

func NewManager(enabled, tailFromEnd bool, pollInterval time.Duration, logger *log.Logger, onUpdate UpdateHandler) *Manager {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &Manager{
		enabled:      enabled,
		tailFromEnd:  tailFromEnd,
		pollInterval: pollInterval,
		watchers:     make(map[string]*watcher),
		logger:       logger,
		onUpdate:     onUpdate,
	}
}

func (m *Manager) Ensure(sessionID, path string) {
	if !m.enabled || sessionID == "" || path == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.watchers[sessionID]; ok {
		if existing.path == path {
			return
		}
		close(existing.cancel)
		delete(m.watchers, sessionID)
	}

	w := &watcher{
		sessionID: sessionID,
		path:      path,
		cancel:    make(chan struct{}),
	}

	if m.tailFromEnd {
		if info, err := os.Stat(path); err == nil {
			w.offset = info.Size()
		}
	}

	m.watchers[sessionID] = w
	m.logger.Printf("registered transcript watcher session=%s path=%s", sessionID, path)
	go m.run(w)
}

func RecoverSession(path, sessionID string) (model.TranscriptUpdate, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.TranscriptUpdate{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	var out model.TranscriptUpdate
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		update, ok := parseLine(sessionID, line)
		if !ok {
			continue
		}
		mergeUpdate(&out, update)
	}
	if err := scanner.Err(); err != nil {
		return model.TranscriptUpdate{}, err
	}

	if out.SessionID == "" {
		out.SessionID = sessionID
	}
	return out, nil
}

func (m *Manager) Snapshot() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[string]string, len(m.watchers))
	for key, value := range m.watchers {
		out[key] = value.path
	}
	return out
}

func (m *Manager) Remove(sessionID string) {
	if sessionID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.watchers[sessionID]
	if !ok {
		return
	}
	close(existing.cancel)
	delete(m.watchers, sessionID)
}

func (m *Manager) run(w *watcher) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.cancel:
			return
		case <-ticker.C:
			if err := m.poll(w); err != nil {
				m.logger.Printf("transcript poll failed session=%s path=%s err=%v", w.sessionID, w.path, err)
			}
		}
	}
}

func (m *Manager) poll(w *watcher) error {
	file, err := os.Open(w.path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < w.offset {
		w.offset = 0
		w.partial = nil
	}
	if info.Size() == w.offset {
		return nil
	}

	if _, err := file.Seek(w.offset, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	chunk, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(chunk) == 0 {
		return nil
	}

	w.offset += int64(len(chunk))
	buffer := append(w.partial, chunk...)
	lines := bytes.Split(buffer, []byte{'\n'})
	if len(buffer) > 0 && buffer[len(buffer)-1] != '\n' {
		w.partial = append([]byte(nil), lines[len(lines)-1]...)
		lines = lines[:len(lines)-1]
	} else {
		w.partial = nil
	}

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		update, ok := parseLine(w.sessionID, line)
		if ok && m.onUpdate != nil {
			m.onUpdate(update)
		}
	}

	return nil
}

type envelope struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type responseItemPayload struct {
	Type    string            `json:"type"`
	Role    string            `json:"role"`
	Name    string            `json:"name"`
	Args    string            `json:"arguments"`
	Content []contentFragment `json:"content"`
}

type contentFragment struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type eventPayload struct {
	Type             string   `json:"type"`
	Message          string   `json:"message"`
	TurnID           string   `json:"turn_id"`
	CWD              string   `json:"cwd"`
	Model            string   `json:"model"`
	Command          []string `json:"command"`
	AggregatedOutput string   `json:"aggregated_output"`
	Status           string   `json:"status"`
}

func parseLine(sessionID string, line []byte) (model.TranscriptUpdate, bool) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return model.TranscriptUpdate{}, false
	}

	parsedTime := time.Now().UTC()
	if env.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339Nano, env.Timestamp); err == nil {
			parsedTime = ts
		}
	}

	update := model.TranscriptUpdate{
		SessionID: sessionID,
		UpdatedAt: parsedTime,
	}

	switch env.Type {
	case "turn_context":
		var payload eventPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return model.TranscriptUpdate{}, false
		}
		update.TurnID = payload.TurnID
		update.CWD = payload.CWD
		update.Model = payload.Model
		return update, hasUsefulTranscriptUpdate(update)
	case "response_item":
		var payload responseItemPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return model.TranscriptUpdate{}, false
		}
		if payload.Type == "message" {
			text := joinContent(payload.Content)
			switch payload.Role {
			case "user":
				update.LastUserPromptPreview = text
			case "assistant":
				update.LastAssistantMessage = text
			}
		}
		if payload.Type == "function_call" && payload.Name == "shell_command" {
			update.LastBashCommand = parseShellCommandArguments(payload.Args)
		}
		return update, hasUsefulTranscriptUpdate(update)
	case "event_msg":
		var payload eventPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return model.TranscriptUpdate{}, false
		}
		switch payload.Type {
		case "user_message":
			update.LastUserPromptPreview = payload.Message
		case "agent_message":
			update.LastAssistantMessage = payload.Message
			update.TurnID = payload.TurnID
		case "exec_command_end":
			update.TurnID = payload.TurnID
			update.LastBashCommand = strings.Join(payload.Command, " ")
			if strings.EqualFold(payload.Status, "failed") && strings.TrimSpace(payload.AggregatedOutput) != "" {
				update.Error = payload.AggregatedOutput
			}
		}
		return update, hasUsefulTranscriptUpdate(update)
	default:
		return model.TranscriptUpdate{}, false
	}
}

func mergeUpdate(dst *model.TranscriptUpdate, src model.TranscriptUpdate) {
	if dst == nil {
		return
	}
	if dst.SessionID == "" {
		dst.SessionID = src.SessionID
	}
	if src.TurnID != "" {
		dst.TurnID = src.TurnID
	}
	if src.CWD != "" {
		dst.CWD = src.CWD
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.LastUserPromptPreview != "" {
		dst.LastUserPromptPreview = src.LastUserPromptPreview
	}
	if src.LastAssistantMessage != "" {
		dst.LastAssistantMessage = src.LastAssistantMessage
	}
	if src.LastBashCommand != "" {
		dst.LastBashCommand = src.LastBashCommand
	}
	if src.Error != "" {
		dst.Error = src.Error
	} else if hasNonErrorProgress(src) {
		dst.Error = ""
	}
	if src.UpdatedAt.After(dst.UpdatedAt) {
		dst.UpdatedAt = src.UpdatedAt
	}
}

func hasNonErrorProgress(update model.TranscriptUpdate) bool {
	return update.TurnID != "" ||
		update.CWD != "" ||
		update.Model != "" ||
		update.LastUserPromptPreview != "" ||
		update.LastAssistantMessage != "" ||
		update.LastBashCommand != ""
}

func joinContent(parts []contentFragment) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		items = append(items, strings.TrimSpace(part.Text))
	}
	return strings.Join(items, "\n")
}

func hasUsefulTranscriptUpdate(update model.TranscriptUpdate) bool {
	return update.TurnID != "" ||
		update.CWD != "" ||
		update.Model != "" ||
		update.LastUserPromptPreview != "" ||
		update.LastAssistantMessage != "" ||
		update.LastBashCommand != "" ||
		update.Error != ""
}

func parseShellCommandArguments(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Command)
}
