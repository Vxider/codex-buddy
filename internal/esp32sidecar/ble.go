package esp32sidecar

import (
	"context"
	"fmt"
)

type BLEOptions struct {
	AdapterName string
}

type BLEPublisher struct{}

func OpenBLEAdvertisement(context.Context, BLEOptions) (*BLEPublisher, error) {
	return nil, fmt.Errorf("ble advertising is not implemented yet; use UART transport")
}

func (p *BLEPublisher) Publish(context.Context, Frame) error {
	return fmt.Errorf("ble advertising is not implemented yet")
}

func (p *BLEPublisher) Close() error {
	return nil
}
