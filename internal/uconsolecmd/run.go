package uconsolecmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/uconsole"
)

func Run(args []string) int {
	fs := flag.NewFlagSet("uconsole", flag.ExitOnError)
	var configPath string
	var serverURL string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	fs.StringVar(&serverURL, "server-url", "", "Override uConsole buddy server URL")
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

	logger := log.New(os.Stdout, "codex-buddy-uconsole: ", log.LstdFlags|log.Lmsgprefix)
	if err := uconsole.Run(context.Background(), cfg, resolvedConfigPath, logger); err != nil {
		logger.Printf("uconsole failed: %v", err)
		return 1
	}
	return 0
}
