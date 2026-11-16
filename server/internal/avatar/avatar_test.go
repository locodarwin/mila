package avatar

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"testing"

	"github.com/locodarwin/mila/server/internal/apperr"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// filled is a w×h picture painted by f.
func filled(w, h int, f func(x, y int) color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, f(x, y))
		}
	}
	return img
}

func solid(c color.Color) func(x, y int) color.Color { return func(int, int) color.Color { return c } }

func decode(t *testing.T, out []byte, mime string) image.Image {
	t.Helper()
	var img image.Image
	var err error
	switch mime {
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(out))
	case "image/png":
		img, err = png.Decode(bytes.NewReader(out))
	default:
		t.Fatalf("unexpected type %q", mime)
	}
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// chunk is one PNG chunk with its CRC.
func chunk(kind string, data []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
	b.WriteString(kind)
	b.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(kind))
	crc.Write(data)
	_ = binary.Write(&b, binary.BigEndian, crc.Sum32())
	return b.Bytes()
}

func wantMessage(t *testing.T, err error, part string) {
	t.Helper()
	if apperr.KindOf(err) != apperr.Invalid || !strings.Contains(err.Error(), part) {
		t.Fatalf("got %v, want an invalid error saying %q", err, part)
	}
}

func TestOpaqueBecomesJPEG(t *testing.T) {
	out, mime, err := Process(context.Background(), encodePNG(t, filled(300, 300, solid(color.NRGBA{20, 120, 200, 255}))))
	if err != nil || mime != "image/jpeg" {
		t.Fatalf("Process = %q, %v", mime, err)
	}
	if b := decode(t, out, mime).Bounds(); b.Dx() != OutSide || b.Dy() != OutSide {
		t.Fatalf("size %v, want %d square", b, OutSide)
	}
}

func TestTransparencyKeptWithoutFringe(t *testing.T) {
	// The left half is clear, the right half half-see-through red (the
	// edge off the new pixels' grid). Pixels
	// that mix the two must stay red, only fainter.
	src := filled(300, 300, func(x, _ int) color.Color {
		if x < 149 {
			return color.NRGBA{}
		}
		return color.NRGBA{255, 0, 0, 128}
	})
	out, mime, err := Process(context.Background(), encodePNG(t, src))
	if err != nil || mime != "image/png" {
		t.Fatalf("Process = %q, %v", mime, err)
	}
	img := decode(t, out, mime)
	if img.Bounds().Dx() != OutSide {
		t.Fatalf("size %v", img.Bounds())
	}
	mixed := false
	for x := range OutSide {
		c := color.NRGBAModel.Convert(img.At(x, 10)).(color.NRGBA)
		if c.A == 0 {
			continue
		}
		if c.A < 128 {
			mixed = true
		}
		if c.R < 250 || c.G > 5 || c.B > 5 {
			t.Fatalf("pixel %d is %v, not red", x, c)
		}
	}
	if !mixed {
		t.Fatal("no pixel mixed the two halves, so the test proves nothing")
	}
}

func TestCenterCrop(t *testing.T) {
	// 600×300: blue and green edges of 150 each around a red middle.
	src := filled(600, 300, func(x, _ int) color.Color {
		switch {
		case x < 150:
			return color.NRGBA{0, 0, 255, 255}
		case x >= 450:
			return color.NRGBA{0, 255, 0, 255}
		}
		return color.NRGBA{255, 0, 0, 255}
	})
	out, mime, err := Process(context.Background(), encodePNG(t, src))
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out, mime)
	for _, p := range []image.Point{{0, 0}, {OutSide - 1, 0}, {0, OutSide - 1}, {OutSide - 1, OutSide - 1}} {
		r, g, b, _ := img.At(p.X, p.Y).RGBA()
		if r>>8 < 200 || g>>8 > 60 || b>>8 > 60 {
			t.Errorf("corner %v is %d,%d,%d, not red", p, r>>8, g>>8, b>>8)
		}
	}
}

func TestSmallNotEnlarged(t *testing.T) {
	out, mime, err := Process(context.Background(), encodePNG(t, filled(40, 40, solid(color.NRGBA{9, 9, 9, 255}))))
	if err != nil {
		t.Fatal(err)
	}
	if b := decode(t, out, mime).Bounds(); b.Dx() != 40 || b.Dy() != 40 {
		t.Fatalf("size %v, want 40 square", b)
	}
}

func TestRefused(t *testing.T) {
	ctx := context.Background()
	tiny := encodePNG(t, filled(15, 15, solid(color.White)))
	_, _, err := Process(ctx, tiny)
	wantMessage(t, err, "at least 16 pixels")

	// A header claiming 100000×100000 with no pixel data after it: the
	// size is refused before anything is decoded.
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 100000)
	binary.BigEndian.PutUint32(ihdr[4:], 100000)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	huge := append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr)...)
	_, _, err = Process(ctx, huge)
	wantMessage(t, err, "at most 1024 pixels")

	wide := encodePNG(t, filled(1025, 20, solid(color.White)))
	_, _, err = Process(ctx, wide)
	wantMessage(t, err, "at most 1024 pixels")

	var jb, gb bytes.Buffer
	_ = jpeg.Encode(&jb, filled(64, 64, solid(color.White)), nil)
	_ = gif.Encode(&gb, filled(64, 64, solid(color.White)), nil)
	for name, data := range map[string][]byte{
		"jpeg": jb.Bytes(),
		"gif":  gb.Bytes(),
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"></svg>`),
		"html": []byte(`<!doctype html><html><body>hi</body></html>`),
	} {
		_, _, err := Process(ctx, data)
		if apperr.KindOf(err) != apperr.Invalid || !strings.Contains(err.Error(), "as a PNG") {
			t.Errorf("%s: got %v", name, err)
		}
	}

	whole := encodePNG(t, filled(64, 64, func(x, y int) color.Color { return color.NRGBA{uint8(x * 4), uint8(y * 4), 7, 255} }))
	_, _, err = Process(ctx, whole[:len(whole)/2])
	wantMessage(t, err, "could not be read")
}

func TestMetadataDropped(t *testing.T) {
	const marker = "GPS 51.5007N 0.1246W"
	for _, c := range []color.NRGBA{{10, 20, 30, 255}, {10, 20, 30, 100}} {
		data := encodePNG(t, filled(64, 64, solid(c)))
		// After the signature (8) and IHDR (25), add the chunks a camera or
		// an editor might leave.
		var b bytes.Buffer
		b.Write(data[:33])
		b.Write(chunk("eXIf", []byte("MM\x00\x2a"+marker)))
		b.Write(chunk("tEXt", []byte("Comment\x00"+marker)))
		b.Write(chunk("iCCP", []byte("profile\x00\x00"+marker)))
		b.Write(data[33:])
		out, _, err := Process(context.Background(), b.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{marker, "eXIf", "tEXt", "iCCP"} {
			if bytes.Contains(out, []byte(s)) {
				t.Errorf("alpha %d: the output still holds %q", c.A, s)
			}
		}
	}
}

func TestBoxFilter(t *testing.T) {
	uniform := image.NewRGBA(image.Rect(0, 0, 97, 97))
	for i := range uniform.Pix {
		uniform.Pix[i] = []uint8{40, 90, 160, 255}[i%4]
	}
	out := scale(uniform, 31)
	for i, v := range out.Pix {
		if want := []uint8{40, 90, 160, 255}[i%4]; v != want {
			t.Fatalf("byte %d is %d, want %d", i, v, want)
		}
	}

	two := image.NewRGBA(image.Rect(0, 0, 2, 2))
	copy(two.Pix, []uint8{
		0, 0, 0, 255, 100, 0, 0, 255,
		0, 200, 0, 255, 0, 0, 40, 255,
	})
	one := scale(two, 1)
	if got := one.Pix[:4]; !bytes.Equal(got, []uint8{25, 50, 10, 255}) {
		t.Fatalf("2×2 averaged to %v", got)
	}

	for _, n := range []int{1, 7, 128, 255} {
		for j, ws := range weights(1000, n) {
			sum := 0.0
			for _, w := range ws {
				sum += w.w
			}
			if math.Abs(sum-1) > 1e-9 {
				t.Fatalf("weights(1000, %d)[%d] add up to %v", n, j, sum)
			}
		}
	}
}

func TestCanceledWhileWaiting(t *testing.T) {
	for range cap(slots) {
		slots <- struct{}{}
	}
	defer func() {
		for range cap(slots) {
			<-slots
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := Process(ctx, encodePNG(t, filled(20, 20, solid(color.White))))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the context's error", err)
	}
}
