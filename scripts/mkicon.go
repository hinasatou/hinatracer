//go:build ignore

// mkicon builds resources/icon.png from the source artwork: it finds the
// rounded-square app tile on the dark backdrop, crops it, and writes an
// RGBA PNG whose outside is fully transparent. The tile edge is an
// analytic rounded rectangle (supersampled) so edges are antialiased and
// no backdrop pixels bleed in as a dark halo.
//
//	go run scripts/mkicon.go -in artwork.jpg [-out resources/icon.png] [-size 1024]
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
)

func lum(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 65535
}

func main() {
	in := flag.String("in", "", "source image (png/jpeg)")
	out := flag.String("out", "resources/icon.png", "output PNG path")
	size := flag.Int("size", 1024, "output square size")
	margin := flag.Float64("margin", 0.04, "transparent margin per side (fraction of size)")
	inset := flag.Float64("inset", 2.5, "pixels to pull the tile edge inward in source space (drops blended backdrop)")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: go run scripts/mkicon.go -in <image> [-out resources/icon.png] [-size 1024]")
		os.Exit(2)
	}
	f, err := os.Open(*in)
	if err != nil {
		panic(err)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		panic(err)
	}
	b := src.Bounds()
	// The backdrop is a near-uniform dark navy (sampled at the corner); the
	// tile is everything clearly different from it (including the dark
	// silhouette, which is much bluer than the backdrop).
	bg := src.At(b.Min.X+2, b.Min.Y+2)
	br, bgc, bb, _ := bg.RGBA()
	bright := func(x, y int) bool {
		r, g, bl, _ := src.At(x, y).RGBA()
		dr := float64(r>>8) - float64(br>>8)
		dg := float64(g>>8) - float64(bgc>>8)
		db := float64(bl>>8) - float64(bb>>8)
		return math.Sqrt(dr*dr+dg*dg+db*db) > 60
	}
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if bright(x, y) {
				x0, y0 = min(x0, x), min(y0, y)
				x1, y1 = max(x1, x), max(y1, y)
			}
		}
	}
	// Corner radius from the 45° diagonal inset at the top-left corner.
	k := 0
	for ; k < (x1-x0)/2; k++ {
		if bright(x0+k, y0+k) {
			break
		}
	}
	r := float64(k) / (1 - 1/math.Sqrt2)
	fx0, fy0 := float64(x0)+*inset, float64(y0)+*inset
	fx1, fy1 := float64(x1+1)-*inset, float64(y1+1)-*inset
	r -= *inset
	fmt.Printf("tile bbox (%d,%d)-(%d,%d) radius≈%.1f\n", x0, y0, x1, y1, r)

	// Crop square around the tile and scale to the inner area.
	S := *size
	m := int(math.Round(float64(S) * *margin))
	inner := S - 2*m
	tw, th := fx1-fx0, fy1-fy0
	scale := float64(inner) / math.Max(tw, th)

	rgba := image.NewRGBA(image.Rect(0, 0, S, S))
	crop := image.Rect(int(math.Floor(fx0)), int(math.Floor(fy0)), int(math.Ceil(fx1)), int(math.Ceil(fy1)))
	dstRect := image.Rect(m, m, m+int(math.Round(float64(crop.Dx())*scale)), m+int(math.Round(float64(crop.Dy())*scale)))
	xdraw.CatmullRom.Scale(rgba, dstRect, src, crop, xdraw.Src, nil)

	// Rounded-rect coverage in output space, 4x4 supersampling.
	ox0 := float64(m) + (fx0-float64(crop.Min.X))*scale
	oy0 := float64(m) + (fy0-float64(crop.Min.Y))*scale
	ox1 := ox0 + tw*scale
	oy1 := oy0 + th*scale
	or := r * scale
	inside := func(px, py float64) bool {
		qx := math.Max(math.Max(ox0+or-px, px-(ox1-or)), 0)
		qy := math.Max(math.Max(oy0+or-py, py-(oy1-or)), 0)
		if px < ox0 || px > ox1 || py < oy0 || py > oy1 {
			return false
		}
		return qx*qx+qy*qy <= or*or
	}
	out1 := image.NewNRGBA(image.Rect(0, 0, S, S))
	const ss = 4
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			n := 0
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					if inside(float64(x)+(float64(i)+0.5)/ss, float64(y)+(float64(j)+0.5)/ss) {
						n++
					}
				}
			}
			if n == 0 {
				continue // fully transparent (0,0,0,0)
			}
			c := rgba.RGBAAt(x, y)
			a := uint8(n * 255 / (ss * ss))
			// Straight (non-premultiplied) color keeps edge pixels the tile's own color.
			out1.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, a})
		}
	}
	o, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer o.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(o, out1); err != nil {
		panic(err)
	}
}
