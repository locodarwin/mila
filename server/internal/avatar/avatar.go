// Package avatar turns an uploaded picture into the square one Mila keeps
// for a person: PNG in, checked for size before any pixels are decoded,
// center-cropped, scaled down to at most OutSide and encoded again, which
// drops every bit of metadata the upload carried.
package avatar

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"

	"github.com/locodarwin/mila/server/internal/apperr"
)

// Limits on what is accepted and what is kept.
const (
	MaxBytes = 4 << 20 // the largest upload
	MaxSide  = 1024    // the longest side accepted
	MinSide  = 16      // the shortest side accepted
	OutSide  = 256     // the side of the picture kept, at most
)

// slots holds the decodes running now: two at most, so a burst of
// uploads cannot take the server's memory.
var slots = make(chan struct{}, 2)

// Process checks a PNG and returns the picture to keep: a JPEG at quality
// 85 when it is opaque, otherwise a PNG, with its MIME type.
func Process(ctx context.Context, data []byte) ([]byte, string, error) {
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer func() { <-slots }()

	if http.DetectContentType(data) != "image/png" {
		return nil, "", apperr.New(apperr.Invalid, "Send the picture as a PNG.")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", unreadable()
	}
	if cfg.Width > MaxSide || cfg.Height > MaxSide {
		return nil, "", apperr.New(apperr.Invalid, "Pictures can be at most %d pixels on a side.", MaxSide)
	}
	if cfg.Width < MinSide || cfg.Height < MinSide {
		return nil, "", apperr.New(apperr.Invalid, "Pictures must be at least %d pixels on a side.", MinSide)
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", unreadable()
	}

	// The centered square, drawn into premultiplied RGBA so averaging
	// leaves no dark fringe where the picture is see-through.
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	at := image.Pt(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2)
	square := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(square, square.Bounds(), src, at, draw.Src)
	img := scale(square, min(OutSide, side))

	var out bytes.Buffer
	if img.Opaque() {
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", err
		}
		return out.Bytes(), "image/jpeg", nil
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, img); err != nil {
		return nil, "", err
	}
	return out.Bytes(), "image/png", nil
}

func unreadable() error { return apperr.New(apperr.Invalid, "That picture could not be read.") }

// scale shrinks a square picture to side n by averaging the area each new
// pixel covers (a box filter), one direction at a time. It never enlarges.
func scale(src *image.RGBA, n int) *image.RGBA {
	s := src.Bounds().Dx()
	if n >= s {
		return src
	}
	w := weights(s, n)
	// Across: s rows of n pixels, four channels each.
	mid := make([]float64, s*n*4)
	for y := range s {
		row := src.Pix[y*src.Stride:]
		for x, ws := range w {
			for _, t := range ws {
				for c := range 4 {
					mid[(y*n+x)*4+c] += t.w * float64(row[t.i*4+c])
				}
			}
		}
	}
	// Down: n rows of n pixels.
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for y, ws := range w {
		for x := range n {
			for c := range 4 {
				var v float64
				for _, t := range ws {
					v += t.w * mid[(t.i*n+x)*4+c]
				}
				dst.Pix[y*dst.Stride+x*4+c] = uint8(min(255, v+0.5))
			}
		}
	}
	return dst
}

type tap struct {
	i int     // the source pixel
	w float64 // the share of the new pixel it covers
}

// weights says, for each of n new pixels along a line of s, which source
// pixels it covers and by how much; each pixel's shares add up to one.
func weights(s, n int) [][]tap {
	out := make([][]tap, n)
	ratio := float64(s) / float64(n)
	for j := range n {
		lo, hi := float64(j)*ratio, float64(j+1)*ratio
		for i := int(lo); i < s && float64(i) < hi; i++ {
			cover := min(hi, float64(i+1)) - max(lo, float64(i))
			if cover > 0 {
				out[j] = append(out[j], tap{i, cover / ratio})
			}
		}
	}
	return out
}
