package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func trayIcon(fill color.NRGBA) []byte {
	const size = 16

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	stroke := color.NRGBA{R: 0xF5, G: 0xF7, B: 0xFA, A: 0xFF}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := x - 7
			dy := y - 7
			dist := dx*dx + dy*dy
			switch {
			case dist <= 30:
				img.SetNRGBA(x, y, fill)
			case dist <= 42:
				img.SetNRGBA(x, y, stroke)
			default:
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil
	}
	return out.Bytes()
}
