package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "hook":
		os.Exit(runHook(os.Args[2:]))
	case "claude-hook":
		os.Exit(runClaudeHook(os.Args[2:]))
	case "setup":
		os.Exit(runSetup(os.Args[2:]))
	case "status":
		os.Exit(runStatus(os.Args[2:]))
	case "tmux-window-dot":
		os.Exit(runTmuxWindowDot(os.Args[2:]))
	case "start":
		os.Exit(runStart(os.Args[2:]))
	case "restart":
		os.Exit(runRestart(os.Args[2:]))
	case "stop":
		os.Exit(runStop(os.Args[2:]))
	case "uconsole":
		os.Exit(runUConsole(os.Args[2:]))
	case "esp32":
		os.Exit(runESP32(os.Args[2:]))
	case "print-config":
		os.Exit(runPrintConfig(os.Args[2:]))
	case "print-hooks":
		os.Exit(runPrintHooks(os.Args[2:]))
	case "print-claude-hooks":
		os.Exit(runPrintClaudeHooks(os.Args[2:]))
	case "print-service":
		os.Exit(runPrintService(os.Args[2:]))
	case "logs":
		os.Exit(runLogs(os.Args[2:]))
	case "doctor":
		os.Exit(runDoctor(os.Args[2:]))
	case "uninstall":
		os.Exit(runUninstall(os.Args[2:]))
	case "version", "--version", "-V":
		fmt.Println("agent-buddy 0.1.0")
		return
	case "help", "--help", "-h":
		usage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  agent-buddy serve [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy hook <event-name> [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy claude-hook <event-name> [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy setup [--config path] [--host 0.0.0.0] [--port 8787] [--skip-systemd]")
	_, _ = fmt.Fprintln(w, "  agent-buddy status [--config path] [--json]")
	_, _ = fmt.Fprintln(w, "  agent-buddy tmux-window-dot [--config path] [--timeout-ms 150] <window-id>")
	_, _ = fmt.Fprintln(w, "  agent-buddy start [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy restart [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy stop [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy uconsole [--config path] [--server-url url]  # compatibility")
	_, _ = fmt.Fprintln(w, "  agent-buddy esp32 --uart /dev/ttyACM0 [--server-url url] [--baud 115200] [--once] [--motor 0-255]")
	_, _ = fmt.Fprintln(w, "  agent-buddy-uconsole [--config path] [--server-url url]")
	_, _ = fmt.Fprintln(w, "  agent-buddy print-config [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy print-hooks")
	_, _ = fmt.Fprintln(w, "  agent-buddy print-claude-hooks [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy print-service")
	_, _ = fmt.Fprintln(w, "  agent-buddy logs [--lines 50]")
	_, _ = fmt.Fprintln(w, "  agent-buddy doctor [--config path]")
	_, _ = fmt.Fprintln(w, "  agent-buddy uninstall [--config path] [--keep-binary]")
	_, _ = fmt.Fprintln(w, "  agent-buddy version")
}
