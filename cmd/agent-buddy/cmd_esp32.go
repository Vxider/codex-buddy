package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vxider/agent-buddy/internal/config"
	"github.com/vxider/agent-buddy/internal/esp32sidecar"
)

func runESP32(args []string) int {
	fs := flag.NewFlagSet("esp32", flag.ExitOnError)
	var configPath string
	var serverURL string
	var uartDevice string
	var baud int
	var once bool
	var motor int
	fs.StringVar(&configPath, "config", "", "Path to agent-buddy JSON config")
	fs.StringVar(&serverURL, "server-url", "", "agent-buddy server URL")
	fs.StringVar(&uartDevice, "uart", "", "UART device such as /dev/ttyACM0")
	fs.IntVar(&baud, "baud", 115200, "UART baud rate")
	fs.BoolVar(&once, "once", false, "Publish one status frame and exit")
	fs.IntVar(&motor, "motor", -1, "Set motor duty 0-255 and exit")
	_ = fs.Parse(args)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("load config: %v\n", err)
		return 1
	}
	if serverURL == "" {
		serverURL = cfg.InternalBaseURL()
	}
	if uartDevice == "" {
		fmt.Println("missing --uart device")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	publisher, err := esp32sidecar.OpenUART(ctx, esp32sidecar.UARTOptions{Device: uartDevice, Baud: baud})
	if err != nil {
		fmt.Printf("open uart: %v\n", err)
		return 1
	}
	defer publisher.Close()

	if motor >= 0 {
		if motor > 255 {
			fmt.Println("--motor must be between 0 and 255")
			return 2
		}
		if _, err := io.WriteString(publisher, fmt.Sprintf("motor %d\n", motor)); err != nil {
			fmt.Printf("motor command failed: %v\n", err)
			return 1
		}
		fmt.Printf("motor %d sent to %s\n", motor, uartDevice)
		return 0
	}

	client := esp32sidecar.NewClient(serverURL, 3*time.Second)
	if once {
		status, err := client.LoadStatus(ctx)
		if err != nil {
			fmt.Printf("status request failed: %v\n", err)
			return 1
		}
		frame := esp32sidecar.FrameFromStatus(status)
		if err := publisher.Publish(ctx, frame); err != nil {
			fmt.Printf("publish failed: %v\n", err)
			return 1
		}
		fmt.Printf("published %s to %s\n", frame.State, uartDevice)
		return 0
	}

	fmt.Printf("bridging %s -> %s at %d baud\n", serverURL, uartDevice, baud)
	if err := esp32sidecar.RunBridge(ctx, client, publisher); err != nil {
		fmt.Printf("bridge failed: %v\n", err)
		return 1
	}
	return 0
}
