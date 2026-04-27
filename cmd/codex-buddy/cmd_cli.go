package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/vxider/codex-buddy/cli"
	"github.com/vxider/codex-buddy/internal/config"
)

func runCLI(args []string) int {
	fs := flag.NewFlagSet("cli", flag.ExitOnError)
	var configPath string
	var serverURL string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.StringVar(&serverURL, "server-url", "", "Override seed remote server URL")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}
	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("resolve config: %v\n", err)
		return 1
	}
	if serverURL != "" {
		cfg.UConsole.ServerURL = serverURL
	}

	logger := log.New(io.Discard, "", 0)
	if err := cli.Run(context.Background(), cfg, resolvedConfigPath, logger); err != nil {
		fmt.Printf("cli failed: %v\n", err)
		return 1
	}
	return 0
}
