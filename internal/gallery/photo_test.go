package gallery

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// testImage builds an RGB test image with a smooth gradient so JPEG
// encoding behaves like a real photo (not a flat color).
func testImage(t *testing.T, w, h int) *image.RGBA {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 255 / max(w, 1)),
				G: uint8(y * 255 / max(h, 1)),
				B: uint8((x + y) * 255 / max(w+h, 1)),
				A: 255,
			})
		}
	}

	return img
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(t, w, h), &jpeg.Options{Quality: 92}); err != nil {
		t.Fatalf("encode test jpeg: %v", err)
	}

	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage(t, w, h)); err != nil {
		t.Fatalf("encode test png: %v", err)
	}

	return buf.Bytes()
}

func mainSize(t *testing.T, data []byte) (int, int) {
	t.Helper()

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode processed jpeg: %v", err)
	}

	return cfg.Width, cfg.Height
}

func TestProcessPhotoProducesFourThreeMainAndPreview(t *testing.T) {
	src := testJPEG(t, 2400, 2400)

	main, preview, ok := ProcessPhoto(src)
	if !ok {
		t.Fatal("expected square photo to be processed")
	}

	mw, mh := mainSize(t, main)
	if mw != MainMaxWidth || mh != MainMaxWidth*3/4 {
		t.Fatalf("expected main %d×%d, got %d×%d", MainMaxWidth, MainMaxWidth*3/4, mw, mh)
	}

	pw, ph := mainSize(t, preview)
	if pw != PreviewMaxWidth || ph != PreviewMaxWidth*3/4 {
		t.Fatalf("expected preview %d×%d, got %d×%d", PreviewMaxWidth, PreviewMaxWidth*3/4, pw, ph)
	}

	if len(main) >= len(src) {
		t.Fatalf("expected main to be smaller than source (%d >= %d)", len(main), len(src))
	}

	if len(preview) >= len(main) {
		t.Fatalf("expected preview smaller than main (%d >= %d)", len(preview), len(main))
	}
}

func TestProcessPhotoAcceptsPNG(t *testing.T) {
	main, _, ok := ProcessPhoto(testPNG(t, 1200, 900))
	if !ok {
		t.Fatal("expected png to be processed")
	}

	if w, h := mainSize(t, main); w != 1200 || h != 900 {
		t.Fatalf("expected 4:3 png to keep its size, got %d×%d", w, h)
	}
}

func TestProcessPhotoDoesNotUpscale(t *testing.T) {
	main, preview, ok := ProcessPhoto(testJPEG(t, 800, 600))
	if !ok {
		t.Fatal("expected small photo to be processed")
	}

	if w, h := mainSize(t, main); w != 800 || h != 600 {
		t.Fatalf("expected main to stay 800×600, got %d×%d", w, h)
	}

	if w, h := mainSize(t, preview); w != 640 || h != 480 {
		t.Fatalf("expected preview capped at 640×480, got %d×%d", w, h)
	}
}

func TestProcessPhotoCropsToFourThree(t *testing.T) {
	// A 2000×1000 panorama center-crops to 1333×1000 (4:3) and needs no
	// downscale, so the main keeps the crop height.
	main, _, ok := ProcessPhoto(testJPEG(t, 2000, 1000))
	if !ok {
		t.Fatal("expected panorama to be processed")
	}

	if w, h := mainSize(t, main); w != 1333 || h != 1000 {
		t.Fatalf("expected centered 4:3 crop 1333×1000, got %d×%d", w, h)
	}
}

func TestProcessPhotoRejectsNonImage(t *testing.T) {
	if _, _, ok := ProcessPhoto([]byte("%PDF-1.4\nnot an image")); ok {
		t.Fatal("expected non-image bytes to be rejected")
	}

	if _, _, ok := ProcessPhoto(nil); ok {
		t.Fatal("expected empty upload to be rejected")
	}
}

func TestPhotoWithinPixelLimit(t *testing.T) {
	if !photoWithinPixelLimit(4000, 3000) {
		t.Fatal("expected 12 MP to be within the limit")
	}

	if photoWithinPixelLimit(MaxPhotoPixels, 2) {
		t.Fatal("expected huge dimensions to exceed the limit")
	}

	if photoWithinPixelLimit(0, 100) || photoWithinPixelLimit(100, 0) {
		t.Fatal("expected zero dimensions to be rejected")
	}
}
