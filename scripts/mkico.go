//go:build ignore

// mkico writes a multi-size Windows .ico (PNG-compressed entries with
// alpha) from resources/icon.png.
//
//	go run scripts/mkico.go [-in resources/icon.png] [-out build/icon.ico]
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

var sizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}

func main() {
	in := flag.String("in", "resources/icon.png", "source PNG with alpha")
	out := flag.String("out", "build/icon.ico", "output .ico")
	flag.Parse()
	f, err := os.Open(*in)
	if err != nil {
		panic(err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		panic(err)
	}
	var blobs [][]byte
	for _, s := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, s, s))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dst); err != nil {
			panic(err)
		}
		blobs = append(blobs, b.Bytes())
	}
	var o bytes.Buffer
	binary.Write(&o, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for i, s := range sizes {
		d := uint8(s)
		if s >= 256 {
			d = 0
		}
		o.Write([]byte{d, d, 0, 0})
		binary.Write(&o, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&o, binary.LittleEndian, [2]uint32{uint32(len(blobs[i])), uint32(off)})
		off += len(blobs[i])
	}
	for _, b := range blobs {
		o.Write(b)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*out, o.Bytes(), 0o644); err != nil {
		panic(err)
	}
}
