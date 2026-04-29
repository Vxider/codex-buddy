package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type Config struct {
	Listen        ListenConfig         `json:"listen"`
	Internal      InternalConfig       `json:"internal"`
	State         StateConfig          `json:"state"`
	Transcript    TranscriptConfig     `json:"transcript"`
	HookClient    HookClientConfig     `json:"hook_client"`
	AppServer     AppServerConfig      `json:"app_server"`
	LocalServer   LocalServerConfig    `json:"local_server"`
	RemoteServers []RemoteServerConfig `json:"remote_servers,omitempty"`
	UConsole      UConsoleConfig       `json:"uconsole"`
}

type ListenConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type InternalConfig struct {
	RequireLoopback bool `json:"require_loopback"`
}

type StateConfig struct {
	AttentionHoldMS int `json:"attention_hold_ms"`
	IdleFallbackMS  int `json:"idle_fallback_ms"`
}

type TranscriptConfig struct {
	Enabled        bool `json:"enabled"`
	TailFromEnd    bool `json:"tail_from_end"`
	PollIntervalMS int  `json:"poll_interval_ms"`
}

type HookClientConfig struct {
	RequestTimeoutMS int      `json:"request_timeout_ms"`
	AutostartEnabled bool     `json:"autostart_enabled"`
	AutostartCommand []string `json:"autostart_command"`
}

type AppServerConfig struct {
	Command          []string         `json:"command"`
	RequestTimeoutMS int              `json:"request_timeout_ms"`
	ClientInfo       ClientInfoConfig `json:"client_info"`
}

type LocalServerConfig struct {
	Enabled bool `json:"enabled"`
}

type RemoteServerConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
}

type ClientInfoConfig struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type UConsoleConfig struct {
	ServerURL        string               `json:"server_url"`
	HTTPTimeoutMS    int                  `json:"http_timeout_ms"`
	ReconnectDelayMS int                  `json:"reconnect_delay_ms"`
	PollFallbackMS   int                  `json:"poll_fallback_ms"`
	ContinueHoldMS   int                  `json:"continue_hold_ms"`
	Window           UConsoleWindowConfig `json:"window"`
	LED              UConsoleLEDConfig    `json:"led"`
}

type UConsoleWindowConfig struct {
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Fullscreen bool `json:"fullscreen"`
}

type UConsoleLEDConfig struct {
	Enabled    bool `json:"enabled"`
	Pixels     int  `json:"pixels"`
	Brightness int  `json:"brightness"`
	GPIOPin    int  `json:"gpio_pin"`
	DmaNum     int  `json:"dma_num"`
	Frequency  int  `json:"frequency"`
}

func Load(path string) (Config, error) {
	cfg := Default()

	resolved, err := ResolvePath(path)
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	applyDefaults(&cfg)
	return cfg, nil
}

func Save(path string, cfg Config) error {
	resolved, err := ResolvePath(path)
	if err != nil {
		return err
	}

	applyDefaults(&cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(resolved, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func ResolvePath(path string) (string, error) {
	if path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".config", "codex-buddy", "config.json"), nil
}

func Default() Config {
	return Config{
		Listen: ListenConfig{
			Host: "127.0.0.1",
			Port: 8787,
		},
		Internal: InternalConfig{
			RequireLoopback: true,
		},
		State: StateConfig{
			AttentionHoldMS: 0,
			IdleFallbackMS:  0,
		},
		Transcript: TranscriptConfig{
			Enabled:        true,
			TailFromEnd:    true,
			PollIntervalMS: 1000,
		},
		HookClient: HookClientConfig{
			RequestTimeoutMS: 1500,
			AutostartEnabled: false,
			AutostartCommand: nil,
		},
		AppServer: AppServerConfig{
			Command:          []string{"codex", "app-server"},
			RequestTimeoutMS: 30000,
			ClientInfo: ClientInfoConfig{
				Name:    "codex_buddy_debug",
				Title:   "codex-buddy Debug Client",
				Version: "0.1.0",
			},
		},
		LocalServer: LocalServerConfig{
			Enabled: false,
		},
		UConsole: UConsoleConfig{
			ServerURL:        "http://127.0.0.1:8787",
			HTTPTimeoutMS:    3000,
			ReconnectDelayMS: 3000,
			PollFallbackMS:   10000,
			ContinueHoldMS:   1000,
			Window: UConsoleWindowConfig{
				Width:      920,
				Height:     540,
				Fullscreen: false,
			},
			LED: UConsoleLEDConfig{
				Enabled:    true,
				Pixels:     8,
				Brightness: 40,
				GPIOPin:    45,
				DmaNum:     10,
				Frequency:  800000,
			},
		},
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Listen.Host == "" {
		cfg.Listen.Host = "127.0.0.1"
	}
	if cfg.Listen.Port == 0 {
		cfg.Listen.Port = 8787
	}
	if cfg.HookClient.RequestTimeoutMS <= 0 {
		cfg.HookClient.RequestTimeoutMS = 1500
	}
	if cfg.Transcript.PollIntervalMS <= 0 {
		cfg.Transcript.PollIntervalMS = 1000
	}
	if len(cfg.AppServer.Command) == 0 {
		cfg.AppServer.Command = []string{"codex", "app-server"}
	}
	if cfg.AppServer.RequestTimeoutMS <= 0 {
		cfg.AppServer.RequestTimeoutMS = 30000
	}
	if cfg.AppServer.ClientInfo.Name == "" {
		cfg.AppServer.ClientInfo.Name = "codex_buddy_debug"
	}
	if cfg.AppServer.ClientInfo.Title == "" {
		cfg.AppServer.ClientInfo.Title = "codex-buddy Debug Client"
	}
	if cfg.AppServer.ClientInfo.Version == "" {
		cfg.AppServer.ClientInfo.Version = "0.1.0"
	}
	if cfg.UConsole.ServerURL == "" {
		cfg.UConsole.ServerURL = "http://127.0.0.1:8787"
	}
	if cfg.UConsole.HTTPTimeoutMS <= 0 {
		cfg.UConsole.HTTPTimeoutMS = 3000
	}
	if cfg.UConsole.ReconnectDelayMS <= 0 {
		cfg.UConsole.ReconnectDelayMS = 3000
	}
	if cfg.UConsole.PollFallbackMS <= 0 {
		cfg.UConsole.PollFallbackMS = 10000
	}
	if cfg.UConsole.ContinueHoldMS <= 0 {
		cfg.UConsole.ContinueHoldMS = 1000
	}
	if cfg.UConsole.Window.Width <= 0 {
		cfg.UConsole.Window.Width = 920
	}
	if cfg.UConsole.Window.Height <= 0 {
		cfg.UConsole.Window.Height = 540
	}
	if cfg.UConsole.LED.Pixels <= 0 {
		cfg.UConsole.LED.Pixels = 8
	}
	if cfg.UConsole.LED.Brightness <= 0 {
		cfg.UConsole.LED.Brightness = 40
	}
	if cfg.UConsole.LED.GPIOPin <= 0 {
		cfg.UConsole.LED.GPIOPin = 45
	}
	if cfg.UConsole.LED.DmaNum <= 0 {
		cfg.UConsole.LED.DmaNum = 10
	}
	if cfg.UConsole.LED.Frequency <= 0 {
		cfg.UConsole.LED.Frequency = 800000
	}
}

func (c ListenConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c Config) InternalBaseURL() string {
	host := c.Listen.Host
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	default:
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			host = "127.0.0.1"
		}
	}
	return fmt.Sprintf("http://%s:%d", host, c.Listen.Port)
}

func (c Config) PublicBaseURL() string {
	return fmt.Sprintf("http://%s", c.Listen.Address())
}

func (c Config) IngestURL() string {
	return c.InternalBaseURL() + "/v1/internal/hooks"
}

func (c Config) ShutdownURL() string {
	return c.InternalBaseURL() + "/v1/internal/shutdown"
}
