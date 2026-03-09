package ui

import (
	"fmt"
	"io"
	"time"

	"github.com/briandowns/spinner"
)

// StepSpinner provides animated feedback for sequential startup steps.
// In TTY mode it shows a braille dot spinner; in non-TTY mode it
// prints static text so piped/CI output stays clean.
type StepSpinner struct {
	w      io.Writer
	s      *spinner.Spinner
	msg    string
	active bool
	noSpin bool // true when not a TTY
}

// NewStepSpinner creates a spinner that writes to w.
// Set noSpin=true for non-interactive environments.
func NewStepSpinner(w io.Writer, noSpin bool) *StepSpinner {
	return &StepSpinner{w: w, noSpin: noSpin}
}

// Start begins a named step with an animated spinner (or static text).
func (ss *StepSpinner) Start(msg string) {
	ss.msg = msg
	if ss.noSpin {
		fmt.Fprintf(ss.w, "  %s", msg)
		return
	}
	ss.s = spinner.New(
		spinner.CharSets[14], // braille dots: ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏
		80*time.Millisecond,
		spinner.WithWriter(ss.w),
	)
	ss.s.Prefix = "  "
	ss.s.Suffix = " " + msg
	ss.s.FinalMSG = ""
	ss.s.Start()
	ss.active = true
}

// Done completes the current step with a green checkmark.
func (ss *StepSpinner) Done() {
	if ss.noSpin {
		fmt.Fprintf(ss.w, " %s\n", StyleSuccess.Render(SymbolCheck))
		return
	}
	if ss.s != nil && ss.active {
		ss.s.Stop()
		ss.active = false
	}
	fmt.Fprintf(ss.w, "\r  %s %s\n", ss.msg, StyleSuccess.Render(SymbolCheck))
}

// Fail completes the current step with a red cross.
func (ss *StepSpinner) Fail() {
	if ss.noSpin {
		fmt.Fprintf(ss.w, " %s\n", StyleError.Render(SymbolCross))
		return
	}
	if ss.s != nil && ss.active {
		ss.s.Stop()
		ss.active = false
	}
	fmt.Fprintf(ss.w, "\r  %s %s\n", ss.msg, StyleError.Render(SymbolCross))
}

// Stop halts the spinner without printing a status (for cleanup on signals).
func (ss *StepSpinner) Stop() {
	if ss.s != nil && ss.active {
		ss.s.Stop()
		ss.active = false
	}
}
