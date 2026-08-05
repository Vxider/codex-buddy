package esp32sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vxider/agent-buddy/internal/model"
)

type Client struct {
	baseURL string
	http    *http.Client
	stream  *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
		stream:  &http.Client{},
	}
}

func (c *Client) LoadStatus(ctx context.Context) (Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/status", nil)
	if err != nil {
		return Status{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("status request failed: %s", resp.Status)
	}
	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (c *Client) StreamStatus(ctx context.Context, onStatus func(Status)) error {
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
				var status Status
				if err := json.Unmarshal([]byte(data.String()), &status); err == nil {
					onStatus(status)
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

func RunBridge(ctx context.Context, client *Client, publisher Publisher) error {
	status, err := client.LoadStatus(ctx)
	if err == nil {
		if err := publisher.Publish(ctx, FrameFromStatus(status)); err != nil {
			return err
		}
	} else {
		_ = publisher.Publish(ctx, Frame{
			State:       model.StateError,
			StateDetail: "network_error",
			LED:         "error",
		})
	}

	streamErr := client.StreamStatus(ctx, func(status Status) {
		_ = publisher.Publish(ctx, FrameFromStatus(status))
	})
	if ctx.Err() != nil {
		return nil
	}
	_ = publisher.Publish(ctx, Frame{
		State:       model.StateError,
		StateDetail: "network_error",
		LED:         "error",
	})
	return streamErr
}
