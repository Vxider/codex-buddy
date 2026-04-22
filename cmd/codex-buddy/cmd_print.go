package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vxider/codex-buddy/internal/config"
)

func runPrintConfig(args []string) int {
	fs := flag.NewFlagSet("print-config", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to codex-buddy JSON config")
	_ = fs.Parse(args)

	resolvedConfigPath, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("resolve config path: %v\n", err)
		return 1
	}

	data, err := os.ReadFile(resolvedConfigPath)
	if err != nil {
		fmt.Printf("read config: %v\n", err)
		return 1
	}

	fmt.Print(string(data))
	if len(data) == 0 || data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func runPrintHooks(args []string) int {
	fs := flag.NewFlagSet("print-hooks", flag.ExitOnError)
	_ = fs.Parse(args)

	paths, err := resolveUserPaths()
	if err != nil {
		fmt.Printf("resolve user home: %v\n", err)
		return 1
	}

	data, err := os.ReadFile(paths.hooksPath)
	if err != nil {
		fmt.Printf("read hooks: %v\n", err)
		return 1
	}

	fmt.Print(string(data))
	if len(data) == 0 || data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func runPrintService(args []string) int {
	fs := flag.NewFlagSet("print-service", flag.ExitOnError)
	_ = fs.Parse(args)

	paths, err := resolveUserPaths()
	if err != nil {
		fmt.Printf("resolve user home: %v\n", err)
		return 1
	}

	data, err := os.ReadFile(paths.servicePath)
	if err != nil {
		fmt.Printf("read service: %v\n", err)
		return 1
	}

	fmt.Print(string(data))
	if len(data) == 0 || data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return 0
}
