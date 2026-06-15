package esp32sidecar

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

type UARTOptions struct {
	Device string
	Baud   int
}

type UARTPublisher struct {
	file *os.File
}

func OpenUART(ctx context.Context, opts UARTOptions) (*UARTPublisher, error) {
	if opts.Device == "" {
		return nil, fmt.Errorf("uart device is required")
	}
	if opts.Baud <= 0 {
		opts.Baud = 115200
	}
	if err := configureSerial(ctx, opts.Device, opts.Baud); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(opts.Device, os.O_WRONLY|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open uart %s: %w", opts.Device, err)
	}
	return &UARTPublisher{file: file}, nil
}

func (p *UARTPublisher) Publish(_ context.Context, frame Frame) error {
	_, err := p.file.Write(Encode(frame))
	return err
}

func (p *UARTPublisher) Write(data []byte) (int, error) {
	return p.file.Write(data)
}

func (p *UARTPublisher) Close() error {
	return p.file.Close()
}

func configureSerial(ctx context.Context, device string, baud int) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "stty", "-F", device, strconv.Itoa(baud), "cs8", "-cstopb", "-parenb", "-ixon", "-ixoff", "raw")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("configure uart %s: %w: %s", device, err, string(output))
	}
	return nil
}

type Publisher interface {
	Publish(context.Context, Frame) error
	io.Closer
}
