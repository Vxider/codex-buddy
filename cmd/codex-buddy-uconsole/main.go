package main

import (
	"os"

	"github.com/vxider/codex-buddy/internal/uconsolecmd"
)

func main() {
	os.Exit(uconsolecmd.Run(os.Args[1:]))
}
