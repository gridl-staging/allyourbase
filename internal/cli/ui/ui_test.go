package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// --- FormatError ---

func TestFormatErrorBasicMessage(t *testing.T) {
	out := FormatError("something broke")
	if !strings.Contains(out, "Error:") {
		t.Error("expected 'Error:' prefix")
	}
	if !strings.Contains(out, "something broke") {
		t.Error("expected message in output")
	}
}

func TestFormatErrorNoSuggestions(t *testing.T) {
	out := FormatError("something broke")
	if strings.Contains(out, "Try:") {
		t.Error("should not contain 'Try:' when no suggestions")
	}
}

func TestFormatErrorWithSuggestions(t *testing.T) {
	out := FormatError("port 8090 in use",
		"ayb start --port 8091",
		"ayb stop",
	)
	if !strings.Contains(out, "Try:") {
		t.Error("expected 'Try:' section")
	}
	if !strings.Contains(out, "ayb start --port 8091") {
		t.Error("expected first suggestion")
	}
	if !strings.Contains(out, "ayb stop") {
		t.Error("expected second suggestion")
	}
	if !strings.Contains(out, SymbolArrow) {
		t.Error("expected arrow symbol in suggestions")
	}
}

func TestFormatErrorSingleSuggestion(t *testing.T) {
	out := FormatError("disk full", "free up space")
	if !strings.Contains(out, "Try:") {
		t.Error("expected 'Try:' section")
	}
	if !strings.Contains(out, "free up space") {
		t.Error("expected suggestion")
	}
}

// --- StepSpinner (non-TTY / noSpin mode) ---

func TestStepSpinnerNoSpinStart(t *testing.T) {
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)
	sp.Start("Connecting...")

	out := buf.String()
	if !strings.Contains(out, "Connecting...") {
		t.Errorf("expected step message, got %q", out)
	}
}

func TestStepSpinnerNoSpinDone(t *testing.T) {
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)
	sp.Start("Connecting...")
	sp.Done()

	out := buf.String()
	if !strings.Contains(out, SymbolCheck) {
		t.Errorf("expected check symbol in done output, got %q", out)
	}
}

func TestStepSpinnerNoSpinFail(t *testing.T) {
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)
	sp.Start("Connecting...")
	sp.Fail()

	out := buf.String()
	if !strings.Contains(out, SymbolCross) {
		t.Errorf("expected cross symbol in fail output, got %q", out)
	}
}

func TestStepSpinnerStopNoPanic(t *testing.T) {
	// Stop without Start should not panic.
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)
	sp.Stop() // no-op, should not panic
}

func TestStepSpinnerDoneWithoutStartNoPanic(t *testing.T) {
	// Done without Start should not panic in noSpin mode.
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)
	sp.Done()
}

func TestStepSpinnerFailWithoutStartNoPanic(t *testing.T) {
	// Fail without Start should not panic in noSpin mode.
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)
	sp.Fail()
}

func TestStepSpinnerMultipleSteps(t *testing.T) {
	var buf bytes.Buffer
	sp := NewStepSpinner(&buf, true)

	sp.Start("Step 1...")
	sp.Done()
	sp.Start("Step 2...")
	sp.Done()

	out := buf.String()
	if !strings.Contains(out, "Step 1...") {
		t.Error("expected Step 1")
	}
	if !strings.Contains(out, "Step 2...") {
		t.Error("expected Step 2")
	}
	if strings.Count(out, SymbolCheck) != 2 {
		t.Errorf("expected 2 check marks, got %d", strings.Count(out, SymbolCheck))
	}
}

// --- Constants ---

func TestBrandEmojiIsNotEmpty(t *testing.T) {
	if BrandEmoji == "" {
		t.Error("BrandEmoji should not be empty")
	}
	if BrandEmoji != "👾" {
		t.Errorf("BrandEmoji should be 👾, got %q", BrandEmoji)
	}
}

func TestSymbolConstants(t *testing.T) {
	symbols := map[string]string{
		"SymbolCheck":   SymbolCheck,
		"SymbolCross":   SymbolCross,
		"SymbolWarning": SymbolWarning,
		"SymbolDot":     SymbolDot,
		"SymbolArrow":   SymbolArrow,
	}
	for name, sym := range symbols {
		if sym == "" {
			t.Errorf("%s should not be empty", name)
		}
	}
}

// --- ColorEnabled ---

func TestColorEnabledRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if ColorEnabled() {
		t.Error("ColorEnabled should return false when NO_COLOR is set")
	}
}

func TestColorEnabledEmptyNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	// NO_COLOR spec says presence (even empty) disables color.
	if ColorEnabled() {
		t.Error("ColorEnabled should return false when NO_COLOR is set to empty string")
	}
}

func TestColorEnabledFdRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if ColorEnabledFd(os.Stderr.Fd()) {
		t.Error("ColorEnabledFd should return false when NO_COLOR is set")
	}
}

func TestColorEnabledInTests(t *testing.T) {
	// In test environments, stderr is not a TTY, so color should be disabled
	// (unless NO_COLOR is set, which also disables it).
	// Use t.Setenv first to snapshot+restore, then unset for this test.
	t.Setenv("NO_COLOR", "placeholder")
	os.Unsetenv("NO_COLOR")
	// Regardless of NO_COLOR, test environment has no TTY
	// so ColorEnabled() should be false.
	if ColorEnabled() {
		t.Error("ColorEnabled should return false in non-TTY test environment")
	}
}

// --- ForcedRenderer ---

func TestForcedRendererReturnsNonNil(t *testing.T) {
	r := ForcedRenderer()
	if r == nil {
		t.Fatal("ForcedRenderer should not return nil")
	}
}

func TestForcedRendererProducesANSI(t *testing.T) {
	r := ForcedRenderer()
	out := r.NewStyle().Bold(true).Render("test")
	if !strings.Contains(out, "test") {
		t.Error("rendered text should contain original text")
	}
	// Verify the bold SGR attribute (1) is present, not just any ANSI code.
	// It may appear as standalone "\033[1m", combined-start "\033[1;", or combined-end ";1m".
	hasBoldSGR := strings.Contains(out, "\033[1m") ||
		strings.Contains(out, "\033[1;") ||
		strings.Contains(out, ";1m")
	if !hasBoldSGR {
		t.Errorf("forced renderer should produce bold SGR attribute (1), got %q", out)
	}
}

func TestForcedRendererSingleton(t *testing.T) {
	r1 := ForcedRenderer()
	r2 := ForcedRenderer()
	if r1 != r2 {
		t.Error("ForcedRenderer should return the same instance")
	}
}

// --- Styles render text ---

func TestStylesRenderText(t *testing.T) {
	// Styles should not lose the original text content.
	tests := []struct {
		name  string
		style func(...string) string
	}{
		{"StyleBold", StyleBold.Render},
		{"StyleDim", StyleDim.Render},
		{"StyleCyan", StyleCyan.Render},
		{"StyleGreen", StyleGreen.Render},
		{"StyleYellow", StyleYellow.Render},
		{"StyleRed", StyleRed.Render},
		{"StyleBoldCyan", StyleBoldCyan.Render},
		{"StyleBoldGreen", StyleBoldGreen.Render},
		{"StyleBoldRed", StyleBoldRed.Render},
		{"StyleSuccess", StyleSuccess.Render},
		{"StyleWarning", StyleWarning.Render},
		{"StyleError", StyleError.Render},
		{"StyleBrandHeader", StyleBrandHeader.Render},
		{"StyleCode", StyleCode.Render},
		{"StyleHint", StyleHint.Render},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.style("hello")
			if !strings.Contains(out, "hello") {
				t.Errorf("%s.Render(\"hello\") = %q, does not contain original text", tt.name, out)
			}
		})
	}
}
