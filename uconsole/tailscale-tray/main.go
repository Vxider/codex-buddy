package main

import (
	"log"
	"os"

	"github.com/vxider/codex-buddy/uconsole/tailscale-tray/internal/tray"
)

func main() {
	logger := log.New(os.Stdout, "tailscale-tray: ", log.LstdFlags|log.Lmsgprefix)
	tray.Run(logger)
}
