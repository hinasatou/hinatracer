//go:build ignore

package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
)

func main() {
	in := flag.String("in", "", "source image (png/jpeg)")
	out := flag.String("out", "resources/icon.png", "output PNG path")
	size := flag.Int("size", 512, "output square size")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: go run scripts/mkicon.go -in <image> [-out resources/icon.png] [-size 512]")
		os.Exit(2)
	}
	f, err := os.Open(*in)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	side := w
	if h < side {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	cropped := image.NewNRGBA(image.Rect(0, 0, side, side))
	draw.Draw(cropped, cropped.Bounds(), img, image.Pt(x0, y0), draw.Src)
	S := *size
	dst := image.NewNRGBA(image.Rect(0, 0, S, S))
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			sx := x * side / S
			sy := y * side / S
			dst.Set(x, y, cropped.At(sx, sy))
		}
	}
	o, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer o.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(o, dst); err != nil {
		panic(err)
	}
}
