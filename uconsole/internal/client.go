package uconsole

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

type Client struct {
	baseURL string
	http    *http.Client
	stream  *http.Client
	logger  *log.Logger
}

func NewClient(baseURL string, timeout time.Duration, logger *log.Logger) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
		stream:  &http.Client{},
		logger:  logger,
	}
}

func (c *Client) LoadStatus(ctx context.Context) (StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/status", nil)
	if err != nil {
		return StatusResponse{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return StatusResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return StatusResponse{}, fmt.Errorf("status request failed: %s", resp.Status)
	}
	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return StatusResponse{}, err
	}
	return status, nil
}

func (c *Client) LoadNotifications(ctx context.Context) ([]NotificationResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/notifications", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notifications request failed: %s", resp.Status)
	}
	var out NotificationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Notifications, nil
}

func (c *Client) AckNotification(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/notifications/"+url.PathEscape(id)+"/ack", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ack failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) ContinueNotification(ctx context.Context, item NotificationResponse) error {
	payload, err := json.Marshal(map[string]string{
		"action":       string(model.NotificationActionContinue),
		"action_token": item.ActionToken,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/notifications/"+url.PathEscape(item.ID)+"/action", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("continue failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) ContinueSession(ctx context.Context, session SessionResponse) error {
	action := session.ContinueAction
	if action == nil {
		return fmt.Errorf("session continue is unavailable")
	}

	endpoint := strings.TrimSpace(action.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("session continue endpoint is missing")
	}
	method := strings.ToUpper(strings.TrimSpace(action.Method))
	if method == "" {
		method = http.MethodPost
	}

	targetURL, err := c.resolveURL(endpoint)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]string{
		"action_token": strings.TrimSpace(action.ActionToken),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("session continue failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) resolveURL(endpoint string) (string, error) {
	base, err := url.Parse(c.baseURL + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func (c *Client) StreamStatus(ctx context.Context, onStatus func(StatusResponse)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/stream", nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream failed: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)

	var event string
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if event == "status" && data.Len() > 0 {
				var status StatusResponse
				if err := json.Unmarshal([]byte(data.String()), &status); err == nil {
					onStatus(status)
				} else if c.logger != nil {
					c.logger.Printf("uconsole: parse stream payload failed: %v", err)
				}
			}
			event = ""
			data.Reset()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return context.Cause(ctx)
}
