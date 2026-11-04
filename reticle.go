package main

import "math"

// Signed distance in physical monitor pixels. Both the editor and the overlay
// sample the same outline, including partially covered edge pixels.
func shapeDistance(kind int, r reticle, x, y float64) float64 {
	length, half, gap := float64(r.Size), float64(r.Width)*0.5, float64(r.Gap)
	ax, ay := math.Abs(x), math.Abs(y)
	switch kind {
	case 1:
		return math.Hypot(x, y) - length*.5
	case 2:
		return math.Abs(math.Hypot(x, y)-length) - half
	case 4:
		return segmentDistance(ax, y, 0, 0, length, length) - half
	case 5:
		return segmentDistance(ax, ay, 0, length, length, 0) - half
	}
	// The cross strokes have flat ends. The T preset omits the upper arm.
	h := math.Max(math.Max(gap-ax, ax-gap-length), ay-half)
	v := ay
	if kind == 3 {
		v = y
	}
	return math.Min(h, math.Max(math.Max(gap-v, v-gap-length), ax-half))
}

func segmentDistance(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-ax-t*dx, y-ay-t*dy)
}

type sprite struct {
	side   int
	pixels []uint32
}

func makeReticle(kind int, r reticle, scale float64) sprite {
	radius := r.Size + r.Width + 3
	if kind == 0 || kind == 3 {
		radius += r.Gap
	}
	half := int(math.Ceil(float64(radius) * scale))
	s := sprite{2*half + 1, make([]uint32, (2*half+1)*(2*half+1))}
	for y := -half; y <= half; y++ {
		for x := -half; x <= half; x++ {
			px, py := float64(x)/scale, float64(y)/scale
			d := shapeDistance(kind, r, px, py)
			if r.Center && kind != 1 {
				d = math.Min(d, math.Hypot(px, py)-math.Max(1, float64(r.Width)*.55))
			}
			fill := math.Max(0, math.Min(1, .5-d*scale))
			total := fill
			if r.Outline {
				total = math.Max(0, math.Min(1, .5-(d-1)*scale))
			}
			if total == 0 {
				continue
			}
			opacity := float64(r.Opacity) / 100
			edge := (total - fill) * opacity
			channel := func(shift uint) uint32 {
				return uint32(math.Round(float64((r.Color>>shift)&255)*fill*opacity + 12*edge))
			}
			s.pixels[(y+half)*s.side+x+half] = uint32(math.Round(255*total*opacity))<<24 | channel(16)<<16 | channel(8)<<8 | channel(0)
		}
	}
	return s
}
