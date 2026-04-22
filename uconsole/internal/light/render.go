package light

import (
	"math"
	"time"
)

func RenderPixels(plan Plan) []Pixel {
	pixels := plan.Pixels
	if pixels <= 0 {
		return nil
	}

	frame := fillPixels(pixels, plan.Background)
	accent := firstPixel(plan.Accent, Pixel{R: 255, G: 255, B: 255})
	base := firstPixel(plan.Base, accent)

	switch plan.Mode {
	case EffectModeOff:
		return frame
	case EffectModeSolid:
		return fillPixels(pixels, blend(plan.Background, base, 1))
	case EffectModeChase:
		index := cycleIndex(plan.Phase, plan.Cycle, pixels)
		frame[index] = accent
		if pixels > 2 {
			frame[(index-1+pixels)%pixels] = blend(plan.Background, accent, 0.42)
			frame[(index+1)%pixels] = blend(plan.Background, accent, 0.65)
		}
		return frame
	case EffectModeScanner:
		index := bounceIndex(plan.Phase, plan.Cycle, pixels)
		frame[index] = accent
		if pixels > 2 {
			frame[(index-1+pixels)%pixels] = blend(plan.Background, accent, 0.35)
			frame[(index+1)%pixels] = blend(plan.Background, accent, 0.35)
		}
		return frame
	case EffectModePulse:
		strength := 0.32 + 0.68*((math.Sin(radians(plan.Phase, plan.Cycle))+1)/2)
		return fillPixels(pixels, blend(plan.Background, accent, strength))
	case EffectModeFlash:
		on := (plan.Cycle == 0) || ((plan.Phase/(plan.Cycle/2))%2 == 0)
		if on {
			return fillPixels(pixels, accent)
		}
		return frame
	default:
		return fillPixels(pixels, base)
	}
}

func fillPixels(count int, pixel Pixel) []Pixel {
	out := make([]Pixel, count)
	for i := range out {
		out[i] = pixel
	}
	return out
}

func cycleIndex(phase, cycle time.Duration, pixels int) int {
	if pixels <= 1 {
		return 0
	}
	if cycle <= 0 {
		cycle = time.Duration(pixels) * 100 * time.Millisecond
	}
	step := int((phase % cycle) * time.Duration(pixels) / cycle)
	if step < 0 {
		return 0
	}
	if step >= pixels {
		return pixels - 1
	}
	return step
}

func bounceIndex(phase, cycle time.Duration, pixels int) int {
	if pixels <= 1 {
		return 0
	}
	span := pixels*2 - 2
	if span <= 0 {
		return 0
	}
	if cycle <= 0 {
		cycle = time.Duration(span) * 90 * time.Millisecond
	}
	step := int((phase % cycle) * time.Duration(span) / cycle)
	if step >= pixels {
		step = span - step
	}
	return step
}

func blend(base, accent Pixel, factor float64) Pixel {
	return Pixel{
		R: mixChannel(base.R, accent.R, factor),
		G: mixChannel(base.G, accent.G, factor),
		B: mixChannel(base.B, accent.B, factor),
	}
}

func mixChannel(a, b uint8, factor float64) uint8 {
	if factor <= 0 {
		return a
	}
	if factor >= 1 {
		return b
	}
	return uint8(float64(a) + (float64(b)-float64(a))*factor)
}

func firstPixel(items []Pixel, fallback Pixel) Pixel {
	if len(items) == 0 {
		return fallback
	}
	return items[0]
}

func radians(phase, cycle time.Duration) float64 {
	if cycle <= 0 {
		cycle = time.Second
	}
	return float64((phase % cycle)) / float64(cycle) * 2 * math.Pi
}
