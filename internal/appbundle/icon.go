package appbundle

import (
	"image"
	"image/color"
	"math"
)

// The icon is drawn on Apple's 1024-point grid: a dark rounded square with
// the app's three bars (oMLX blue, Splash orange) and a faint dot grid like
// the stage in the app.
const grid = 1024.0

var (
	tileColor  = color.RGBA{0x17, 0x1C, 0x24, 0xFF}
	dotColor   = color.RGBA{0x2A, 0x32, 0x40, 0xFF}
	omlxColor  = color.RGBA{0x5B, 0x7B, 0xD5, 0xFF}
	splashTone = color.RGBA{0xF2, 0x9A, 0x5B, 0xFF}
)

type roundRect struct {
	x0, y0, x1, y1, r float64
	fill              color.RGBA
}

// contains uses the signed distance of a rounded rectangle.
func (b roundRect) contains(x, y float64) bool {
	cx, cy := (b.x0+b.x1)/2, (b.y0+b.y1)/2
	hx, hy := (b.x1-b.x0)/2-b.r, (b.y1-b.y0)/2-b.r
	dx := math.Max(math.Abs(x-cx)-hx, 0)
	dy := math.Max(math.Abs(y-cy)-hy, 0)
	return dx*dx+dy*dy <= b.r*b.r
}

var tile = roundRect{100, 100, 924, 924, 185, tileColor}

// bars maps the 30-unit logo of the page (bars at x 3, 12, 21; 6 wide;
// bottom at 27) onto a 512-point box in the middle of the tile.
var bars = func() []roundRect {
	const unit, offset = 512.0 / 30, 256.0
	logo := []struct {
		x, top float64
		fill   color.RGBA
	}{{3, 17, omlxColor}, {12, 10, splashTone}, {21, 3, splashTone}}
	out := make([]roundRect, 0, len(logo))
	for _, l := range logo {
		out = append(out, roundRect{offset + l.x*unit, offset + l.top*unit, offset + (l.x+6)*unit, offset + 27*unit, 1.5 * unit, l.fill})
	}
	return out
}()

// colorAt returns the topmost shape's color at a point of the grid and
// whether any shape covers it.
func colorAt(x, y float64, dots bool) (color.RGBA, bool) {
	for i := len(bars) - 1; i >= 0; i-- {
		if bars[i].contains(x, y) {
			return bars[i].fill, true
		}
	}
	if !tile.contains(x, y) {
		return color.RGBA{}, false
	}
	if dots {
		// 40-point spacing, dot radius 4, centred in the tile.
		gx, gy := math.Mod(x-112, 40)-20, math.Mod(y-112, 40)-20
		if gx*gx+gy*gy <= 16 {
			return dotColor, true
		}
	}
	return tileColor, true
}

// Icon renders the icon at size×size pixels with 4×4 supersampling. The dot
// grid only appears from 128 pixels, below it would turn to noise.
func Icon(size int) *image.RGBA {
	const samples = 4
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := grid / float64(size)
	dots := size >= 128
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range samples {
				for sx := range samples {
					x := (float64(px) + (float64(sx)+0.5)/samples) * scale
					y := (float64(py) + (float64(sy)+0.5)/samples) * scale
					if c, ok := colorAt(x, y, dots); ok {
						r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+255
					}
				}
			}
			const n = samples * samples
			// Premultiplied: covered samples carry their colour, the rest is transparent.
			img.SetRGBA(px, py, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return img
}

// IconsetEntry is one image of a macOS .iconset folder.
type IconsetEntry struct {
	Name string
	Size int
}

// Iconset lists the images iconutil expects.
var Iconset = []IconsetEntry{
	{"icon_16x16.png", 16}, {"icon_16x16@2x.png", 32},
	{"icon_32x32.png", 32}, {"icon_32x32@2x.png", 64},
	{"icon_128x128.png", 128}, {"icon_128x128@2x.png", 256},
	{"icon_256x256.png", 256}, {"icon_256x256@2x.png", 512},
	{"icon_512x512.png", 512}, {"icon_512x512@2x.png", 1024},
}
