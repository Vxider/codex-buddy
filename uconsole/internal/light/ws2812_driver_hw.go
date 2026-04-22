//go:build ws281x

package light

import (
	"context"
	"log"

	ws2811 "github.com/rpi-ws281x/rpi-ws281x-go"
)

type WS2812Driver struct {
	device *ws2811.WS2811
	logger *log.Logger
}

func NewWS2812Driver(cfg WS2812Config, logger *log.Logger) (Driver, error) {
	if !cfg.Enabled {
		return NewNoopDriver(), nil
	}

	options := ws2811.DefaultOptions
	options.Frequency = cfg.Frequency
	options.DmaNum = cfg.DmaNum
	options.Channels = []ws2811.ChannelOption{
		{
			GpioPin:    cfg.GPIOPin,
			LedCount:   cfg.Pixels,
			Brightness: cfg.Brightness,
			StripeType: ws2811.WS2812Strip,
			Invert:     false,
		},
	}

	device, err := ws2811.MakeWS2811(&options)
	if err != nil {
		return nil, err
	}
	if err := device.Init(); err != nil {
		return nil, err
	}

	if logger != nil {
		logger.Printf("ws2812 driver initialized pixels=%d gpio=%d brightness=%d dma=%d", cfg.Pixels, cfg.GPIOPin, cfg.Brightness, cfg.DmaNum)
	}

	return &WS2812Driver{device: device, logger: logger}, nil
}

func (d *WS2812Driver) Apply(_ context.Context, plan Plan) error {
	frame := RenderPixels(plan)
	leds := d.device.Leds(0)
	for i := range leds {
		if i < len(frame) {
			leds[i] = rgbToUint32(frame[i])
			continue
		}
		leds[i] = 0
	}
	return d.device.Render()
}

func (d *WS2812Driver) Close() error {
	if d.device == nil {
		return nil
	}
	leds := d.device.Leds(0)
	for i := range leds {
		leds[i] = 0
	}
	_ = d.device.Render()
	d.device.Fini()
	return nil
}

func rgbToUint32(pixel Pixel) uint32 {
	return uint32(pixel.R)<<16 | uint32(pixel.G)<<8 | uint32(pixel.B)
}
