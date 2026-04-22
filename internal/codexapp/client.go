package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/vxider/codex-buddy/internal/config"
)

type Client struct {
	cfg    config.AppServerConfig
	logger *log.Logger

	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	startedAt    time.Time
	connected    bool
	initialized  bool
	currentPID   int
	threadID     string
	account      map[string]any
	requiresAuth bool

	nextID      int64
	pending     map[string]chan rpcMessage
	subscribers map[chan Event]struct{}
	recent      []Event
	closed      chan struct{}
}

type Event struct {
	Time   time.Time      `json:"time"`
	Kind   string         `json:"kind"`
	Method string         `json:"method,omitempty"`
	ID     int64          `json:"id,omitempty"`
	IDRaw  string         `json:"id_raw,omitempty"`
	Raw    map[string]any `json:"raw"`
}

type Status struct {
	Connected     bool           `json:"connected"`
	Initialized   bool           `json:"initialized"`
	PID           int            `json:"pid,omitempty"`
	StartedAt     time.Time      `json:"started_at,omitempty"`
	CurrentThread string         `json:"current_thread_id,omitempty"`
	Account       map[string]any `json:"account,omitempty"`
	RequiresAuth  bool           `json:"requires_openai_auth"`
	Recent        []Event        `json:"recent"`
}

type rpcMessage struct {
	ID     *json.RawMessage `json:"id,omitempty"`
	Method string           `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func New(cfg config.AppServerConfig, logger *log.Logger) *Client {
	return &Client{
		cfg:         cfg,
		logger:      logger,
		pending:     make(map[string]chan rpcMessage),
		subscribers: make(map[chan Event]struct{}),
		recent:      make([]Event, 0, 200),
		closed:      make(chan struct{}),
	}
}

func (c *Client) EnsureReady(ctx context.Context) error {
	c.mu.Lock()
	if c.connected && c.initialized {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if err := c.ensureProcess(); err != nil {
		return err
	}
	if err := c.initialize(ctx); err != nil {
		return err
	}
	return c.refreshAccount(ctx)
}

func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()

	recent := make([]Event, len(c.recent))
	copy(recent, c.recent)
	return Status{
		Connected:     c.connected,
		Initialized:   c.initialized,
		PID:           c.currentPID,
		StartedAt:     c.startedAt,
		CurrentThread: c.threadID,
		Account:       cloneMap(c.account),
		RequiresAuth:  c.requiresAuth,
		Recent:        recent,
	}
}

func (c *Client) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	c.mu.Lock()
	c.subscribers[ch] = struct{}{}
	c.mu.Unlock()

	cancel := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.subscribers[ch]; ok {
			delete(c.subscribers, ch)
			close(ch)
		}
	}
	return ch, cancel
}

func (c *Client) ThreadStart(ctx context.Context, modelName, cwd string) (map[string]any, error) {
	if err := c.EnsureReady(ctx); err != nil {
		return nil, err
	}

	params := map[string]any{
		"experimentalRawEvents":  false,
		"persistExtendedHistory": false,
	}
	if strings.TrimSpace(modelName) != "" {
		params["model"] = strings.TrimSpace(modelName)
	}
	if strings.TrimSpace(cwd) != "" {
		params["cwd"] = strings.TrimSpace(cwd)
	}

	msg, err := c.request(ctx, "thread/start", params)
	if err != nil {
		return nil, err
	}

	result, err := decodeRawMap(msg.Result)
	if err != nil {
		return nil, err
	}
	if thread, ok := nestedMap(result, "thread"); ok {
		if id, _ := thread["id"].(string); id != "" {
			c.mu.Lock()
			c.threadID = id
			c.mu.Unlock()
		}
	}
	return result, nil
}

func (c *Client) TurnStart(ctx context.Context, threadID, prompt, cwd, modelName string) (map[string]any, error) {
	if err := c.EnsureReady(ctx); err != nil {
		return nil, err
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		c.mu.Lock()
		threadID = c.threadID
		c.mu.Unlock()
	}
	if threadID == "" {
		return nil, fmt.Errorf("thread id is required")
	}

	params := map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{
				"type":          "text",
				"text":          prompt,
				"text_elements": []any{},
			},
		},
	}
	if strings.TrimSpace(cwd) != "" {
		params["cwd"] = strings.TrimSpace(cwd)
	}
	if strings.TrimSpace(modelName) != "" {
		params["model"] = strings.TrimSpace(modelName)
	}

	msg, err := c.request(ctx, "turn/start", params)
	if err != nil {
		return nil, err
	}
	return decodeRawMap(msg.Result)
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

func (c *Client) ensureProcess() error {
	c.mu.Lock()
	if c.connected && c.cmd != nil && c.cmd.Process != nil {
		c.mu.Unlock()
		return nil
	}
	if len(c.cfg.Command) == 0 {
		c.mu.Unlock()
		return fmt.Errorf("app-server command is empty")
	}

	cmd := exec.Command(c.cfg.Command[0], c.cfg.Command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("open stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("start codex app-server: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.connected = true
	c.initialized = false
	c.startedAt = time.Now().UTC()
	c.currentPID = cmd.Process.Pid
	c.pending = make(map[string]chan rpcMessage)

	go c.readLoop(stdout)
	go c.stderrLoop(stderr)
	go c.waitLoop(cmd)
	c.mu.Unlock()

	c.appendEvent(Event{
		Time: time.Now().UTC(),
		Kind: "lifecycle",
		Raw: map[string]any{
			"status": "started",
			"pid":    c.currentPID,
		},
	})
	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	c.mu.Lock()
	if c.initialized {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    c.cfg.ClientInfo.Name,
			"title":   c.cfg.ClientInfo.Title,
			"version": c.cfg.ClientInfo.Version,
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
			"optOutNotificationMethods": []string{
				"command/exec/outputDelta",
				"item/agentMessage/delta",
				"item/plan/delta",
				"item/fileChange/outputDelta",
				"item/reasoning/summaryTextDelta",
				"item/reasoning/textDelta",
			},
		},
	}

	c.logger.Printf("codex app-server: sending initialize")
	if _, err := c.request(ctx, "initialize", params); err != nil {
		if !strings.Contains(err.Error(), "Already initialized") {
			return err
		}
	}
	c.logger.Printf("codex app-server: sending initialized")
	if err := c.notify("initialized", map[string]any{}); err != nil {
		return err
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()
	return nil
}

func (c *Client) refreshAccount(ctx context.Context) error {
	c.logger.Printf("codex app-server: requesting account/read")
	msg, err := c.request(ctx, "account/read", map[string]any{"refreshToken": false})
	if err != nil {
		return err
	}
	result, err := decodeRawMap(msg.Result)
	if err != nil {
		return err
	}

	c.mu.Lock()
	if account, ok := nestedMap(result, "account"); ok {
		c.account = account
	} else {
		c.account = nil
	}
	if requires, ok := result["requiresOpenaiAuth"].(bool); ok {
		c.requiresAuth = requires
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) request(ctx context.Context, method string, params any) (rpcMessage, error) {
	waitCh := make(chan rpcMessage, 1)

	c.mu.Lock()
	if !c.connected || c.stdin == nil {
		c.mu.Unlock()
		return rpcMessage{}, fmt.Errorf("app-server is not connected")
	}
	c.nextID++
	id := fmt.Sprintf("%d", c.nextID)
	c.pending[id] = waitCh
	payload := map[string]any{
		"method": method,
		"id":     id,
		"params": params,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return rpcMessage{}, fmt.Errorf("marshal request: %w", err)
	}
	if err := writeFrame(c.stdin, data); err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return rpcMessage{}, fmt.Errorf("write request: %w", err)
	}
	c.mu.Unlock()
	c.logger.Printf("codex app-server: request sent method=%s id=%s", method, id)

	timeout := time.Duration(c.cfg.RequestTimeoutMS) * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	select {
	case msg, ok := <-waitCh:
		if !ok {
			return rpcMessage{}, fmt.Errorf("%s closed while waiting for response", method)
		}
		c.logger.Printf("codex app-server: response received method=%s id=%s", method, id)
		if msg.Error != nil {
			return rpcMessage{}, fmt.Errorf("%s failed: (%d) %s", method, msg.Error.Code, msg.Error.Message)
		}
		return msg, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return rpcMessage{}, fmt.Errorf("%s timed out", method)
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return rpcMessage{}, ctx.Err()
	}
}

func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected || c.stdin == nil {
		return fmt.Errorf("app-server is not connected")
	}
	payload := map[string]any{
		"method": method,
		"params": params,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notify: %w", err)
	}
	if err := writeFrame(c.stdin, data); err != nil {
		return fmt.Errorf("write notify: %w", err)
	}
	return nil
}

func (c *Client) readLoop(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		frame, err := readFrame(reader)
		if err != nil {
			if err != io.EOF {
				c.logger.Printf("app-server stdout closed with error: %v", err)
			}
			return
		}
		if len(frame) == 0 {
			continue
		}
		msg, raw, err := parseRPC(frame)
		if err != nil {
			c.logger.Printf("parse app-server line failed: %v", err)
			continue
		}
		c.handleMessage(msg, raw)
	}
}

func (c *Client) stderrLoop(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		text := scanner.Text()
		c.appendEvent(Event{
			Time: time.Now().UTC(),
			Kind: "stderr",
			Raw: map[string]any{
				"line": text,
			},
		})
	}
}

func (c *Client) waitLoop(cmd *exec.Cmd) {
	err := cmd.Wait()
	c.mu.Lock()
	c.connected = false
	c.initialized = false
	c.stdin = nil
	c.currentPID = 0
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()

	payload := map[string]any{"status": "stopped"}
	if err != nil {
		payload["error"] = err.Error()
	}
	c.appendEvent(Event{
		Time: time.Now().UTC(),
		Kind: "lifecycle",
		Raw:  payload,
	})
}

func (c *Client) handleMessage(msg rpcMessage, raw map[string]any) {
	now := time.Now().UTC()
	if msg.Method == "" && msg.ID != nil {
		id := parseIDString(msg.ID)
		c.logger.Printf("codex app-server: incoming response id=%s", id)
		c.mu.Lock()
		waitCh, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		if ok {
			waitCh <- msg
			close(waitCh)
		}
		c.appendEvent(Event{
			Time:  now,
			Kind:  "response",
			ID:    parseIDNumber(msg.ID),
			IDRaw: id,
			Raw:   raw,
		})
		return
	}

	kind := "notification"
	if msg.Method != "" && msg.ID != nil {
		kind = "server_request"
	}
	event := Event{
		Time:   now,
		Kind:   kind,
		Method: msg.Method,
		ID:     parseIDNumber(msg.ID),
		IDRaw:  parseIDString(msg.ID),
		Raw:    raw,
	}
	c.logger.Printf("codex app-server: incoming %s method=%s", kind, msg.Method)
	c.applyMessageSideEffects(msg, raw)
	c.appendEvent(event)
}

func (c *Client) applyMessageSideEffects(msg rpcMessage, raw map[string]any) {
	switch msg.Method {
	case "thread/started":
		if params, ok := nestedMap(raw, "params"); ok {
			if thread, ok := nestedMap(params, "thread"); ok {
				if id, _ := thread["id"].(string); id != "" {
					c.mu.Lock()
					c.threadID = id
					c.mu.Unlock()
				}
			}
		}
	case "account/updated":
		if params, ok := nestedMap(raw, "params"); ok {
			if account, ok := nestedMap(params, "account"); ok {
				c.mu.Lock()
				c.account = account
				c.mu.Unlock()
			}
			if requires, ok := params["requiresOpenaiAuth"].(bool); ok {
				c.mu.Lock()
				c.requiresAuth = requires
				c.mu.Unlock()
			}
		}
	}
}

func (c *Client) appendEvent(event Event) {
	c.mu.Lock()
	c.recent = append(c.recent, event)
	if len(c.recent) > 200 {
		c.recent = c.recent[len(c.recent)-200:]
	}
	subs := make([]chan Event, 0, len(c.subscribers))
	for ch := range c.subscribers {
		subs = append(subs, ch)
	}
	c.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func parseRPC(line []byte) (rpcMessage, map[string]any, error) {
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return rpcMessage{}, nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return rpcMessage{}, nil, err
	}
	return msg, raw, nil
}

func parseIDNumber(raw *json.RawMessage) int64 {
	if raw == nil {
		return 0
	}
	var id int64
	if err := json.Unmarshal(*raw, &id); err == nil {
		return id
	}
	return 0
}

func parseIDString(raw *json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var id string
	if err := json.Unmarshal(*raw, &id); err == nil {
		return id
	}
	var number int64
	if err := json.Unmarshal(*raw, &number); err == nil {
		return fmt.Sprintf("%d", number)
	}
	return ""
}

func decodeRawMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func nestedMap(root map[string]any, key string) (map[string]any, bool) {
	value, ok := root[key]
	if !ok {
		return nil, false
	}
	typed, ok := value.(map[string]any)
	return typed, ok
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func writeFrame(w io.Writer, data []byte) error {
	_, err := w.Write(append(data, '\n'))
	return err
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimSpace(string(line))), nil
}
