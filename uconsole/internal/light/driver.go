package light

import "context"

type WS2812Config struct {
	Enabled    bool
	Pixels     int
	Brightness int
	GPIOPin    int
	DmaNum     int
	Frequency  int
}

type NoopDriver struct{}

func NewNoopDriver() *NoopDriver {
	return &NoopDriver{}
}

func (d *NoopDriver) Apply(_ context.Context, _ Plan) error {
	return nil
}

func (d *NoopDriver) Close() error {
	return nil
}
