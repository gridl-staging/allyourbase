package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// makeTestJPEG creates a solid-color JPEG image of the given dimensions.
func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encoding test JPEG: %v", err)
	}
	return buf.Bytes()
}

// makeTestPNG creates a solid-color PNG image of the given dimensions.
func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test PNG: %v", err)
	}
	return buf.Bytes()
}

// decodeResult decodes the output bytes back into an image for dimension assertions.
func decodeResult(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	return img
}

func TestParseFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  Fit
	}{
		{"contain", FitContain},
		{"cover", FitCover},
		{"fill", FitFill},
		{"COVER", FitCover},
		{"Cover", FitCover},
		{"unknown", FitContain},
		{"", FitContain},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tc.want, ParseFit(tc.input))
		})
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input  string
		want   Format
		wantOK bool
	}{
		{"jpeg", FormatJPEG, true},
		{"jpg", FormatJPEG, true},
		{"JPEG", FormatJPEG, true},
		{"png", FormatPNG, true},
		{"PNG", FormatPNG, true},
		{"webp", "", false},
		{"gif", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseFormat(tc.input)
			testutil.Equal(t, tc.want, got)
			testutil.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestFormatFromContentType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ct     string
		want   Format
		wantOK bool
	}{
		{"image/jpeg", FormatJPEG, true},
		{"image/jpeg; charset=utf-8", FormatJPEG, true},
		{"image/png", FormatPNG, true},
		{"IMAGE/PNG", FormatPNG, true},
		{"image/gif", "", false},
		{"application/octet-stream", "", false},
		{"text/plain", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.ct, func(t *testing.T) {
			t.Parallel()
			got, ok := FormatFromContentType(tc.ct)
			testutil.Equal(t, tc.want, got)
			testutil.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestFormatContentType(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "image/jpeg", FormatJPEG.ContentType())
	testutil.Equal(t, "image/png", FormatPNG.ContentType())
	testutil.Equal(t, "application/octet-stream", Format("").ContentType())
}

func TestCalcDimensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		srcW, srcH       int
		targetW, targetH int
		wantW, wantH     int
	}{
		{"both specified", 800, 600, 400, 300, 400, 300},
		{"width only", 800, 600, 400, 0, 400, 300},
		{"height only", 800, 600, 0, 300, 400, 300},
		{"width only non-proportional", 1000, 500, 200, 0, 200, 100},
		{"height only non-proportional", 1000, 500, 0, 100, 200, 100},
		{"clamp to 1 min", 1000, 1, 1, 0, 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := calcDimensions(tc.srcW, tc.srcH, tc.targetW, tc.targetH)
			testutil.Equal(t, tc.wantW, gotW)
			testutil.Equal(t, tc.wantH, gotH)
		})
	}
}

func TestTransformContainJPEG(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 800, 600)
	result, err := TransformBytes(src, Options{Width: 200, Height: 150, Fit: FitContain, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 200, img.Bounds().Dx())
	testutil.Equal(t, 150, img.Bounds().Dy())
}

func TestTransformContainWidthOnly(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 800, 600)
	result, err := TransformBytes(src, Options{Width: 400, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 400, img.Bounds().Dx())
	testutil.Equal(t, 300, img.Bounds().Dy())
}

func TestTransformContainHeightOnly(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 800, 600)
	result, err := TransformBytes(src, Options{Height: 300, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 400, img.Bounds().Dx())
	testutil.Equal(t, 300, img.Bounds().Dy())
}

func TestTransformContainNonProportional(t *testing.T) {
	// 800x600 into 200x400 container → should scale to 200x150 (fit width).
	t.Parallel()

	src := makeTestJPEG(t, 800, 600)
	result, err := TransformBytes(src, Options{Width: 200, Height: 400, Fit: FitContain, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 200, img.Bounds().Dx())
	testutil.Equal(t, 150, img.Bounds().Dy())
}

func TestTransformCover(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 800, 600)
	result, err := TransformBytes(src, Options{Width: 200, Height: 200, Fit: FitCover, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 200, img.Bounds().Dx())
	testutil.Equal(t, 200, img.Bounds().Dy())
}

func TestTransformFill(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 800, 600)
	result, err := TransformBytes(src, Options{Width: 300, Height: 100, Fit: FitFill, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 300, img.Bounds().Dx())
	testutil.Equal(t, 100, img.Bounds().Dy())
}

func TestTransformNoUpscale(t *testing.T) {
	// Requesting dimensions larger than source should return original size.
	t.Parallel()

	src := makeTestJPEG(t, 200, 100)
	result, err := TransformBytes(src, Options{Width: 800, Height: 600, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 200, img.Bounds().Dx())
	testutil.Equal(t, 100, img.Bounds().Dy())
}

func TestTransformFormatConversionJPEGToPNG(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	result, err := TransformBytes(src, Options{Width: 200, Format: FormatPNG})
	testutil.NoError(t, err)

	// Verify it decodes as PNG by checking the raw bytes start with PNG header.
	testutil.True(t, len(result) > 4, "result should not be empty")
	testutil.Equal(t, byte(0x89), result[0])
	testutil.Equal(t, byte('P'), result[1])
	testutil.Equal(t, byte('N'), result[2])
	testutil.Equal(t, byte('G'), result[3])
}

func TestTransformFormatConversionPNGToJPEG(t *testing.T) {
	t.Parallel()
	src := makeTestPNG(t, 400, 300)
	result, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG})
	testutil.NoError(t, err)

	// Verify JPEG header (SOI marker: 0xFF 0xD8).
	testutil.True(t, len(result) > 2, "result should not be empty")
	testutil.Equal(t, byte(0xFF), result[0])
	testutil.Equal(t, byte(0xD8), result[1])
}

func TestTransformPNGSource(t *testing.T) {
	t.Parallel()
	src := makeTestPNG(t, 600, 400)
	result, err := TransformBytes(src, Options{Width: 300, Format: FormatPNG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 300, img.Bounds().Dx())
	testutil.Equal(t, 200, img.Bounds().Dy())
}

func TestTransformQuality(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)

	// Low quality should produce smaller output than high quality.
	lowQ, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG, Quality: 10})
	testutil.NoError(t, err)

	highQ, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG, Quality: 95})
	testutil.NoError(t, err)

	testutil.True(t, len(lowQ) < len(highQ), "low quality should be smaller than high quality")
}

func TestTransformDefaultQuality(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	// Quality 0 should default to 80 (DefaultQuality).
	defaultResult, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG, Quality: 0})
	testutil.NoError(t, err)

	// Explicitly set quality 80 — should produce identical output.
	explicit80, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG, Quality: 80})
	testutil.NoError(t, err)

	testutil.Equal(t, len(explicit80), len(defaultResult))
}

func TestTransformQualityClamped(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	// Quality > 100 should be clamped to 100 (MaxQuality).
	clampedResult, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG, Quality: 999})
	testutil.NoError(t, err)

	// Explicitly set quality 100 — should produce identical output.
	explicit100, err := TransformBytes(src, Options{Width: 200, Format: FormatJPEG, Quality: 100})
	testutil.NoError(t, err)

	testutil.Equal(t, len(explicit100), len(clampedResult))
}

func TestTransformErrorNoDimensions(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	_, err := TransformBytes(src, Options{Format: FormatJPEG})
	testutil.ErrorContains(t, err, "width or height is required")
}

func TestTransformErrorWidthTooLarge(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	_, err := TransformBytes(src, Options{Width: MaxDimension + 1, Format: FormatJPEG})
	testutil.ErrorContains(t, err, "width must be 0-4096")
}

func TestTransformErrorHeightTooLarge(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	_, err := TransformBytes(src, Options{Width: 200, Height: MaxDimension + 1, Format: FormatJPEG})
	testutil.ErrorContains(t, err, "height must be 0-4096")
}

func TestTransformErrorNegativeWidth(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	_, err := TransformBytes(src, Options{Width: -1, Format: FormatJPEG})
	testutil.ErrorContains(t, err, "width must be 0-4096")
}

func TestTransformErrorInvalidImage(t *testing.T) {
	t.Parallel()
	_, err := TransformBytes([]byte("not an image"), Options{Width: 200, Format: FormatJPEG})
	testutil.ErrorContains(t, err, "decoding image")
}

func TestTransformDefaultFormat(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	// When Format is empty, should default to JPEG.
	result, err := TransformBytes(src, Options{Width: 200})
	testutil.NoError(t, err)

	// Verify JPEG header.
	testutil.Equal(t, byte(0xFF), result[0])
	testutil.Equal(t, byte(0xD8), result[1])
}

func TestTransformSquareSource(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 500, 500)
	result, err := TransformBytes(src, Options{Width: 100, Height: 100, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 100, img.Bounds().Dx())
	testutil.Equal(t, 100, img.Bounds().Dy())
}

func TestTransformCoverTallTarget(t *testing.T) {
	// Landscape source into tall target → cover should crop sides.
	t.Parallel()

	src := makeTestJPEG(t, 800, 400)
	result, err := TransformBytes(src, Options{Width: 100, Height: 200, Fit: FitCover, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 100, img.Bounds().Dx())
	testutil.Equal(t, 200, img.Bounds().Dy())
}

func TestTransformCoverWideTarget(t *testing.T) {
	// Portrait source into wide target → cover should crop top/bottom.
	t.Parallel()

	src := makeTestJPEG(t, 400, 800)
	result, err := TransformBytes(src, Options{Width: 200, Height: 100, Fit: FitCover, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 200, img.Bounds().Dx())
	testutil.Equal(t, 100, img.Bounds().Dy())
}

func TestTransformSmallImage(t *testing.T) {
	// Very small source image.
	t.Parallel()

	src := makeTestJPEG(t, 10, 10)
	result, err := TransformBytes(src, Options{Width: 5, Height: 5, Format: FormatJPEG})
	testutil.NoError(t, err)

	img := decodeResult(t, result)
	testutil.Equal(t, 5, img.Bounds().Dx())
	testutil.Equal(t, 5, img.Bounds().Dy())
}
