package gallery

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Processed output limits. Every gallery photo is stored as a 4:3 JPEG
// (matching the grid tiles) at these widths, so uploads from cameras and
// phones never ship multi-megabyte downloads to visitors.
const (
	// MaxPhotoPixels rejects decompression bombs before decoding.
	MaxPhotoPixels = 40_000_000
	// MainMaxWidth is the long-edge width of the full-size gallery photo.
	MainMaxWidth = 1600
	// PreviewMaxWidth is the width of the lightweight grid tile image.
	PreviewMaxWidth = 640
)

// JPEG encoding quality for processed photos. 85 keeps the 1600px main
// visually lossless for a photo gallery while staying a few hundred KB.
const (
	mainJPEGQuality    = 85
	previewJPEGQuality = 78
)

// photoBackground is composited behind transparent PNG/WebP pixels before
// JPEG encoding, matching the dark gallery tiles.
var photoBackground = color.RGBA{R: 10, G: 10, B: 10, A: 255}

// ProcessPhoto normalizes an uploaded photo for the gallery:
//   - decodes JPEG, PNG or WebP and rejects decompression bombs,
//   - center-crops to the 4:3 tile ratio,
//   - downscales to at most 1600 px wide (never upscales),
//   - re-encodes as a compressed JPEG,
//   - produces a 640 px wide preview for the grid.
//
// It reports false for unsupported or unreadable input.
func ProcessPhoto(data []byte) (main []byte, preview []byte, ok bool) {
	if len(data) == 0 {
		return nil, nil, false
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !photoWithinPixelLimit(cfg.Width, cfg.Height) {
		return nil, nil, false
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, false
	}

	cropped := centerCropFourThree(src)
	if cropped.Bounds().Dx() == 0 || cropped.Bounds().Dy() == 0 {
		return nil, nil, false
	}

	main, err = encodeJPEG(cropped, MainMaxWidth, mainJPEGQuality)
	if err != nil {
		return nil, nil, false
	}

	preview, err = encodeJPEG(cropped, PreviewMaxWidth, previewJPEGQuality)
	if err != nil {
		return nil, nil, false
	}

	return main, preview, true
}

// photoWithinPixelLimit reports whether an image of w×h pixels is small
// enough to decode safely.
func photoWithinPixelLimit(w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}

	return h <= MaxPhotoPixels/w
}

// centerCropFourThree returns the largest centered 4:3 rectangle of img.
func centerCropFourThree(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}

	cropW, cropH := w, h
	if w*3 > h*4 {
		cropW = h * 4 / 3
	} else if w*3 < h*4 {
		cropH = w * 3 / 4
	}

	x0 := b.Min.X + (w-cropW)/2
	y0 := b.Min.Y + (h-cropH)/2

	return &subImage{Image: img, rect: image.Rect(x0, y0, x0+cropW, y0+cropH)}
}

// subImage is a zero-copy view of a rectangle inside another image.
// image/draw only needs Bounds/At/ColorModel, so the view keeps cropping
// cheap even for 40 MP uploads.
type subImage struct {
	image.Image
	rect image.Rectangle
}

// Bounds returns the cropped rectangle.
func (s *subImage) Bounds() image.Rectangle { return s.rect }

// At returns the pixel at the absolute coordinates of the crop.
func (s *subImage) At(x, y int) color.Color { return s.Image.At(x, y) }

// encodeJPEG scales img to at most targetWidth (never upscaling), keeping
// the 4:3 ratio, and encodes it as an opaque JPEG.
func encodeJPEG(img image.Image, targetWidth, quality int) ([]byte, error) {
	b := img.Bounds()
	srcW := b.Dx()

	dstW := min(srcW, targetWidth)
	dstH := (dstW*3 + 2) / 4
	if dstH == 0 {
		dstH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: photoBackground}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
