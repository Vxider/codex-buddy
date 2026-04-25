//go:build uconsole_gui

package uconsole

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type tailscaleState struct {
	Installed    bool
	Online       bool
	BackendState string
	Error        string
	ExitNodeName string
	ExitNodeIP   string
	ExitNodeID   string
	ExitNodes    []tailscaleExitNode
}

type tailscaleExitNode struct {
	ID      string
	Name    string
	IP      string
	Online  bool
	Current bool
}

func loadTailscaleState(ctx context.Context) tailscaleState {
	if _, err := exec.LookPath("tailscale"); err != nil {
		return tailscaleState{}
	}

	state := tailscaleState{Installed: true}
	statusRaw, err := runTailscaleCommand(ctx, "status", "--json")
	if err != nil {
		state.Error = err.Error()
		return state
	}

	var statusPayload map[string]any
	if err := json.Unmarshal(statusRaw, &statusPayload); err != nil {
		state.Error = fmt.Sprintf("tailscale status parse failed: %v", err)
		return state
	}

	state.BackendState = strings.TrimSpace(asString(statusPayload["BackendState"]))
	self := asMap(statusPayload["Self"])
	state.Online = asBool(self["Online"])

	for peerID, rawPeer := range asMap(statusPayload["Peer"]) {
		peer := asMap(rawPeer)
		if !asBool(peer["ExitNodeOption"]) {
			continue
		}
		state.ExitNodes = append(state.ExitNodes, tailscaleExitNode{
			ID:     firstNonEmptyText(asString(peer["ID"]), asString(peer["StableID"]), asString(peer["NodeID"]), peerID),
			Name:   preferredTailscaleNodeName(peer),
			IP:     firstString(asSlice(peer["TailscaleIPs"])),
			Online: asBool(peer["Online"]),
		})
	}

	prefsRaw, err := runTailscaleCommand(ctx, "debug", "prefs")
	if err == nil {
		var prefsPayload map[string]any
		if json.Unmarshal(prefsRaw, &prefsPayload) == nil {
			state.ExitNodeID = strings.TrimSpace(asString(prefsPayload["ExitNodeID"]))
			state.ExitNodeIP = strings.TrimSpace(asString(prefsPayload["ExitNodeIP"]))
		}
	}

	for i := range state.ExitNodes {
		node := &state.ExitNodes[i]
		if node.ID != "" && node.ID == state.ExitNodeID {
			node.Current = true
		}
		if !node.Current && node.IP != "" && node.IP == state.ExitNodeIP {
			node.Current = true
		}
		if node.Current {
			state.ExitNodeName = node.Name
		}
	}

	if state.ExitNodeName == "" {
		state.ExitNodeName = firstNonEmptyText(state.ExitNodeIP, state.ExitNodeID)
	}

	sort.SliceStable(state.ExitNodes, func(i, j int) bool {
		if state.ExitNodes[i].Current != state.ExitNodes[j].Current {
			return state.ExitNodes[i].Current
		}
		if state.ExitNodes[i].Online != state.ExitNodes[j].Online {
			return state.ExitNodes[i].Online
		}
		return state.ExitNodes[i].Name < state.ExitNodes[j].Name
	})

	return state
}

func setTailscaleExitNode(ctx context.Context, ip string) error {
	if _, err := exec.LookPath("tailscale"); err != nil {
		return fmt.Errorf("tailscale cli not found")
	}

	arg := "--exit-node="
	if trimmed := strings.TrimSpace(ip); trimmed != "" {
		arg = "--exit-node=" + trimmed
	}

	_, err := runTailscaleCommand(ctx, "set", arg)
	return err
}

func runTailscaleCommand(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("tailscale %s: %s", strings.Join(args, " "), message)
	}
	return stdout.Bytes(), nil
}

func preferredTailscaleNodeName(node map[string]any) string {
	dnsName := strings.TrimSuffix(strings.TrimSpace(asString(node["DNSName"])), ".")
	if dnsName != "" {
		parts := strings.Split(dnsName, ".")
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
		return dnsName
	}
	return firstNonEmptyText(
		strings.TrimSpace(asString(node["HostName"])),
		strings.TrimSpace(asString(node["Name"])),
		strings.TrimSpace(asString(node["ComputedName"])),
		"unknown",
	)
}

func asMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func asSlice(value any) []any {
	result, _ := value.([]any)
	return result
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func asBool(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

func firstString(values []any) string {
	for _, value := range values {
		text := strings.TrimSpace(asString(value))
		if text != "" {
			return text
		}
	}
	return ""
}
