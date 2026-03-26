// Package imaging provides image transformation and resizing with support for multiple output formats, fit strategies, and intelligent cropping.
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"

	"github.com/chai2010/webp"
	"github.com/gen2brain/avif"
	"golang.org/x/image/draw"
)

// Fit describes how the image should be resized relative to the target dimensions.
type Fit string

const (
	FitContain Fit = "contain" // Scale to fit within dimensions, preserving aspect ratio.
	FitCover   Fit = "cover"   // Scale and center-crop to fill dimensions exactly.
	FitFill    Fit = "fill"    // Stretch to fill dimensions exactly (may distort).
)

// Format is the output image format.
type Format string

const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatWebP Format = "webp"
	FormatAVIF Format = "avif"
)

// CropMode controls how cropping is applied when the image is scaled to cover
// target dimensions.
type CropMode string

const (
	CropNone   CropMode = ""
	CropCenter CropMode = "center" // Scale to cover, then center-crop.
	CropSmart  CropMode = "smart"  // Scale to cover, then entropy-based crop.
)

const (
	MaxDimension   = 4096
	DefaultQuality = 80
	MaxQuality     = 100
)

// Options specifies the desired image transformation.
type Options struct {
	Width   int
	Height  int
	Fit     Fit
	Crop    CropMode // When set, overrides Fit with cover behavior and uses specified crop strategy.
	Quality int      // JPEG/WebP quality 1-100 (default 80).
	Format  Format   // Output format (empty = preserve source format).
}

// ParseFit parses a fit string, returning FitContain for unrecognized values.
func ParseFit(s string) Fit {
	switch strings.ToLower(s) {
	case "cover":
		return FitCover
	case "fill":
		return FitFill
	default:
		return FitContain
	}
}

// ParseFormat parses a format string, returning ok=false for unsupported formats.
func ParseFormat(s string) (Format, bool) {
	switch strings.ToLower(s) {
	case "jpeg", "jpg":
		return FormatJPEG, true
	case "png":
		return FormatPNG, true
	case "webp":
		return FormatWebP, true
	case "avif":
		return FormatAVIF, true
	default:
		return "", false
	}
}

// ParseCropMode parses a crop mode string. Returns ok=false for unrecognized values.
func ParseCropMode(s string) (CropMode, bool) {
	switch strings.ToLower(s) {
	case "center":
		return CropCenter, true
	case "smart":
		return CropSmart, true
	case "":
		return CropNone, true
	default:
		return "", false
	}
}

// FormatFromContentType returns the image format for a MIME content type.
func FormatFromContentType(ct string) (Format, bool) {
	ct = strings.ToLower(ct)
	switch {
	case strings.HasPrefix(ct, "image/jpeg"):
		return FormatJPEG, true
	case strings.HasPrefix(ct, "image/png"):
		return FormatPNG, true
	case strings.HasPrefix(ct, "image/webp"):
		return FormatWebP, true
	default:
		return "", false
	}
}

// ContentType returns the MIME type for a format.
func (f Format) ContentType() string {
	switch f {
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	case FormatWebP:
		return "image/webp"
	case FormatAVIF:
		return "image/avif"
	default:
		return "application/octet-stream"
	}
}

// IsAnimatedGIF reads GIF data and returns true if it contains more than one frame.
// The reader is consumed; callers should buffer or duplicate the data if it will be
// needed again.
func IsAnimatedGIF(r io.Reader) (bool, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return false, fmt.Errorf("decoding GIF: %w", err)
	}
	return len(g.Image) > 1, nil
}

// Transform reads an image, applies the requested transformations, and writes the result.
func Transform(r io.Reader, w io.Writer, opts Options) error {
	src, _, err := image.Decode(r)
	if err != nil {
		return fmt.Errorf("decoding image: %w", err)
	}

	if err := validateOptions(&opts); err != nil {
		return err
	}

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	if srcW == 0 || srcH == 0 {
		return fmt.Errorf("source image has zero dimensions")
	}

	targetW, targetH := calcDimensions(srcW, srcH, opts.Width, opts.Height)

	// Don't upscale: if both target dims exceed source, use source dims.
	if targetW >= srcW && targetH >= srcH {
		targetW = srcW
		targetH = srcH
	}

	var dst *image.RGBA
	switch {
	case opts.Crop == CropSmart:
		dst = resizeSmartCrop(src, srcW, srcH, targetW, targetH)
	case opts.Crop == CropCenter:
		dst = resizeCover(src, srcW, srcH, targetW, targetH)
	case opts.Fit == FitCover:
		dst = resizeCover(src, srcW, srcH, targetW, targetH)
	case opts.Fit == FitFill:
		dst = resizeFill(src, targetW, targetH)
	default:
		dst = resizeContain(src, srcW, srcH, targetW, targetH)
	}

	return encode(w, dst, opts)
}

// TransformBytes is a convenience wrapper that operates on byte slices.
func TransformBytes(data []byte, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := Transform(bytes.NewReader(data), &buf, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Validates the Options fields, ensuring dimensions are in valid ranges and required option combinations are specified, then applies defaults for Fit, Format, and Quality based on the specified format.
func validateOptions(opts *Options) error {
	if opts.Width < 0 || opts.Width > MaxDimension {
		return fmt.Errorf("width must be 0-%d", MaxDimension)
	}
	if opts.Height < 0 || opts.Height > MaxDimension {
		return fmt.Errorf("height must be 0-%d", MaxDimension)
	}
	if opts.Width == 0 && opts.Height == 0 {
		return fmt.Errorf("width or height is required")
	}
	if opts.Crop != CropNone {
		// Crop modes require both dimensions.
		if opts.Width == 0 || opts.Height == 0 {
			return fmt.Errorf("crop mode requires both width and height")
		}
	}
	if opts.Fit == "" {
		opts.Fit = FitContain
	}
	if opts.Format == "" {
		opts.Format = FormatJPEG
	}
	if opts.Quality <= 0 {
		if opts.Format == FormatAVIF {
			opts.Quality = 50
		} else {
			opts.Quality = DefaultQuality
		}
	}
	if opts.Quality > MaxQuality {
		opts.Quality = MaxQuality
	}
	return nil
}

// calcDimensions computes target width and height, preserving aspect ratio
// when only one dimension is specified.
func calcDimensions(srcW, srcH, targetW, targetH int) (int, int) {
	if targetW == 0 && targetH != 0 {
		targetW = srcW * targetH / srcH
	}
	if targetH == 0 && targetW != 0 {
		targetH = srcH * targetW / srcW
	}
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}
	return targetW, targetH
}

// resizeContain scales the image to fit within targetW x targetH, preserving aspect ratio.
// The result may be smaller than target dimensions on one axis.
func resizeContain(src image.Image, srcW, srcH, targetW, targetH int) *image.RGBA {
	ratioW := float64(targetW) / float64(srcW)
	ratioH := float64(targetH) / float64(srcH)
	ratio := ratioW
	if ratioH < ratioW {
		ratio = ratioH
	}

	dstW := max(1, int(float64(srcW)*ratio))
	dstH := max(1, int(float64(srcH)*ratio))

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// resizeCover scales and center-crops the image to exactly fill targetW x targetH.
func resizeCover(src image.Image, srcW, srcH, targetW, targetH int) *image.RGBA {
	ratioW := float64(targetW) / float64(srcW)
	ratioH := float64(targetH) / float64(srcH)
	ratio := ratioW
	if ratioH > ratioW {
		ratio = ratioH
	}

	scaledW := max(1, int(float64(srcW)*ratio))
	scaledH := max(1, int(float64(srcH)*ratio))

	scaled := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), src, src.Bounds(), draw.Over, nil)

	// Center-crop to target.
	offsetX := (scaledW - targetW) / 2
	offsetY := (scaledH - targetH) / 2
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Copy(dst, image.Point{}, scaled, image.Rect(offsetX, offsetY, offsetX+targetW, offsetY+targetH), draw.Over, nil)
	return dst
}

// resizeSmartCrop scales to cover dimensions and crops around the highest-entropy region.
func resizeSmartCrop(src image.Image, srcW, srcH, targetW, targetH int) *image.RGBA {
	ratioW := float64(targetW) / float64(srcW)
	ratioH := float64(targetH) / float64(srcH)
	ratio := ratioW
	if ratioH > ratioW {
		ratio = ratioH
	}

	scaledW := max(1, int(float64(srcW)*ratio))
	scaledH := max(1, int(float64(srcH)*ratio))

	scaled := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), src, src.Bounds(), draw.Over, nil)

	offsetX, offsetY := findEntropyOffset(scaled, scaledW, scaledH, targetW, targetH)
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Copy(dst, image.Point{}, scaled, image.Rect(offsetX, offsetY, offsetX+targetW, offsetY+targetH), draw.Over, nil)
	return dst
}

// findEntropyOffset finds the crop offset within the scaled image that maximizes
// pixel variance (entropy). It slides the target-sized window along the axis that
// has excess pixels and picks the position with the highest variance sum.
func findEntropyOffset(img *image.RGBA, scaledW, scaledH, targetW, targetH int) (int, int) {
	excessX := scaledW - targetW
	excessY := scaledH - targetH

	// If no excess on either axis, no offset needed.
	if excessX <= 0 && excessY <= 0 {
		return 0, 0
	}

	bestX, bestY := 0, 0

	// Slide along X axis if there's horizontal excess.
	if excessX > 0 {
		bestVariance := -1.0
		step := max(1, excessX/20) // Sample up to ~20 positions for speed.
		for x := 0; x <= excessX; x += step {
			v := regionVariance(img, x, 0, targetW, targetH)
			if v > bestVariance {
				bestVariance = v
				bestX = x
			}
		}
	}

	// Slide along Y axis if there's vertical excess.
	if excessY > 0 {
		bestVariance := -1.0
		step := max(1, excessY/20)
		for y := 0; y <= excessY; y += step {
			v := regionVariance(img, bestX, y, targetW, targetH)
			if v > bestVariance {
				bestVariance = v
				bestY = y
			}
		}
	}

	return bestX, bestY
}

// regionVariance calculates the sum of per-channel variance for a rectangular region,
// used as an entropy proxy. Higher variance = more detail = more "interesting".
func regionVariance(img *image.RGBA, x0, y0, w, h int) float64 {
	// Sample pixels for speed — no need to check every single pixel.
	const maxSamples = 256
	stepX := max(1, w/16)
	stepY := max(1, h/16)

	var sumR, sumG, sumB float64
	var sumR2, sumG2, sumB2 float64
	var n float64

	for y := y0; y < y0+h && n < maxSamples; y += stepY {
		for x := x0; x < x0+w && n < maxSamples; x += stepX {
			c := img.RGBAAt(x, y)
			fr, fg, fb := float64(c.R), float64(c.G), float64(c.B)
			sumR += fr
			sumG += fg
			sumB += fb
			sumR2 += fr * fr
			sumG2 += fg * fg
			sumB2 += fb * fb
			n++
		}
	}
	if n == 0 {
		return 0
	}

	varR := sumR2/n - math.Pow(sumR/n, 2)
	varG := sumG2/n - math.Pow(sumG/n, 2)
	varB := sumB2/n - math.Pow(sumB/n, 2)
	return varR + varG + varB
}

// resizeFill stretches the image to exactly fill targetW x targetH (may distort).
func resizeFill(src image.Image, targetW, targetH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func encode(w io.Writer, img image.Image, opts Options) error {
	switch opts.Format {
	case FormatPNG:
		return png.Encode(w, img)
	case FormatWebP:
		return webp.Encode(w, img, &webp.Options{Quality: float32(opts.Quality)})
	case FormatAVIF:
		return avif.Encode(w, img, avif.Options{Quality: opts.Quality})
	default:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: opts.Quality})
	}
}
