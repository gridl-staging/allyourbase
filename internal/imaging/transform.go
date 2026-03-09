package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

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
	Quality int    // JPEG quality 1-100 (default 80).
	Format  Format // Output format (empty = preserve source format).
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
	default:
		return "application/octet-stream"
	}
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
	switch opts.Fit {
	case FitCover:
		dst = resizeCover(src, srcW, srcH, targetW, targetH)
	case FitFill:
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
	if opts.Fit == "" {
		opts.Fit = FitContain
	}
	if opts.Quality <= 0 {
		opts.Quality = DefaultQuality
	}
	if opts.Quality > MaxQuality {
		opts.Quality = MaxQuality
	}
	if opts.Format == "" {
		opts.Format = FormatJPEG
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
	default:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: opts.Quality})
	}
}
