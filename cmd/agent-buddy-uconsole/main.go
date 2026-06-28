package main

import (
	"os"

	"github.com/vxider/agent-buddy/internal/uconsolecmd"
)

func main() {
	os.Exit(uconsolecmd.Run(os.Args[1:]))
}
