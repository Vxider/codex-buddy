//go:build !ws281x

package light

import (
	"log"
)

func NewWS2812Driver(cfg WS2812Config, logger *log.Logger) (Driver, error) {
	if logger != nil && cfg.Enabled {
		logger.Printf("ws2812 driver disabled: build without -tags ws281x, using noop driver")
	}
	return NewNoopDriver(), nil
}
