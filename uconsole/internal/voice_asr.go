//go:build uconsole_gui

package uconsole

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type voiceCapture struct {
	cfg       voiceASRConfig
	cmd       *exec.Cmd
	audioFile string
	startedAt time.Time
	session   SessionResponse
}

type voiceASRConfig struct {
	WhisperURL              string
	WhisperModel            string
	WhisperLanguage         string
	WhisperAuthToken        string
	WhisperPrompt           string
	WhisperPromptField      string
	WhisperGlossaryField    string
	WhisperContextField     string
	WhisperEnableCorrection bool
	WhisperNoProxy          bool
	VoiceRecorder           string
	VoiceInput              string
	VoiceMinRecord          time.Duration
	VoiceSampleRate         int
	VoiceChannels           int
	VoiceGlossaryFile       string
	VoiceTmuxContext        bool
	VoiceTmuxContextLines   int
	VoiceTmuxContextMaxChar int
}

func loadVoiceASRConfig() voiceASRConfig {
	values := currentEnvMap()
	for key, value := range loadSimpleEnvFile(voiceConfigPath()) {
		values[key] = value
	}

	cfg := voiceASRConfig{
		WhisperURL:              strings.TrimSpace(values["WHISPER_URL"]),
		WhisperModel:            strings.TrimSpace(values["WHISPER_MODEL"]),
		WhisperLanguage:         strings.TrimSpace(values["WHISPER_LANGUAGE"]),
		WhisperAuthToken:        strings.TrimSpace(values["WHISPER_AUTH_TOKEN"]),
		WhisperPrompt:           strings.TrimSpace(values["WHISPER_PROMPT"]),
		WhisperPromptField:      firstNonEmptyText(values["WHISPER_PROMPT_FIELD"], "prompt"),
		WhisperGlossaryField:    firstNonEmptyText(values["WHISPER_PROMPT_GLOSSARY_FIELD"], "promptGlossary"),
		WhisperContextField:     firstNonEmptyText(values["WHISPER_CONTEXT_FIELD"], "contextText"),
		WhisperEnableCorrection: envBool(values["WHISPER_ENABLE_CORRECTION"], true),
		WhisperNoProxy:          envBool(values["WHISPER_NO_PROXY"], true),
		VoiceRecorder:           firstNonEmptyText(values["VOICE_RECORDER"], "auto"),
		VoiceInput:              firstNonEmptyText(values["VOICE_INPUT"], "default"),
		VoiceMinRecord:          time.Duration(envInt(values["VOICE_MIN_RECORD_MS"], 350)) * time.Millisecond,
		VoiceSampleRate:         envInt(values["VOICE_SAMPLE_RATE"], 16000),
		VoiceChannels:           envInt(values["VOICE_CHANNELS"], 1),
		VoiceGlossaryFile:       expandHome(firstNonEmptyText(values["VOICE_GLOSSARY_FILE"], "~/.config/uconsole-mapper/voice-glossary.txt")),
		VoiceTmuxContext:        envBool(values["VOICE_TMUX_CONTEXT"], true),
		VoiceTmuxContextLines:   envInt(values["VOICE_TMUX_CONTEXT_LINES"], 30),
		VoiceTmuxContextMaxChar: envInt(values["VOICE_TMUX_CONTEXT_MAX_CHARS"], 1200),
	}
	if cfg.VoiceMinRecord <= 0 {
		cfg.VoiceMinRecord = 350 * time.Millisecond
	}
	if cfg.VoiceSampleRate <= 0 {
		cfg.VoiceSampleRate = 16000
	}
	if cfg.VoiceChannels <= 0 {
		cfg.VoiceChannels = 1
	}
	if cfg.VoiceTmuxContextLines <= 0 {
		cfg.VoiceTmuxContextLines = 30
	}
	if cfg.VoiceTmuxContextMaxChar <= 0 {
		cfg.VoiceTmuxContextMaxChar = 1200
	}
	return cfg
}

func startVoiceCapture(session SessionResponse) (*voiceCapture, error) {
	cfg := loadVoiceASRConfig()
	recorder, err := chooseVoiceRecorder(cfg.VoiceRecorder)
	if err != nil {
		return nil, err
	}

	audioFile, err := newVoiceAudioTemp()
	if err != nil {
		return nil, err
	}

	cmd, err := buildVoiceRecorderCommand(recorder, cfg, audioFile)
	if err != nil {
		_ = os.Remove(audioFile)
		return nil, err
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = os.Remove(audioFile)
		return nil, fmt.Errorf("%s start failed: %w", recorder, err)
	}

	return &voiceCapture{
		cfg:       cfg,
		cmd:       cmd,
		audioFile: audioFile,
		startedAt: time.Now(),
		session:   session,
	}, nil
}

func (c *voiceCapture) StopAndTranscribe(ctx context.Context) (string, error) {
	if c == nil {
		return "", errors.New("voice capture is unavailable")
	}
	defer func() {
		_ = os.Remove(c.audioFile)
	}()

	if err := stopVoiceRecorder(c.cmd); err != nil {
		return "", err
	}
	if time.Since(c.startedAt) < c.cfg.VoiceMinRecord {
		return "", errors.New("recording was too short")
	}
	info, err := os.Stat(c.audioFile)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", errors.New("recorded audio is empty")
	}
	if strings.TrimSpace(c.cfg.WhisperURL) == "" {
		return "", errors.New("WHISPER_URL is not configured")
	}

	text, err := transcribeVoiceCapture(ctx, c.cfg, c.session, c.audioFile)
	if err != nil {
		return "", err
	}
	text = normalizeTranscriptText(text)
	if text == "" {
		return "", errors.New("no transcript text was returned")
	}
	return text, nil
}

func transcribeVoiceCapture(ctx context.Context, cfg voiceASRConfig, session SessionResponse, audioFile string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	fileWriter, err := writer.CreateFormFile("file", filepath.Base(audioFile))
	if err != nil {
		return "", err
	}
	audio, err := os.Open(audioFile)
	if err != nil {
		return "", err
	}
	defer audio.Close()
	if _, err := io.Copy(fileWriter, audio); err != nil {
		return "", err
	}

	if cfg.WhisperModel != "" {
		_ = writer.WriteField("model", cfg.WhisperModel)
	}
	if cfg.WhisperLanguage != "" {
		_ = writer.WriteField("language", cfg.WhisperLanguage)
	}
	if cfg.WhisperEnableCorrection {
		_ = writer.WriteField("enableCorrection", "true")
	}
	if cfg.WhisperPrompt != "" {
		_ = writer.WriteField(cfg.WhisperPromptField, cfg.WhisperPrompt)
	}
	if glossary := loadVoiceGlossaryJSON(cfg.VoiceGlossaryFile); glossary != "" {
		_ = writer.WriteField(cfg.WhisperGlossaryField, glossary)
	}
	if contextText := buildVoiceContext(cfg, session); contextText != "" {
		_ = writer.WriteField(cfg.WhisperContextField, contextText)
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WhisperURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cfg.WhisperAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.WhisperAuthToken)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.WhisperNoProxy {
		transport.Proxy = nil
	}
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("whisper request failed: %s: %s", resp.Status, strings.TrimSpace(string(bodyBytes)))
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return extractTranscriptText(payload), nil
}

func chooseVoiceRecorder(configured string) (string, error) {
	switch configured {
	case "", "auto":
		for _, name := range []string{"pw-record", "ffmpeg", "arecord"} {
			if _, err := exec.LookPath(name); err == nil {
				return name, nil
			}
		}
		return "", errors.New("no supported recorder found; install pw-record, ffmpeg, or arecord")
	case "pw-record", "ffmpeg", "arecord":
		if _, err := exec.LookPath(configured); err != nil {
			return "", fmt.Errorf("configured recorder not found: %s", configured)
		}
		return configured, nil
	default:
		return "", fmt.Errorf("unsupported recorder: %s", configured)
	}
}

func buildVoiceRecorderCommand(recorder string, cfg voiceASRConfig, audioFile string) (*exec.Cmd, error) {
	switch recorder {
	case "pw-record":
		return exec.Command("pw-record",
			"--rate", strconv.Itoa(cfg.VoiceSampleRate),
			"--channels", strconv.Itoa(cfg.VoiceChannels),
			audioFile,
		), nil
	case "ffmpeg":
		return exec.Command("ffmpeg",
			"-hide_banner",
			"-loglevel", "error",
			"-y",
			"-f", "pulse",
			"-i", cfg.VoiceInput,
			"-ac", strconv.Itoa(cfg.VoiceChannels),
			"-ar", strconv.Itoa(cfg.VoiceSampleRate),
			audioFile,
		), nil
	case "arecord":
		return exec.Command("arecord",
			"-q",
			"-f", "S16_LE",
			"-r", strconv.Itoa(cfg.VoiceSampleRate),
			"-c", strconv.Itoa(cfg.VoiceChannels),
			audioFile,
		), nil
	default:
		return nil, fmt.Errorf("unsupported recorder: %s", recorder)
	}
}

func stopVoiceRecorder(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil && !isExpectedVoiceStopError(err) {
			return err
		}
		return nil
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		err := <-done
		if err != nil && !isExpectedVoiceStopError(err) {
			return err
		}
		return nil
	}
}

func isExpectedVoiceStopError(err error) bool {
	if err == nil {
		return true
	}
	text := err.Error()
	return strings.Contains(text, "signal: interrupt") || strings.Contains(text, "signal: killed")
}

func newVoiceAudioTemp() (string, error) {
	dir := os.TempDir()
	file, err := os.CreateTemp(dir, "uconsole-voice-*.wav")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func buildVoiceContext(cfg voiceASRConfig, session SessionResponse) string {
	if !cfg.VoiceTmuxContext {
		return ""
	}
	paneID := strings.TrimSpace(session.TmuxPane)
	if paneID == "" {
		return ""
	}

	fullText, _ := runCommandOutput("tmux", "capture-pane", "-p", "-S", fmt.Sprintf("-%d", cfg.VoiceTmuxContextLines), "-t", paneID)
	if strings.TrimSpace(fullText) == "" {
		fullText, _ = runCommandOutput("tmux", "capture-pane", "-p", "-t", paneID)
	}
	fullText = strings.TrimSpace(strings.ReplaceAll(fullText, "\r", ""))
	if fullText == "" {
		return ""
	}

	context := strings.Builder{}
	context.WriteString("session: ")
	context.WriteString(sessionListTitle(session))
	context.WriteString("\n")
	if session.TmuxSession != "" {
		context.WriteString("tmux session: ")
		context.WriteString(strings.TrimSpace(session.TmuxSession))
		context.WriteString("\n")
	}
	context.WriteString("[active pane ")
	context.WriteString(paneID)
	context.WriteString("]\n")
	context.WriteString(fullText)
	text := context.String()
	if len(text) > cfg.VoiceTmuxContextMaxChar {
		text = text[len(text)-cfg.VoiceTmuxContextMaxChar:]
	}
	return text
}

func runCommandOutput(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func loadVoiceGlossaryJSON(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	terms := make([]string, 0, 16)
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || seen[line] {
			continue
		}
		seen[line] = true
		terms = append(terms, line)
	}
	if len(terms) == 0 {
		return ""
	}
	payload, err := json.Marshal(map[string]any{"terms": terms})
	if err != nil {
		return ""
	}
	return string(payload)
}

func extractTranscriptText(payload any) string {
	switch value := payload.(type) {
	case map[string]any:
		if text, ok := value["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
		if nested, ok := value["result"]; ok {
			if text := extractTranscriptText(nested); text != "" {
				return text
			}
		}
		if nested, ok := value["data"]; ok {
			if text := extractTranscriptText(nested); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range value {
			if text := extractTranscriptText(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func normalizeTranscriptText(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")), " ")
}

func currentEnvMap() map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[parts[0]] = parts[1]
	}
	return values
}

func loadSimpleEnvFile(path string) map[string]string {
	values := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values
}

func voiceConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("VOICE_PTT_CONFIG")); path != "" {
		return expandHome(path)
	}
	return expandHome("~/.config/uconsole-mapper/voice.env")
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func envBool(value string, fallback bool) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return fallback
	}
	switch trimmed {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(value string, fallback int) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return fallback
	}
	return parsed
}
