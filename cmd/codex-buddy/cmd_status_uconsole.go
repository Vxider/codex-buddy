package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/uconsole"
)

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	var configPath string
	var asJSON bool
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.BoolVar(&asJSON, "json", false, "Print raw JSON status")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}

	status, err := fetchStatus(cfg)
	if err != nil {
		fmt.Printf("status request failed: %v\n", err)
		return 1
	}

	if asJSON {
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			fmt.Printf("marshal status: %v\n", err)
			return 1
		}
	fmt.Println(string(data))
	return 0
}

fmt.Printf("overall: %s\n", status.OverallState)
fmt.Printf("sessions: %d\n", status.SessionsCount)
for _, session := range status.Sessions {
	fmt.Printf("- %s %s\n", session.SessionID, session.State)
}
	return 0
}

func runUConsole(args []string) int {
	fs := flag.NewFlagSet("uconsole", flag.ExitOnError)
	var configPath string
	var serverURL string
	var noLED bool
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.StringVar(&serverURL, "server-url", "", "Override uConsole buddy server URL")
	fs.BoolVar(&noLED, "no-led", false, "Disable WS2812 output for local debugging")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}
	if serverURL != "" {
		cfg.UConsole.ServerURL = serverURL
	}
	if noLED {
		cfg.UConsole.LED.Enabled = false
	}

	logger := log.New(os.Stdout, "codex-buddy-uconsole: ", log.LstdFlags|log.Lmsgprefix)
	if err := uconsole.Run(context.Background(), cfg.UConsole, logger); err != nil {
		logger.Printf("uconsole failed: %v", err)
		return 1
	}
	return 0
}
